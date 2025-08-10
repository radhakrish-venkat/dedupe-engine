package main

import (
	"fmt"
	"time"

	"github.com/radhakrishnan.venkat/dedupe-engine/internal/cache"
	"github.com/radhakrishnan.venkat/dedupe-engine/internal/pool"
)

func main() {
	fmt.Println("🚀 Phase 1 Critical Fixes - Performance Test")
	fmt.Println("=============================================")

	// Test 1: Cuckoo Filter Performance
	testCuckooFilterPerformance()

	// Test 2: Object Pool Performance
	testObjectPoolPerformance()

	// Test 3: Memory Efficiency
	testMemoryEfficiency()

	fmt.Println("\n✅ Phase 1 tests completed successfully!")
}

func testCuckooFilterPerformance() {
	fmt.Println("\n📊 Testing Cuckoo Filter Performance...")

	// Create new Cuckoo filter
	cf := cache.NewCuckooFilter(100000, 0.01)

	// Test insertion performance
	start := time.Now()
	for i := 0; i < 100000; i++ {
		fp := fmt.Sprintf("test_fingerprint_%d", i)
		cf.Add(fp)
	}
	insertTime := time.Since(start)

	// Test lookup performance
	start = time.Now()
	hits := 0
	for i := 0; i < 100000; i++ {
		fp := fmt.Sprintf("test_fingerprint_%d", i)
		if cf.Contains(fp) {
			hits++
		}
	}
	lookupTime := time.Since(start)

	// Test false positive rate
	falsePositives := 0
	for i := 0; i < 10000; i++ {
		fp := fmt.Sprintf("non_existent_fp_%d", i)
		if cf.Contains(fp) {
			falsePositives++
		}
	}
	falsePositiveRate := float64(falsePositives) / 10000.0

	// Get stats
	stats := cf.GetStats()

	fmt.Printf("  ✅ Insertion: %d fingerprints in %v (%.0f ops/sec)\n",
		100000, insertTime, float64(100000)/insertTime.Seconds())
	fmt.Printf("  ✅ Lookup: %d hits in %v (%.0f ops/sec)\n",
		hits, lookupTime, float64(100000)/lookupTime.Seconds())
	fmt.Printf("  ✅ False Positive Rate: %.4f%% (target: <1%%)\n",
		falsePositiveRate*100)
	fmt.Printf("  ✅ Load Factor: %.2f%%\n", stats["load_factor"].(float64)*100)
}

func testObjectPoolPerformance() {
	fmt.Println("\n📊 Testing Object Pool Performance...")

	// Test chunk pool
	chunkPool := pool.NewChunkPool(1000)

	start := time.Now()
	for i := 0; i < 100000; i++ {
		chunk := chunkPool.GetChunk()
		chunk.Data = append(chunk.Data, []byte("test data")...)
		chunk.Fingerprint = fmt.Sprintf("fp_%d", i)
		chunk.Offset = int64(i)
		chunk.Size = int64(len(chunk.Data))
		chunkPool.PutChunk(chunk)
	}
	chunkPoolTime := time.Since(start)

	// Test metadata pool
	metadataPool := pool.NewMetadataPool(1000)

	start = time.Now()
	for i := 0; i < 100000; i++ {
		metadata := metadataPool.GetMetadata()
		metadata.Fingerprint = fmt.Sprintf("fp_%d", i)
		metadata.StorageLocation = fmt.Sprintf("location_%d", i)
		metadata.Size = int64(1024 + i)
		metadata.CreationTime = time.Now()
		metadata.LastReferencedTime = time.Now()
		metadataPool.PutMetadata(metadata)
	}
	metadataPoolTime := time.Since(start)

	// Test buffer pool
	bufferPool := pool.NewBufferPool(8192, 1000)

	start = time.Now()
	for i := 0; i < 100000; i++ {
		buffer := bufferPool.GetBuffer()
		buffer = append(buffer, []byte("test data")...)
		bufferPool.PutBuffer(buffer)
	}
	bufferPoolTime := time.Since(start)

	fmt.Printf("  ✅ Chunk Pool: %d operations in %v (%.0f ops/sec)\n",
		100000, chunkPoolTime, float64(100000)/chunkPoolTime.Seconds())
	fmt.Printf("  ✅ Metadata Pool: %d operations in %v (%.0f ops/sec)\n",
		100000, metadataPoolTime, float64(100000)/metadataPoolTime.Seconds())
	fmt.Printf("  ✅ Buffer Pool: %d operations in %v (%.0f ops/sec)\n",
		100000, bufferPoolTime, float64(100000)/bufferPoolTime.Seconds())

	// Get pool stats
	chunkStats := chunkPool.GetStats()
	metadataStats := metadataPool.GetStats()
	bufferStats := bufferPool.GetStats()

	fmt.Printf("  ✅ Chunk Pool Reuse Rate: %.1f%%\n",
		float64(chunkStats.TotalReused)/float64(chunkStats.TotalAllocated)*100)
	fmt.Printf("  ✅ Metadata Pool Reuse Rate: %.1f%%\n",
		float64(metadataStats.TotalReused)/float64(metadataStats.TotalAllocated)*100)
	fmt.Printf("  ✅ Buffer Pool Reuse Rate: %.1f%%\n",
		float64(bufferStats.TotalReused)/float64(bufferStats.TotalAllocated)*100)
}

func testMemoryEfficiency() {
	fmt.Println("\n📊 Testing Memory Efficiency...")

	// Test global pools
	globalPools := pool.GetGlobalPools()

	// Simulate heavy workload
	start := time.Now()
	for i := 0; i < 50000; i++ {
		// Get objects from pools
		chunk := globalPools.ChunkPool.GetChunk()
		metadata := globalPools.MetadataPool.GetMetadata()
		buffer := globalPools.BufferPool.GetBuffer()

		// Use objects
		chunk.Data = append(chunk.Data, []byte("heavy workload data")...)
		chunk.Fingerprint = fmt.Sprintf("heavy_fp_%d", i)
		chunk.Offset = int64(i * 1024)
		chunk.Size = int64(len(chunk.Data))

		metadata.Fingerprint = chunk.Fingerprint
		metadata.StorageLocation = fmt.Sprintf("heavy_location_%d", i)
		metadata.Size = chunk.Size
		metadata.CreationTime = time.Now()
		metadata.LastReferencedTime = time.Now()

		buffer = append(buffer, chunk.Data...)

		// Return objects to pools
		globalPools.ChunkPool.PutChunk(chunk)
		globalPools.MetadataPool.PutMetadata(metadata)
		globalPools.BufferPool.PutBuffer(buffer)
	}
	workloadTime := time.Since(start)

	fmt.Printf("  ✅ Heavy Workload: %d operations in %v (%.0f ops/sec)\n",
		50000, workloadTime, float64(50000)/workloadTime.Seconds())

	// Get final stats
	chunkStats := globalPools.ChunkPool.GetStats()
	metadataStats := globalPools.MetadataPool.GetStats()
	bufferStats := globalPools.BufferPool.GetStats()

	fmt.Printf("  ✅ Final Chunk Pool Stats: Allocated=%d, Reused=%d, Returned=%d\n",
		chunkStats.TotalAllocated, chunkStats.TotalReused, chunkStats.TotalReturned)
	fmt.Printf("  ✅ Final Metadata Pool Stats: Allocated=%d, Reused=%d, Returned=%d\n",
		metadataStats.TotalAllocated, metadataStats.TotalReused, metadataStats.TotalReturned)
	fmt.Printf("  ✅ Final Buffer Pool Stats: Allocated=%d, Reused=%d, Returned=%d\n",
		bufferStats.TotalAllocated, bufferStats.TotalReused, bufferStats.TotalReturned)
}

func testConcurrentPerformance() {
	fmt.Println("\n📊 Testing Concurrent Performance...")

	// Test concurrent Cuckoo filter access
	cf := cache.NewCuckooFilter(100000, 0.01)

	start := time.Now()
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10000; j++ {
				fp := fmt.Sprintf("concurrent_fp_%d_%d", id, j)
				cf.Add(fp)
				cf.Contains(fp)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	concurrentTime := time.Since(start)

	fmt.Printf("  ✅ Concurrent Cuckoo Filter: %d operations in %v (%.0f ops/sec)\n",
		200000, concurrentTime, float64(200000)/concurrentTime.Seconds())

	// Test concurrent object pool access
	globalPools := pool.GetGlobalPools()

	start = time.Now()
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10000; j++ {
				chunk := globalPools.ChunkPool.GetChunk()
				metadata := globalPools.MetadataPool.GetMetadata()
				buffer := globalPools.BufferPool.GetBuffer()

				// Use objects
				chunk.Data = append(chunk.Data, []byte("concurrent data")...)
				chunk.Fingerprint = fmt.Sprintf("concurrent_fp_%d_%d", id, j)
				chunk.Offset = int64(j)
				chunk.Size = int64(len(chunk.Data))

				metadata.Fingerprint = chunk.Fingerprint
				metadata.StorageLocation = fmt.Sprintf("concurrent_location_%d_%d", id, j)
				metadata.Size = chunk.Size
				metadata.CreationTime = time.Now()
				metadata.LastReferencedTime = time.Now()

				buffer = append(buffer, chunk.Data...)

				// Return objects
				globalPools.ChunkPool.PutChunk(chunk)
				globalPools.MetadataPool.PutMetadata(metadata)
				globalPools.BufferPool.PutBuffer(buffer)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	concurrentPoolTime := time.Since(start)

	fmt.Printf("  ✅ Concurrent Object Pools: %d operations in %v (%.0f ops/sec)\n",
		300000, concurrentPoolTime, float64(300000)/concurrentPoolTime.Seconds())
}
