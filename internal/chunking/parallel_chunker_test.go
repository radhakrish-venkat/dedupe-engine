package chunking

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"
)

func TestParallelChunkerBasic(t *testing.T) {
	chunker := NewParallelChunker(64, 8192, 4)
	defer chunker.Close()

	// Test with small data
	data := []byte("This is a test string for chunking. It should be processed correctly.")
	chunks, err := chunker.ChunkDataParallel(data)
	if err != nil {
		t.Fatalf("Failed to chunk data: %v", err)
	}

	if len(chunks) == 0 {
		t.Error("Expected at least one chunk")
	}

	// Verify chunks
	for i, chunk := range chunks {
		if chunk.Size == 0 {
			t.Errorf("Chunk %d has zero size", i)
		}
		if chunk.Fingerprint == "" {
			t.Errorf("Chunk %d has empty fingerprint", i)
		}
		if chunk.Offset < 0 {
			t.Errorf("Chunk %d has negative offset", i)
		}
	}
}

func TestParallelChunkerLargeData(t *testing.T) {
	chunker := NewParallelChunker(64, 8192, 4)
	defer chunker.Close()

	// Generate large test data
	data := generateLargeTestData(1024 * 1024) // 1MB

	start := time.Now()
	chunks, err := chunker.ChunkDataParallel(data)
	processingTime := time.Since(start)

	if err != nil {
		t.Fatalf("Failed to chunk large data: %v", err)
	}

	if len(chunks) == 0 {
		t.Error("Expected chunks from large data")
	}

	// Verify total size
	var totalSize int64
	for _, chunk := range chunks {
		totalSize += chunk.Size
	}

	if totalSize != int64(len(data)) {
		t.Errorf("Total chunk size %d doesn't match input size %d", totalSize, len(data))
	}

	t.Logf("Processed %d chunks in %v (%.0f MB/s)", len(chunks), processingTime, float64(len(data))/1024/1024/processingTime.Seconds())
}

func TestParallelChunkerStreaming(t *testing.T) {
	chunker := NewParallelChunker(64, 8192, 4)
	defer chunker.Close()

	// Create test data
	data := generateLargeTestData(512 * 1024) // 512KB
	reader := bytes.NewReader(data)

	// Test streaming
	chunkChannel, err := chunker.ChunkStream(reader)
	if err != nil {
		t.Fatalf("Failed to start streaming: %v", err)
	}

	var chunks []Chunk
	for chunk := range chunkChannel {
		chunks = append(chunks, chunk)
	}

	if len(chunks) == 0 {
		t.Error("Expected chunks from streaming")
	}

	// Verify total size
	var totalSize int64
	for _, chunk := range chunks {
		totalSize += chunk.Size
	}

	if totalSize != int64(len(data)) {
		t.Errorf("Total chunk size %d doesn't match input size %d", totalSize, len(data))
	}
}

func TestParallelChunkerConcurrent(t *testing.T) {
	chunker := NewParallelChunker(64, 8192, 8)
	defer chunker.Close()

	// Test concurrent chunking
	done := make(chan bool, 4)
	for i := 0; i < 4; i++ {
		go func(id int) {
			data := generateLargeTestData(256 * 1024) // 256KB per goroutine
			chunks, err := chunker.ChunkDataParallel(data)
			if err != nil {
				t.Errorf("Goroutine %d failed: %v", id, err)
			} else if len(chunks) == 0 {
				t.Errorf("Goroutine %d got no chunks", id)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 4; i++ {
		<-done
	}
}

func TestParallelChunkerBoundaryDetection(t *testing.T) {
	chunker := NewParallelChunker(64, 8192, 4)
	defer chunker.Close()

	// Test data with known boundaries
	testCases := []struct {
		name     string
		data     []byte
		expected int // Expected number of chunks
	}{
		{
			name:     "repetitive_data",
			data:     bytes.Repeat([]byte("A"), 10000),
			expected: 1, // Should be one chunk due to repetition
		},
		{
			name:     "random_data",
			data:     generateRandomData(10000),
			expected: 2, // Should be multiple chunks
		},
		{
			name:     "mixed_data",
			data:     generateMixedData(10000),
			expected: 3, // Should be multiple chunks
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			chunks, err := chunker.ChunkDataParallel(tc.data)
			if err != nil {
				t.Fatalf("Failed to chunk %s: %v", tc.name, err)
			}

			if len(chunks) < 1 {
				t.Errorf("Expected at least 1 chunk for %s, got %d", tc.name, len(chunks))
			}

			// Verify total size
			var totalSize int64
			for _, chunk := range chunks {
				totalSize += chunk.Size
			}

			if totalSize != int64(len(tc.data)) {
				t.Errorf("Total chunk size %d doesn't match input size %d for %s", totalSize, len(tc.data), tc.name)
			}
		})
	}
}

func TestParallelChunkerStats(t *testing.T) {
	chunker := NewParallelChunker(64, 8192, 4)
	defer chunker.Close()

	// Reset stats
	chunker.ResetStats()

	// Process some data
	data := generateLargeTestData(1024 * 1024) // 1MB
	_, err := chunker.ChunkDataParallel(data)
	if err != nil {
		t.Fatalf("Failed to chunk data: %v", err)
	}

	// Get stats
	stats := chunker.GetStats()

	if stats.TotalChunks == 0 {
		t.Error("Expected non-zero total chunks")
	}

	if stats.TotalBytes == 0 {
		t.Error("Expected non-zero total bytes")
	}

	if stats.ProcessingTime == 0 {
		t.Error("Expected non-zero processing time")
	}

	if stats.UniqueChunks == 0 {
		t.Error("Expected non-zero unique chunks")
	}

	t.Logf("Stats: Total=%d, Unique=%d, Bytes=%d, Time=%v",
		stats.TotalChunks, stats.UniqueChunks, stats.TotalBytes, stats.ProcessingTime)
}

func TestWorkStealingQueue(t *testing.T) {
	queue := NewWorkStealingQueue(10)

	// Test push and pop
	work := ChunkWork{
		Data:      []byte("test"),
		Offset:    100,
		WorkerID:  1,
		Priority:  5,
		Timestamp: time.Now(),
	}

	// Push work
	if !queue.Push(work) {
		t.Error("Failed to push work")
	}

	// Pop work
	popped, ok := queue.Pop()
	if !ok {
		t.Error("Failed to pop work")
	}

	if !bytes.Equal(popped.Data, work.Data) {
		t.Error("Popped data doesn't match pushed data")
	}

	if popped.Offset != work.Offset {
		t.Error("Popped offset doesn't match pushed offset")
	}

	// Test empty queue
	_, ok = queue.Pop()
	if ok {
		t.Error("Should not be able to pop from empty queue")
	}

	// Test full queue
	for i := 0; i < 10; i++ {
		work.Data = []byte(fmt.Sprintf("work_%d", i))
		if !queue.Push(work) {
			t.Errorf("Failed to push work %d", i)
		}
	}

	// Try to push to full queue
	if queue.Push(work) {
		t.Error("Should not be able to push to full queue")
	}
}

func TestParallelChunkerDifferentSizes(t *testing.T) {
	chunker := NewParallelChunker(64, 8192, 4)
	defer chunker.Close()

	testSizes := []int{
		64,      // Minimum size
		1024,    // 1KB
		10240,   // 10KB
		102400,  // 100KB
		1024000, // 1MB
	}

	for _, size := range testSizes {
		t.Run(fmt.Sprintf("size_%d", size), func(t *testing.T) {
			data := generateLargeTestData(size)
			chunks, err := chunker.ChunkDataParallel(data)
			if err != nil {
				t.Fatalf("Failed to chunk data of size %d: %v", size, err)
			}

			if len(chunks) == 0 {
				t.Errorf("Expected chunks for size %d", size)
			}

			// Verify total size
			var totalSize int64
			for _, chunk := range chunks {
				totalSize += chunk.Size
			}

			if totalSize != int64(len(data)) {
				t.Errorf("Total chunk size %d doesn't match input size %d for size %d", totalSize, len(data), size)
			}
		})
	}
}

func TestParallelChunkerCancellation(t *testing.T) {
	chunker := NewParallelChunker(64, 8192, 4)
	defer chunker.Close()

	// Create a large dataset
	data := generateLargeTestData(10 * 1024 * 1024) // 10MB

	// Start chunking in a goroutine
	done := make(chan bool)
	go func() {
		_, err := chunker.ChunkDataParallel(data)
		if err != nil {
			t.Logf("Chunking error (expected): %v", err)
		}
		done <- true
	}()

	// Cancel after a short delay
	time.Sleep(10 * time.Millisecond)
	chunker.Close()

	// Wait for completion
	select {
	case <-done:
		// Expected
	case <-time.After(1 * time.Second):
		t.Error("Chunking didn't cancel within timeout")
	}
}

// Helper functions

func generateLargeTestData(size int) []byte {
	data := make([]byte, size)

	// Fill with some pattern
	for i := 0; i < size; i++ {
		data[i] = byte(i % 256)
	}

	// Add some repetitive sections
	for i := 0; i < size/4; i++ {
		data[i] = 'A'
	}

	return data
}

func generateRandomData(size int) []byte {
	data := make([]byte, size)
	rand.Read(data)
	return data
}

func generateMixedData(size int) []byte {
	var builder strings.Builder

	// Add repetitive section
	for i := 0; i < size/3; i++ {
		builder.WriteByte('A')
	}

	// Add random section
	randomData := make([]byte, size/3)
	rand.Read(randomData)
	builder.Write(randomData)

	// Add another repetitive section
	for i := 0; i < size/3; i++ {
		builder.WriteByte('B')
	}

	return []byte(builder.String())
}

func BenchmarkParallelChunker(b *testing.B) {
	chunker := NewParallelChunker(64, 8192, 4)
	defer chunker.Close()

	data := generateLargeTestData(1024 * 1024) // 1MB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := chunker.ChunkDataParallel(data)
		if err != nil {
			b.Fatalf("Benchmark failed: %v", err)
		}
	}
}

func BenchmarkParallelChunkerStreaming(b *testing.B) {
	chunker := NewParallelChunker(64, 8192, 4)
	defer chunker.Close()

	data := generateLargeTestData(1024 * 1024) // 1MB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(data)
		chunkChannel, err := chunker.ChunkStream(reader)
		if err != nil {
			b.Fatalf("Failed to start streaming: %v", err)
		}

		// Consume all chunks
		for range chunkChannel {
			// Just consume
		}
	}
}

func BenchmarkParallelChunkerConcurrent(b *testing.B) {
	chunker := NewParallelChunker(64, 8192, 8)
	defer chunker.Close()

	data := generateLargeTestData(256 * 1024) // 256KB per goroutine

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := chunker.ChunkDataParallel(data)
			if err != nil {
				b.Fatalf("Concurrent benchmark failed: %v", err)
			}
		}
	})
}
