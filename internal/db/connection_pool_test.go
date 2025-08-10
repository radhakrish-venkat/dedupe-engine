package db

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestConnectionPool(t *testing.T) {
	// Skip if no database connection available
	connStr := getTestConnectionString()
	if connStr == "" {
		t.Skip("No test database connection available")
	}

	// Create connection pool
	pool, err := NewConnectionPool(connStr, 10, 5)
	if err != nil {
		t.Fatalf("Failed to create connection pool: %v", err)
	}
	defer pool.Close()

	// Test basic operations
	db := pool.GetDB()
	if db == nil {
		t.Error("GetDB returned nil")
	}

	// Test ping
	if err := db.Ping(); err != nil {
		t.Errorf("Failed to ping database: %v", err)
	}

	// Test stats
	stats := pool.GetStats()
	if stats.TotalConnections == 0 {
		t.Error("Expected non-zero total connections")
	}
}

func TestBatchProcessor(t *testing.T) {
	// Skip if no database connection available
	connStr := getTestConnectionString()
	if connStr == "" {
		t.Skip("No test database connection available")
	}

	// Create connection pool
	pool, err := NewConnectionPool(connStr, 10, 5)
	if err != nil {
		t.Fatalf("Failed to create connection pool: %v", err)
	}
	defer pool.Close()

	// Create batch processor
	bp := NewBatchProcessor(pool, 5)
	defer bp.Close()

	// Test adding chunks
	chunks := []*ChunkMetadata{
		{
			Fingerprint:        "test_fp_1",
			StorageLocation:    "test_location_1",
			Size:               1024,
			CreationTime:       time.Now(),
			LastReferencedTime: time.Now(),
		},
		{
			Fingerprint:        "test_fp_2",
			StorageLocation:    "test_location_2",
			Size:               2048,
			CreationTime:       time.Now(),
			LastReferencedTime: time.Now(),
		},
	}

	// Add chunks
	for _, chunk := range chunks {
		if err := bp.AddChunk(chunk); err != nil {
			t.Errorf("Failed to add chunk: %v", err)
		}
	}

	// Flush to ensure data is written
	if err := bp.Flush(); err != nil {
		t.Errorf("Failed to flush batch: %v", err)
	}

	// Wait a bit for background operations
	time.Sleep(100 * time.Millisecond)
}

func TestBatchQueryProcessor(t *testing.T) {
	// Skip if no database connection available
	connStr := getTestConnectionString()
	if connStr == "" {
		t.Skip("No test database connection available")
	}

	// Create connection pool
	pool, err := NewConnectionPool(connStr, 10, 5)
	if err != nil {
		t.Fatalf("Failed to create connection pool: %v", err)
	}
	defer pool.Close()

	// Create batch query processor
	bqp := NewBatchQueryProcessor(pool)

	// Test batch query with empty list
	result, err := bqp.GetChunksBatch(context.Background(), []string{})
	if err != nil {
		t.Errorf("Failed to query empty batch: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected empty result, got %d items", len(result))
	}

	// Test batch query with non-existent fingerprints
	result, err = bqp.GetChunksBatch(context.Background(), []string{"non_existent_1", "non_existent_2"})
	if err != nil {
		t.Errorf("Failed to query non-existent batch: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected empty result for non-existent fingerprints, got %d items", len(result))
	}
}

func TestBatchProcessorConcurrent(t *testing.T) {
	// Skip if no database connection available
	connStr := getTestConnectionString()
	if connStr == "" {
		t.Skip("No test database connection available")
	}

	// Create connection pool
	pool, err := NewConnectionPool(connStr, 20, 10)
	if err != nil {
		t.Fatalf("Failed to create connection pool: %v", err)
	}
	defer pool.Close()

	// Create batch processor
	bp := NewBatchProcessor(pool, 10)
	defer bp.Close()

	// Test concurrent adds
	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 20; j++ {
				chunk := &ChunkMetadata{
					Fingerprint:        fmt.Sprintf("concurrent_fp_%d_%d", id, j),
					StorageLocation:    fmt.Sprintf("concurrent_location_%d_%d", id, j),
					Size:               int64(1024 + j),
					CreationTime:       time.Now(),
					LastReferencedTime: time.Now(),
				}
				if err := bp.AddChunk(chunk); err != nil {
					t.Errorf("Failed to add chunk in goroutine %d: %v", id, err)
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 5; i++ {
		<-done
	}

	// Flush to ensure all data is written
	if err := bp.Flush(); err != nil {
		t.Errorf("Failed to flush batch: %v", err)
	}

	// Wait a bit for background operations
	time.Sleep(100 * time.Millisecond)
}

func BenchmarkBatchProcessorAdd(b *testing.B) {
	// Skip if no database connection available
	connStr := getTestConnectionString()
	if connStr == "" {
		b.Skip("No test database connection available")
	}

	// Create connection pool
	pool, err := NewConnectionPool(connStr, 10, 5)
	if err != nil {
		b.Fatalf("Failed to create connection pool: %v", err)
	}
	defer pool.Close()

	// Create batch processor
	bp := NewBatchProcessor(pool, 100)
	defer bp.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chunk := &ChunkMetadata{
			Fingerprint:        fmt.Sprintf("benchmark_fp_%d", i),
			StorageLocation:    fmt.Sprintf("benchmark_location_%d", i),
			Size:               int64(1024 + i),
			CreationTime:       time.Now(),
			LastReferencedTime: time.Now(),
		}
		if err := bp.AddChunk(chunk); err != nil {
			b.Errorf("Failed to add chunk: %v", err)
		}
	}
}

func BenchmarkBatchQueryProcessor(b *testing.B) {
	// Skip if no database connection available
	connStr := getTestConnectionString()
	if connStr == "" {
		b.Skip("No test database connection available")
	}

	// Create connection pool
	pool, err := NewConnectionPool(connStr, 10, 5)
	if err != nil {
		b.Fatalf("Failed to create connection pool: %v", err)
	}
	defer pool.Close()

	// Create batch query processor
	bqp := NewBatchQueryProcessor(pool)

	// Prepare test fingerprints
	fingerprints := make([]string, 100)
	for i := 0; i < 100; i++ {
		fingerprints[i] = fmt.Sprintf("benchmark_query_fp_%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Query a subset of fingerprints
		subset := fingerprints[i%len(fingerprints) : (i%len(fingerprints))+10]
		if len(subset) < 10 {
			subset = fingerprints[:10]
		}

		_, err := bqp.GetChunksBatch(context.Background(), subset)
		if err != nil {
			b.Errorf("Failed to query batch: %v", err)
		}
	}
}

// getTestConnectionString returns a test database connection string
// In a real test environment, this would be configured via environment variables
func getTestConnectionString() string {
	// For testing, we'll use a simple in-memory approach or skip
	// In production, this would read from environment variables
	return "" // Return empty to skip tests that require database
}
