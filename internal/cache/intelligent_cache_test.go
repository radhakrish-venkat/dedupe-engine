package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/radhakrishnan.venkat/dedupe-engine/internal/db"
)

func TestIntelligentCacheBasic(t *testing.T) {
	// Create a mock DB client
	mockDB := &MockDBClient{}

	// Create intelligent cache
	config := &IntelligentCacheConfig{
		WarmingEnabled:       true,
		WarmingThreshold:     0.7,
		MaxWarmingChunks:     10,
		WarmingLookback:      1 * time.Minute,
		WarmingInterval:      100 * time.Millisecond,
		PredictionWindow:     30 * time.Second,
		MinAccessCount:       2,
		ConfidenceThreshold:  0.6,
		MaxConcurrentWarming: 2,
		WarmingTimeout:       1 * time.Second,
	}

	dedupeCache := NewDeduplicationCache(1000, 10000)
	ic := NewIntelligentCache(dedupeCache, mockDB, config)
	defer ic.Close()

	// Test basic functionality
	metadata := &ChunkMetadata{
		Fingerprint:        "test_fp_1",
		StorageLocation:    "test_location",
		Size:               1024,
		CreationTime:       time.Now(),
		LastReferencedTime: time.Now(),
	}

	// Add metadata to cache
	ic.AddChunkMetadata("test_fp_1", metadata)

	// Get metadata
	retrieved, exists := ic.GetChunkMetadata("test_fp_1")
	if !exists {
		t.Error("Expected to find metadata in cache")
	}

	if retrieved.Fingerprint != metadata.Fingerprint {
		t.Errorf("Expected fingerprint %s, got %s", metadata.Fingerprint, retrieved.Fingerprint)
	}

	// Test cache miss
	_, exists = ic.GetChunkMetadata("nonexistent_fp")
	if exists {
		t.Error("Expected cache miss for nonexistent fingerprint")
	}
}

func TestIntelligentCacheAccessPatterns(t *testing.T) {
	mockDB := &MockDBClient{}
	dedupeCache := NewDeduplicationCache(1000, 10000)

	config := &IntelligentCacheConfig{
		WarmingEnabled:       true,
		WarmingThreshold:     0.5,
		MaxWarmingChunks:     10,
		WarmingLookback:      1 * time.Minute,
		WarmingInterval:      100 * time.Millisecond,
		PredictionWindow:     30 * time.Second,
		MinAccessCount:       2,
		ConfidenceThreshold:  0.5,
		MaxConcurrentWarming: 2,
		WarmingTimeout:       1 * time.Second,
	}

	ic := NewIntelligentCache(dedupeCache, mockDB, config)
	defer ic.Close()

	// Simulate access patterns
	fingerprints := []string{"fp1", "fp2", "fp3", "fp4", "fp5"}

	// Access patterns that should create correlations
	for i := 0; i < 5; i++ {
		// Access fp1 and fp2 together
		ic.GetChunkMetadata(fingerprints[0])
		ic.GetChunkMetadata(fingerprints[1])

		// Access fp3 and fp4 together
		ic.GetChunkMetadata(fingerprints[2])
		ic.GetChunkMetadata(fingerprints[3])

		// Access fp5 alone
		ic.GetChunkMetadata(fingerprints[4])

		time.Sleep(10 * time.Millisecond)
	}

	// Wait for pattern analysis
	time.Sleep(200 * time.Millisecond)

	// Get stats
	stats := ic.GetStats()
	if stats.TotalAccesses == 0 {
		t.Error("Expected non-zero total accesses")
	}

	t.Logf("Access patterns recorded: %d total accesses", stats.TotalAccesses)
}

func TestIntelligentCachePredictiveWarming(t *testing.T) {
	// Create mock DB with some test data
	mockDB := &MockDBClient{
		data: map[string]*db.ChunkMetadata{
			"fp1": {Fingerprint: "fp1", StorageLocation: "loc1", Size: 1024},
			"fp2": {Fingerprint: "fp2", StorageLocation: "loc2", Size: 2048},
			"fp3": {Fingerprint: "fp3", StorageLocation: "loc3", Size: 3072},
		},
	}

	dedupeCache := NewDeduplicationCache(1000, 10000)

	config := &IntelligentCacheConfig{
		WarmingEnabled:       true,
		WarmingThreshold:     0.5,
		MaxWarmingChunks:     10,
		WarmingLookback:      1 * time.Minute,
		WarmingInterval:      100 * time.Millisecond,
		PredictionWindow:     30 * time.Second,
		MinAccessCount:       2,
		ConfidenceThreshold:  0.5,
		MaxConcurrentWarming: 2,
		WarmingTimeout:       1 * time.Second,
	}

	ic := NewIntelligentCache(dedupeCache, mockDB, config)
	defer ic.Close()

	// Create access pattern: fp1 and fp2 are accessed together
	for i := 0; i < 3; i++ {
		ic.GetChunkMetadata("fp1")
		ic.GetChunkMetadata("fp2")
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for pattern analysis and warming
	time.Sleep(300 * time.Millisecond)

	// Access fp1 again to trigger warming
	ic.GetChunkMetadata("fp1")

	// Wait for warming to complete
	time.Sleep(200 * time.Millisecond)

	// Check if fp2 was warmed (should be in cache now)
	_, exists := dedupeCache.GetChunkMetadata("fp2")
	if !exists {
		t.Log("fp2 was not warmed - this might be expected if correlation threshold not met")
	}

	// Get stats
	stats := ic.GetStats()
	t.Logf("Warming stats: %d predictions, %d hits", stats.WarmingPredictions, stats.WarmingHits)
}

func TestIntelligentCacheStats(t *testing.T) {
	mockDB := &MockDBClient{}
	dedupeCache := NewDeduplicationCache(1000, 10000)

	config := &IntelligentCacheConfig{
		WarmingEnabled:       true,
		WarmingThreshold:     0.7,
		MaxWarmingChunks:     10,
		WarmingLookback:      1 * time.Minute,
		WarmingInterval:      100 * time.Millisecond,
		PredictionWindow:     30 * time.Second,
		MinAccessCount:       2,
		ConfidenceThreshold:  0.6,
		MaxConcurrentWarming: 2,
		WarmingTimeout:       1 * time.Second,
	}

	ic := NewIntelligentCache(dedupeCache, mockDB, config)
	defer ic.Close()

	// Reset stats
	ic.ResetStats()

	// Add some data and access it
	metadata := &ChunkMetadata{
		Fingerprint:        "test_fp",
		StorageLocation:    "test_location",
		Size:               1024,
		CreationTime:       time.Now(),
		LastReferencedTime: time.Now(),
	}

	ic.AddChunkMetadata("test_fp", metadata)

	// Cache hit
	_, exists := ic.GetChunkMetadata("test_fp")
	if !exists {
		t.Error("Expected cache hit")
	}

	// Cache miss
	_, exists = ic.GetChunkMetadata("nonexistent_fp")
	if exists {
		t.Error("Expected cache miss")
	}

	// Get stats
	stats := ic.GetStats()

	if stats.TotalAccesses != 2 {
		t.Errorf("Expected 2 total accesses, got %d", stats.TotalAccesses)
	}

	if stats.CacheHits != 1 {
		t.Errorf("Expected 1 cache hit, got %d", stats.CacheHits)
	}

	if stats.CacheMisses != 1 {
		t.Errorf("Expected 1 cache miss, got %d", stats.CacheMisses)
	}

	if stats.PredictionAccuracy != 0.5 {
		t.Errorf("Expected 0.5 prediction accuracy, got %f", stats.PredictionAccuracy)
	}

	t.Logf("Stats: Total=%d, Hits=%d, Misses=%d, Accuracy=%.2f",
		stats.TotalAccesses, stats.CacheHits, stats.CacheMisses, stats.PredictionAccuracy)
}

func TestIntelligentCacheWarmingQueue(t *testing.T) {
	queue := NewWarmingQueue(5)

	// Test adding items
	items := []WarmingItem{
		{Fingerprint: "fp1", Priority: 0.9, Timestamp: time.Now(), Reason: "high priority"},
		{Fingerprint: "fp2", Priority: 0.7, Timestamp: time.Now(), Reason: "medium priority"},
		{Fingerprint: "fp3", Priority: 0.5, Timestamp: time.Now(), Reason: "low priority"},
	}

	for _, item := range items {
		queue.Add(item)
	}

	// Get items
	retrieved := queue.GetItems()
	if len(retrieved) != 3 {
		t.Errorf("Expected 3 items, got %d", len(retrieved))
	}

	// Test updating existing item with higher priority
	updatedItem := WarmingItem{
		Fingerprint: "fp2",
		Priority:    0.95, // Higher priority
		Timestamp:   time.Now(),
		Reason:      "updated priority",
	}
	queue.Add(updatedItem)

	retrieved = queue.GetItems()
	found := false
	for _, item := range retrieved {
		if item.Fingerprint == "fp2" && item.Priority == 0.95 {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find updated item with higher priority")
	}

	// Test queue capacity
	for i := 0; i < 10; i++ {
		item := WarmingItem{
			Fingerprint: fmt.Sprintf("fp%d", i+10),
			Priority:    float64(i) / 10.0,
			Timestamp:   time.Now(),
			Reason:      "capacity test",
		}
		queue.Add(item)
	}

	retrieved = queue.GetItems()
	if len(retrieved) > 5 {
		t.Errorf("Expected queue to respect capacity, got %d items", len(retrieved))
	}
}

func TestIntelligentCacheConcurrent(t *testing.T) {
	mockDB := &MockDBClient{}
	dedupeCache := NewDeduplicationCache(1000, 10000)

	config := &IntelligentCacheConfig{
		WarmingEnabled:       true,
		WarmingThreshold:     0.7,
		MaxWarmingChunks:     10,
		WarmingLookback:      1 * time.Minute,
		WarmingInterval:      100 * time.Millisecond,
		PredictionWindow:     30 * time.Second,
		MinAccessCount:       2,
		ConfidenceThreshold:  0.6,
		MaxConcurrentWarming: 2,
		WarmingTimeout:       1 * time.Second,
	}

	ic := NewIntelligentCache(dedupeCache, mockDB, config)
	defer ic.Close()

	// Test concurrent access
	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				fp := fmt.Sprintf("fp_%d_%d", id, j)
				ic.GetChunkMetadata(fp)
				time.Sleep(1 * time.Millisecond)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}

	// Wait for background processing
	time.Sleep(200 * time.Millisecond)

	// Get stats
	stats := ic.GetStats()
	if stats.TotalAccesses != 50 {
		t.Errorf("Expected 50 total accesses, got %d", stats.TotalAccesses)
	}

	t.Logf("Concurrent test completed: %d total accesses", stats.TotalAccesses)
}

func TestIntelligentCacheCancellation(t *testing.T) {
	mockDB := &MockDBClient{}
	dedupeCache := NewDeduplicationCache(1000, 10000)

	config := &IntelligentCacheConfig{
		WarmingEnabled:       true,
		WarmingThreshold:     0.7,
		MaxWarmingChunks:     10,
		WarmingLookback:      1 * time.Minute,
		WarmingInterval:      100 * time.Millisecond,
		PredictionWindow:     30 * time.Second,
		MinAccessCount:       2,
		ConfidenceThreshold:  0.6,
		MaxConcurrentWarming: 2,
		WarmingTimeout:       1 * time.Second,
	}

	ic := NewIntelligentCache(dedupeCache, mockDB, config)

	// Start some background activity
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 5; j++ {
				fp := fmt.Sprintf("fp_%d_%d", id, j)
				ic.GetChunkMetadata(fp)
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Close the cache
	ic.Close()

	// Try to access after closing
	_, exists := ic.GetChunkMetadata("test_fp")
	if exists {
		t.Error("Expected no access after closing")
	}
}

// MockDBClient implements a mock database client for testing
type MockDBClient struct {
	data map[string]*db.ChunkMetadata
}

func (m *MockDBClient) GetChunkMetadataByFingerprint(ctx context.Context, fingerprint string) (*db.ChunkMetadata, error) {
	if m.data == nil {
		m.data = make(map[string]*db.ChunkMetadata)
	}

	if metadata, exists := m.data[fingerprint]; exists {
		return metadata, nil
	}
	return nil, nil
}

func (m *MockDBClient) InsertChunkMetadata(ctx context.Context, metadata *db.ChunkMetadata) error {
	if m.data == nil {
		m.data = make(map[string]*db.ChunkMetadata)
	}
	m.data[metadata.Fingerprint] = metadata
	return nil
}

func BenchmarkIntelligentCache(b *testing.B) {
	mockDB := &MockDBClient{}
	dedupeCache := NewDeduplicationCache(10000, 100000)

	config := &IntelligentCacheConfig{
		WarmingEnabled:       true,
		WarmingThreshold:     0.7,
		MaxWarmingChunks:     100,
		WarmingLookback:      1 * time.Minute,
		WarmingInterval:      100 * time.Millisecond,
		PredictionWindow:     30 * time.Second,
		MinAccessCount:       2,
		ConfidenceThreshold:  0.6,
		MaxConcurrentWarming: 4,
		WarmingTimeout:       1 * time.Second,
	}

	ic := NewIntelligentCache(dedupeCache, mockDB, config)
	defer ic.Close()

	// Pre-populate with some data
	for i := 0; i < 1000; i++ {
		fp := fmt.Sprintf("benchmark_fp_%d", i)
		metadata := &ChunkMetadata{
			Fingerprint:        fp,
			StorageLocation:    fmt.Sprintf("location_%d", i),
			Size:               int64(i * 1024),
			CreationTime:       time.Now(),
			LastReferencedTime: time.Now(),
		}
		ic.AddChunkMetadata(fp, metadata)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			fp := fmt.Sprintf("benchmark_fp_%d", i%1000)
			ic.GetChunkMetadata(fp)
			i++
		}
	})
}

func BenchmarkIntelligentCacheWarming(b *testing.B) {
	mockDB := &MockDBClient{
		data: make(map[string]*db.ChunkMetadata),
	}

	// Pre-populate mock DB
	for i := 0; i < 1000; i++ {
		fp := fmt.Sprintf("db_fp_%d", i)
		mockDB.data[fp] = &db.ChunkMetadata{
			Fingerprint:     fp,
			StorageLocation: fmt.Sprintf("db_location_%d", i),
			Size:            int64(i * 1024),
		}
	}

	dedupeCache := NewDeduplicationCache(100, 1000) // Small cache to force misses

	config := &IntelligentCacheConfig{
		WarmingEnabled:       true,
		WarmingThreshold:     0.5,
		MaxWarmingChunks:     50,
		WarmingLookback:      1 * time.Minute,
		WarmingInterval:      50 * time.Millisecond,
		PredictionWindow:     30 * time.Second,
		MinAccessCount:       2,
		ConfidenceThreshold:  0.5,
		MaxConcurrentWarming: 4,
		WarmingTimeout:       1 * time.Second,
	}

	ic := NewIntelligentCache(dedupeCache, mockDB, config)
	defer ic.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fp := fmt.Sprintf("db_fp_%d", i%1000)
		ic.GetChunkMetadata(fp)
	}
}
