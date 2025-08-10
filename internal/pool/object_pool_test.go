package pool

import (
	"testing"
	"time"
)

func TestObjectPool(t *testing.T) {
	// Create a simple object pool
	pool := NewObjectPool(func() interface{} {
		return "new_object"
	}, 10)

	// Test basic get/put operations
	obj1 := pool.Get()
	if obj1 == nil {
		t.Error("Get returned nil")
	}

	pool.Put(obj1)

	// Test that we can get the same object back
	obj2 := pool.Get()
	if obj2 == nil {
		t.Error("Get returned nil after Put")
	}

	// Test stats
	stats := pool.GetStats()
	if stats.TotalAllocated < 1 {
		t.Error("Expected at least 1 allocation")
	}
}

func TestChunkPool(t *testing.T) {
	pool := NewChunkPool(10)

	// Test getting chunks
	chunk1 := pool.GetChunk()
	if chunk1 == nil {
		t.Error("GetChunk returned nil")
	}

	// Test that chunk has pre-allocated buffer
	if cap(chunk1.Data) < 8192 {
		t.Errorf("Expected buffer capacity >= 8192, got %d", cap(chunk1.Data))
	}

	// Use the chunk
	chunk1.Data = append(chunk1.Data, []byte("test data")...)
	chunk1.Fingerprint = "test_fp"
	chunk1.Offset = 100
	chunk1.Size = 9

	// Return to pool
	pool.PutChunk(chunk1)

	// Get another chunk (should be the same one)
	chunk2 := pool.GetChunk()
	if chunk2 == nil {
		t.Error("GetChunk returned nil after PutChunk")
	}

	// Verify chunk was reset
	if len(chunk2.Data) != 0 {
		t.Error("Chunk data was not reset")
	}
	if chunk2.Fingerprint != "" {
		t.Error("Chunk fingerprint was not reset")
	}
	if chunk2.Offset != 0 {
		t.Error("Chunk offset was not reset")
	}
	if chunk2.Size != 0 {
		t.Error("Chunk size was not reset")
	}

	// Verify buffer capacity is preserved
	if cap(chunk2.Data) < 8192 {
		t.Errorf("Expected buffer capacity >= 8192, got %d", cap(chunk2.Data))
	}
}

func TestMetadataPool(t *testing.T) {
	pool := NewMetadataPool(10)

	// Test getting metadata
	metadata1 := pool.GetMetadata()
	if metadata1 == nil {
		t.Error("GetMetadata returned nil")
	}

	// Use the metadata
	metadata1.Fingerprint = "test_fp"
	metadata1.StorageLocation = "test_location"
	metadata1.Size = 1024
	metadata1.CreationTime = time.Now()
	metadata1.LastReferencedTime = time.Now()

	// Return to pool
	pool.PutMetadata(metadata1)

	// Get another metadata (should be the same one)
	metadata2 := pool.GetMetadata()
	if metadata2 == nil {
		t.Error("GetMetadata returned nil after PutMetadata")
	}

	// Verify metadata was reset
	if metadata2.Fingerprint != "" {
		t.Error("Metadata fingerprint was not reset")
	}
	if metadata2.StorageLocation != "" {
		t.Error("Metadata storage location was not reset")
	}
	if metadata2.Size != 0 {
		t.Error("Metadata size was not reset")
	}
	if !metadata2.CreationTime.IsZero() {
		t.Error("Metadata creation time was not reset")
	}
	if !metadata2.LastReferencedTime.IsZero() {
		t.Error("Metadata last referenced time was not reset")
	}
}

func TestBufferPool(t *testing.T) {
	pool := NewBufferPool(1024, 10)

	// Test getting buffer
	buffer1 := pool.GetBuffer()
	if buffer1 == nil {
		t.Error("GetBuffer returned nil")
	}

	// Test that buffer has correct initial capacity
	if cap(buffer1) < 1024 {
		t.Errorf("Expected buffer capacity >= 1024, got %d", cap(buffer1))
	}

	// Use the buffer
	buffer1 = append(buffer1, []byte("test data")...)

	// Return to pool
	pool.PutBuffer(buffer1)

	// Get another buffer (should be the same one)
	buffer2 := pool.GetBuffer()
	if buffer2 == nil {
		t.Error("GetBuffer returned nil after PutBuffer")
	}

	// Verify buffer was reset
	if len(buffer2) != 0 {
		t.Error("Buffer was not reset")
	}

	// Verify capacity is preserved
	if cap(buffer2) < 1024 {
		t.Errorf("Expected buffer capacity >= 1024, got %d", cap(buffer2))
	}
}

func TestObjectPoolConcurrent(t *testing.T) {
	pool := NewObjectPool(func() interface{} {
		return make([]byte, 0, 1024)
	}, 100)

	// Test concurrent access
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				obj := pool.Get()
				if obj == nil {
					t.Errorf("Get returned nil in goroutine %d", id)
					continue
				}

				// Use the object
				buffer := obj.([]byte)
				buffer = append(buffer, []byte("test")...)

				// Return to pool
				pool.Put(buffer)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Check stats
	stats := pool.GetStats()
	if stats.TotalAllocated == 0 {
		t.Error("Expected some allocations")
	}
}

func TestGlobalPools(t *testing.T) {
	// Test that global pools are created correctly
	pools := GetGlobalPools()
	if pools == nil {
		t.Error("GetGlobalPools returned nil")
	}

	if pools.ChunkPool == nil {
		t.Error("ChunkPool is nil")
	}

	if pools.MetadataPool == nil {
		t.Error("MetadataPool is nil")
	}

	if pools.BufferPool == nil {
		t.Error("BufferPool is nil")
	}

	// Test that we get the same instance
	pools2 := GetGlobalPools()
	if pools != pools2 {
		t.Error("GetGlobalPools returned different instances")
	}
}

func TestPoolReset(t *testing.T) {
	pool := NewObjectPool(func() interface{} {
		return "new_object"
	}, 10)

	// Add some objects to the pool
	for i := 0; i < 5; i++ {
		obj := pool.Get()
		pool.Put(obj)
	}

	// Check initial stats
	stats1 := pool.GetStats()
	if stats1.TotalAllocated == 0 {
		t.Error("Expected some allocations before reset")
	}

	// Reset the pool
	pool.Reset()

	// Check stats after reset
	stats2 := pool.GetStats()
	if stats2.TotalAllocated != 0 {
		t.Error("Expected zero allocations after reset")
	}
	if stats2.TotalReused != 0 {
		t.Error("Expected zero reuses after reset")
	}
	if stats2.TotalReturned != 0 {
		t.Error("Expected zero returns after reset")
	}
}

func BenchmarkObjectPool(b *testing.B) {
	pool := NewObjectPool(func() interface{} {
		return make([]byte, 0, 1024)
	}, 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		obj := pool.Get()
		buffer := obj.([]byte)
		buffer = append(buffer, []byte("test")...)
		pool.Put(buffer)
	}
}

func BenchmarkChunkPool(b *testing.B) {
	pool := NewChunkPool(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chunk := pool.GetChunk()
		chunk.Data = append(chunk.Data, []byte("test data")...)
		chunk.Fingerprint = "test_fp"
		chunk.Offset = int64(i)
		chunk.Size = int64(len(chunk.Data))
		pool.PutChunk(chunk)
	}
}

func BenchmarkMetadataPool(b *testing.B) {
	pool := NewMetadataPool(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metadata := pool.GetMetadata()
		metadata.Fingerprint = "test_fp"
		metadata.StorageLocation = "test_location"
		metadata.Size = int64(i)
		metadata.CreationTime = time.Now()
		metadata.LastReferencedTime = time.Now()
		pool.PutMetadata(metadata)
	}
}

func BenchmarkBufferPool(b *testing.B) {
	pool := NewBufferPool(1024, 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buffer := pool.GetBuffer()
		buffer = append(buffer, []byte("test data")...)
		pool.PutBuffer(buffer)
	}
}
