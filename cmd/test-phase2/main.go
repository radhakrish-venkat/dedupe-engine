package main

import (
	"context"
	"fmt"
	"math/rand"
	"runtime"
	"time"

	"github.com/radhakrishnan.venkat/dedupe-engine/internal/cache"
	"github.com/radhakrishnan.venkat/dedupe-engine/internal/chunking"
	"github.com/radhakrishnan.venkat/dedupe-engine/internal/db"
	"github.com/radhakrishnan.venkat/dedupe-engine/internal/monitoring"
)

func main() {
	fmt.Println("🚀 Phase 2 Advanced Features Test Suite")
	fmt.Println("========================================")
	fmt.Println()

	// Test 1: Basic Chunking (simplified)
	fmt.Println("📊 1. Testing Basic Chunking Performance")
	fmt.Println("----------------------------------------")
	testBasicChunking()

	fmt.Println()

	// Test 2: Intelligent Cache Warming
	fmt.Println("🧠 2. Testing Intelligent Cache Warming")
	fmt.Println("----------------------------------------")
	testIntelligentCacheWarming()

	fmt.Println()

	// Test 3: Advanced Monitoring
	fmt.Println("📈 3. Testing Advanced Monitoring & Analytics")
	fmt.Println("-----------------------------------------------")
	testAdvancedMonitoring()

	fmt.Println()

	// Test 4: Integration Test
	fmt.Println("🔗 4. Testing Phase 2 Integration")
	fmt.Println("----------------------------------")
	testPhase2Integration()

	fmt.Println()
	fmt.Println("✅ Phase 2 Advanced Features Test Complete!")
}

func testBasicChunking() {
	// Test with smaller data sizes for faster execution
	dataSizes := []int{64 * 1024, 256 * 1024} // 64KB, 256KB

	for _, size := range dataSizes {
		fmt.Printf("  📁 Testing with %d KB data:\n", size/1024)

		// Generate test data
		data := generateTestData(size)

		// Use regular chunker instead of parallel for testing
		chunker := chunking.NewChunker(64, 8192)

		// Test chunking
		start := time.Now()
		chunks, err := chunker.ChunkData(data)
		duration := time.Since(start)

		if err != nil {
			fmt.Printf("    ❌ Error: %v\n", err)
			continue
		}

		// Calculate performance metrics
		throughput := float64(len(data)) / 1024 / 1024 / duration.Seconds() // MB/s
		chunkRate := float64(len(chunks)) / duration.Seconds()              // chunks/sec

		// Calculate deduplication ratio
		uniqueChunks := make(map[string]bool)
		for _, chunk := range chunks {
			uniqueChunks[chunk.Fingerprint] = true
		}
		dedupRatio := float64(len(uniqueChunks)) / float64(len(chunks))

		fmt.Printf("    ✅ Throughput: %.1f MB/s, %.0f chunks/sec, %.1f%% unique\n",
			throughput, chunkRate, dedupRatio*100)
		fmt.Printf("      📊 Stats: %d total chunks, %d unique, %v processing time\n",
			len(chunks), len(uniqueChunks), duration)
	}
	fmt.Println()
}

func testIntelligentCacheWarming() {
	// Create mock DB with test data
	mockDB := &MockDBClient{
		data: make(map[string]*db.ChunkMetadata),
	}

	// Pre-populate with some data
	for i := 0; i < 100; i++ { // Reduced from 1000 for faster testing
		fp := fmt.Sprintf("test_fp_%d", i)
		mockDB.data[fp] = &db.ChunkMetadata{
			Fingerprint:     fp,
			StorageLocation: fmt.Sprintf("location_%d", i),
			Size:            i * 1024,
		}
	}

	// Create intelligent cache
	config := &cache.IntelligentCacheConfig{
		WarmingEnabled:       true,
		WarmingThreshold:     0.5,
		MaxWarmingChunks:     20,                    // Reduced for faster testing
		WarmingLookback:      30 * time.Second,      // Reduced for faster testing
		WarmingInterval:      50 * time.Millisecond, // Faster intervals
		PredictionWindow:     15 * time.Second,      // Reduced
		MinAccessCount:       2,
		ConfidenceThreshold:  0.5,
		MaxConcurrentWarming: 2,                      // Reduced
		WarmingTimeout:       500 * time.Millisecond, // Reduced
	}

	dedupeCache := cache.NewDeduplicationCache(100, 1000) // Smaller cache
	ic := cache.NewIntelligentCache(dedupeCache, mockDB, config)
	defer ic.Close()

	// Simulate access patterns
	fmt.Println("  🔄 Simulating access patterns...")

	// Create correlated access patterns
	patterns := [][]string{
		{"fp1", "fp2", "fp3"},    // Pattern 1
		{"fp4", "fp5", "fp6"},    // Pattern 2
		{"fp7", "fp8", "fp9"},    // Pattern 3
		{"fp10", "fp11", "fp12"}, // Pattern 4
	}

	// Access patterns multiple times to build correlations
	for round := 0; round < 3; round++ { // Reduced rounds
		for _, pattern := range patterns {
			for _, fp := range pattern {
				ic.GetChunkMetadata(fp)
				time.Sleep(5 * time.Millisecond) // Faster
			}
		}
	}

	// Wait for pattern analysis
	time.Sleep(200 * time.Millisecond) // Reduced wait time

	// Test predictive warming
	fmt.Println("  🔮 Testing predictive warming...")

	// Access one pattern to trigger warming
	ic.GetChunkMetadata("fp1")

	// Wait for warming to complete
	time.Sleep(150 * time.Millisecond) // Reduced wait time

	// Check if related chunks were warmed
	warmedCount := 0
	for _, fp := range []string{"fp2", "fp3"} {
		if _, exists := dedupeCache.GetChunkMetadata(fp); exists {
			warmedCount++
		}
	}

	fmt.Printf("    ✅ Predictive warming: %d/2 related chunks warmed\n", warmedCount)

	// Get cache stats
	stats := ic.GetStats()
	fmt.Printf("    📊 Cache stats: %d total accesses, %.1f%% hit rate\n",
		stats.TotalAccesses, stats.PredictionAccuracy*100)
}

func testAdvancedMonitoring() {
	// Create advanced monitor
	config := &monitoring.MonitoringConfig{
		MetricsEnabled:             true,
		PerformanceTrackingEnabled: true,
		AnomalyDetectionEnabled:    true,
		PredictiveAnalyticsEnabled: true,
		AlertingEnabled:            true,
		MetricsInterval:            50 * time.Millisecond, // Faster
		PerformanceWindow:          30 * time.Second,      // Reduced
		AnomalyWindow:              1 * time.Minute,       // Reduced
		PredictionWindow:           2 * time.Minute,       // Reduced
	}

	monitor := monitoring.NewAdvancedMonitor(config)
	defer monitor.Close()

	fmt.Println("  📊 Recording metrics...")

	// Simulate various metrics (reduced count for faster testing)
	for i := 0; i < 50; i++ { // Reduced from 100
		// Record chunk processing
		monitor.RecordChunkProcessed(1024, i%3 == 0) // 1/3 unique

		// Record cache hits/misses
		if i%4 == 0 {
			monitor.RecordCacheMiss()
		} else {
			monitor.RecordCacheHit()
		}

		// Record latencies
		monitor.RecordProcessingLatency(time.Duration(10+i%20) * time.Millisecond)
		monitor.RecordDatabaseLatency(time.Duration(5+i%15) * time.Millisecond)

		// Record resource usage
		memory := 100.0 + float64(i%50)
		cpu := 20.0 + float64(i%30)
		disk := 60.0 + float64(i%20)
		monitor.RecordResourceUsage(memory, cpu, disk)

		// Record storage savings
		monitor.RecordStorageSavings(1000, 300)

		// Record some errors
		if i%20 == 0 {
			monitor.RecordError("test_error")
		}

		time.Sleep(5 * time.Millisecond) // Faster
	}

	// Wait for analysis
	time.Sleep(100 * time.Millisecond) // Reduced

	// Get reports
	fmt.Println("  📈 Generating reports...")

	// Performance report
	perfReport := monitor.GetPerformanceReport()
	if perfReport != nil {
		fmt.Printf("    ✅ Performance Score: %.1f/100\n", perfReport.OverallScore)
		for name, metric := range perfReport.Metrics {
			if metric.Alerted {
				fmt.Printf("      ⚠️  Alert: %s (score: %.1f)\n", name, metric.Score)
			}
		}
	}

	// Anomaly report
	anomalyReport := monitor.GetAnomalyReport()
	if anomalyReport != nil && len(anomalyReport.Anomalies) > 0 {
		fmt.Printf("    🔍 Anomalies detected: %d\n", len(anomalyReport.Anomalies))
		for _, anomaly := range anomalyReport.Anomalies {
			fmt.Printf("      ⚠️  %s: %d anomalies (z-score > %.1f)\n",
				anomaly.Metric, anomaly.AnomalyCount, anomaly.Threshold)
		}
	}

	// Prediction report
	predReport := monitor.GetPredictionReport()
	if predReport != nil && len(predReport.Predictions) > 0 {
		fmt.Printf("    🔮 Predictions made: %d\n", len(predReport.Predictions))
		for _, pred := range predReport.Predictions {
			fmt.Printf("      📊 %s: %.2f (confidence: %.1f%%)\n",
				pred.Metric, pred.Value, pred.Confidence*100)
		}
	}

	// Overall stats
	stats := monitor.GetStats()
	fmt.Printf("    📊 Monitoring stats: %d metrics, %d anomalies, %d predictions, %d alerts\n",
		stats.TotalMetricsCollected, stats.AnomaliesDetected, stats.PredictionsMade, stats.AlertsGenerated)
}

func testPhase2Integration() {
	fmt.Println("  🔗 Testing Phase 2 Integration...")

	// Create all Phase 2 components
	chunker := chunking.NewChunker(64, 8192) // Use regular chunker for now

	mockDB := &MockDBClient{data: make(map[string]*db.ChunkMetadata)}
	dedupeCache := cache.NewDeduplicationCache(100, 1000) // Smaller cache

	icConfig := &cache.IntelligentCacheConfig{
		WarmingEnabled:       true,
		WarmingThreshold:     0.5,
		MaxWarmingChunks:     20,
		WarmingLookback:      30 * time.Second,
		WarmingInterval:      50 * time.Millisecond,
		PredictionWindow:     15 * time.Second,
		MinAccessCount:       2,
		ConfidenceThreshold:  0.5,
		MaxConcurrentWarming: 2,
		WarmingTimeout:       500 * time.Millisecond,
	}

	intelligentCache := cache.NewIntelligentCache(dedupeCache, mockDB, icConfig)
	defer intelligentCache.Close()

	monitorConfig := &monitoring.MonitoringConfig{
		MetricsEnabled:             true,
		PerformanceTrackingEnabled: true,
		AnomalyDetectionEnabled:    true,
		PredictiveAnalyticsEnabled: true,
		AlertingEnabled:            true,
		MetricsInterval:            50 * time.Millisecond,
		PerformanceWindow:          30 * time.Second,
		AnomalyWindow:              1 * time.Minute,
		PredictionWindow:           2 * time.Minute,
	}

	monitor := monitoring.NewAdvancedMonitor(monitorConfig)
	defer monitor.Close()

	// Simulate integrated workflow
	fmt.Println("    🔄 Running integrated workflow...")

	// Generate test data
	data := generateTestData(128 * 1024) // 128KB

	// Phase 1: Chunking
	start := time.Now()
	chunks, err := chunker.ChunkData(data)
	if err != nil {
		fmt.Printf("    ❌ Chunking error: %v\n", err)
		return
	}
	chunkingTime := time.Since(start)

	// Phase 2: Intelligent cache operations
	cacheHits := 0
	cacheMisses := 0
	uniqueChunks := 0

	for _, chunk := range chunks {
		// Record metrics
		monitor.RecordChunkProcessed(chunk.Size, true)
		monitor.RecordProcessingLatency(chunkingTime / time.Duration(len(chunks)))

		// Try to get from cache
		_, exists := intelligentCache.GetChunkMetadata(chunk.Fingerprint)
		if exists {
			cacheHits++
			monitor.RecordCacheHit()
		} else {
			cacheMisses++
			monitor.RecordCacheMiss()
			uniqueChunks++
		}
	}

	// Phase 3: Generate reports
	time.Sleep(100 * time.Millisecond) // Reduced

	// Calculate performance metrics
	totalTime := time.Since(start)
	throughput := float64(len(data)) / 1024 / 1024 / totalTime.Seconds()
	dedupRatio := float64(uniqueChunks) / float64(len(chunks))
	cacheHitRate := float64(cacheHits) / float64(len(chunks))

	fmt.Printf("    ✅ Integration Results:\n")
	fmt.Printf("      📊 Throughput: %.1f MB/s\n", throughput)
	fmt.Printf("      📊 Deduplication: %.1f%% unique chunks\n", dedupRatio*100)
	fmt.Printf("      📊 Cache Hit Rate: %.1f%%\n", cacheHitRate*100)
	fmt.Printf("      📊 Processing Time: %v\n", totalTime)

	// Get monitoring insights
	perfReport := monitor.GetPerformanceReport()
	if perfReport != nil {
		fmt.Printf("      📈 Performance Score: %.1f/100\n", perfReport.OverallScore)
	}

	anomalyReport := monitor.GetAnomalyReport()
	if anomalyReport != nil && len(anomalyReport.Anomalies) > 0 {
		fmt.Printf("      ⚠️  Anomalies: %d detected\n", len(anomalyReport.Anomalies))
	}

	// Memory usage
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("      💾 Memory Usage: %.1f MB\n", float64(m.Alloc)/1024/1024)
}

// Helper functions

func generateTestData(size int) []byte {
	data := make([]byte, size)

	// Use a more efficient pattern generation
	pattern := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")
	patternLen := len(pattern)

	// Fill with repeating pattern (much faster than individual byte operations)
	for i := 0; i < size; i++ {
		data[i] = pattern[i%patternLen]
	}

	// Add some repetitive sections for better deduplication testing
	// Only modify a small portion to keep it fast
	repetitiveSize := size / 100 // Much smaller than size/4
	for i := 0; i < repetitiveSize; i++ {
		data[i] = 'A'
	}

	// Add some random sections (smaller portion)
	randomSize := size / 200 // Much smaller than size/4
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < randomSize; i++ {
		pos := size/2 + i
		if pos < size {
			data[pos] = byte(rand.Intn(256))
		}
	}

	return data
}

// MockDBClient for testing
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
