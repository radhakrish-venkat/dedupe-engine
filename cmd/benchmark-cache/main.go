package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/radhakrishnan.venkat/dedupe-engine/internal/cache"
	"github.com/radhakrishnan.venkat/dedupe-engine/internal/rocksdb"
)

func main() {
	fmt.Println("Cache Performance Benchmark")
	fmt.Println("==========================")

	// Test parameters
	numChunks := 100000
	cacheSize := 10000
	rocksdbPath := "/tmp/rocksdb_benchmark"

	// Clean up RocksDB after test
	defer os.RemoveAll(rocksdbPath)

	// Test 1: In-Memory LRU Cache + Cuckoo Filter
	fmt.Println("\n1. Testing In-Memory LRU Cache + Cuckoo Filter:")
	benchmarkInMemoryCache(numChunks, cacheSize)

	// Test 2: RocksDB
	fmt.Println("\n2. Testing RocksDB:")
	benchmarkRocksDB(numChunks, rocksdbPath)

	// Test 3: Hybrid approach (RocksDB + small in-memory cache)
	fmt.Println("\n3. Testing Hybrid Approach (RocksDB + small in-memory cache):")
	benchmarkHybrid(numChunks, cacheSize/10, rocksdbPath)
}

func benchmarkInMemoryCache(numChunks, cacheSize int) {
	// Create cache
	dc := cache.NewDeduplicationCache(cacheSize, cacheSize*10)

	// Generate test data
	fingerprints := generateFingerprints(numChunks)

	// Test 1: Write performance
	start := time.Now()
	for i, fp := range fingerprints {
		metadata := &cache.ChunkMetadata{
			Fingerprint:        fp,
			StorageLocation:    fmt.Sprintf("location_%d", i),
			Size:               int64(1024 + i%1000),
			CreationTime:       time.Now(),
			LastReferencedTime: time.Now(),
		}
		dc.PutChunkMetadata(fp, metadata)
	}
	writeTime := time.Since(start)

	// Test 2: Read performance (cache hits)
	start = time.Now()
	hits := 0
	for _, fp := range fingerprints[:cacheSize/2] { // Test first half (likely in cache)
		if _, exists := dc.GetChunkMetadata(fp); exists {
			hits++
		}
	}
	readTime := time.Since(start)

	// Test 3: Read performance (cache misses)
	start = time.Now()
	misses := 0
	for _, fp := range fingerprints[numChunks-cacheSize/2:] { // Test last half (likely not in cache)
		if _, exists := dc.GetChunkMetadata(fp); !exists {
			misses++
		}
	}
	missTime := time.Since(start)

	// Test 4: Cuckoo filter performance
	start = time.Now()
	filterHits := 0
	for _, fp := range fingerprints {
		if dc.MightContain(fp) {
			filterHits++
		}
	}
	filterTime := time.Since(start)

	fmt.Printf("  Write %d chunks: %v (%.0f ops/sec)\n", numChunks, writeTime, float64(numChunks)/writeTime.Seconds())
	fmt.Printf("  Read %d cache hits: %v (%.0f ops/sec)\n", hits, readTime, float64(hits)/readTime.Seconds())
	fmt.Printf("  Read %d cache misses: %v (%.0f ops/sec)\n", misses, missTime, float64(misses)/missTime.Seconds())
	fmt.Printf("  Cuckoo filter %d checks: %v (%.0f ops/sec)\n", numChunks, filterTime, float64(numChunks)/filterTime.Seconds())
	fmt.Printf("  Memory usage: ~%d MB\n", estimateMemoryUsage(cacheSize))
}

func benchmarkRocksDB(numChunks int, dbPath string) {
	// Create RocksDB store
	store, err := rocksdb.NewRocksDBDedupeStore(dbPath)
	if err != nil {
		log.Fatalf("Failed to create RocksDB store: %v", err)
	}
	defer store.Close()

	// Generate test data
	fingerprints := generateFingerprints(numChunks)

	// Test 1: Write performance
	start := time.Now()
	for i, fp := range fingerprints {
		metadata := &rocksdb.ChunkMetadata{
			Fingerprint:        fp,
			StorageLocation:    fmt.Sprintf("location_%d", i),
			Size:               int64(1024 + i%1000),
			CreationTime:       time.Now(),
			LastReferencedTime: time.Now(),
			ReferenceCount:     1,
		}
		if err := store.PutChunkMetadata(fp, metadata); err != nil {
			log.Printf("Warning: Failed to write metadata: %v", err)
		}
	}
	writeTime := time.Since(start)

	// Test 2: Read performance (with metadata)
	start = time.Now()
	hits := 0
	for _, fp := range fingerprints[:numChunks/2] {
		if _, exists, _ := store.GetChunkMetadata(fp); exists {
			hits++
		}
	}
	readTime := time.Since(start)

	// Test 3: Fast existence check
	start = time.Now()
	fastHits := 0
	for _, fp := range fingerprints {
		if exists, _ := store.Contains(fp); exists {
			fastHits++
		}
	}
	fastTime := time.Since(start)

	// Get stats
	stats, _ := store.GetStats()

	fmt.Printf("  Write %d chunks: %v (%.0f ops/sec)\n", numChunks, writeTime, float64(numChunks)/writeTime.Seconds())
	fmt.Printf("  Read %d chunks with metadata: %v (%.0f ops/sec)\n", hits, readTime, float64(hits)/readTime.Seconds())
	fmt.Printf("  Fast existence check %d chunks: %v (%.0f ops/sec)\n", fastHits, fastTime, float64(fastHits)/fastTime.Seconds())
	fmt.Printf("  Total chunks in DB: %v\n", stats["total_chunks"])
	fmt.Printf("  Disk usage: ~%d MB\n", estimateDiskUsage(numChunks))
}

func benchmarkHybrid(numChunks, cacheSize int, dbPath string) {
	// Create RocksDB store
	store, err := rocksdb.NewRocksDBDedupeStore(dbPath)
	if err != nil {
		log.Fatalf("Failed to create RocksDB store: %v", err)
	}
	defer store.Close()

	// Create small in-memory cache for hot data
	hotCache := cache.NewDeduplicationCache(cacheSize, cacheSize*10)

	// Generate test data
	fingerprints := generateFingerprints(numChunks)

	// Test 1: Write performance (to RocksDB)
	start := time.Now()
	for i, fp := range fingerprints {
		metadata := &rocksdb.ChunkMetadata{
			Fingerprint:        fp,
			StorageLocation:    fmt.Sprintf("location_%d", i),
			Size:               int64(1024 + i%1000),
			CreationTime:       time.Now(),
			LastReferencedTime: time.Now(),
			ReferenceCount:     1,
		}
		if err := store.PutChunkMetadata(fp, metadata); err != nil {
			log.Printf("Warning: Failed to write metadata: %v", err)
		}
	}
	writeTime := time.Since(start)

	// Test 2: Read performance with hybrid approach
	start = time.Now()
	hits := 0
	cacheHits := 0
	dbHits := 0

	for _, fp := range fingerprints[:numChunks/2] {
		// First check hot cache
		if metadata, exists := hotCache.GetChunkMetadata(fp); exists {
			hits++
			cacheHits++
			continue
		}

		// Then check RocksDB
		if metadata, exists, _ := store.GetChunkMetadata(fp); exists {
			hits++
			dbHits++
			// Add to hot cache for future access
			hotCache.PutChunkMetadata(fp, &cache.ChunkMetadata{
				Fingerprint:        metadata.Fingerprint,
				StorageLocation:    metadata.StorageLocation,
				Size:               metadata.Size,
				CreationTime:       metadata.CreationTime,
				LastReferencedTime: metadata.LastReferencedTime,
			})
		}
	}
	readTime := time.Since(start)

	fmt.Printf("  Write %d chunks: %v (%.0f ops/sec)\n", numChunks, writeTime, float64(numChunks)/writeTime.Seconds())
	fmt.Printf("  Read %d chunks (hybrid): %v (%.0f ops/sec)\n", hits, readTime, float64(hits)/readTime.Seconds())
	fmt.Printf("  Cache hits: %d, DB hits: %d\n", cacheHits, dbHits)
	fmt.Printf("  Memory usage: ~%d MB\n", estimateMemoryUsage(cacheSize))
}

func generateFingerprints(count int) []string {
	fingerprints := make([]string, count)
	for i := 0; i < count; i++ {
		fingerprints[i] = fmt.Sprintf("fingerprint_%d_%x", i, i*12345)
	}
	return fingerprints
}

func estimateMemoryUsage(cacheSize int) int {
	// Rough estimate: each cache entry ~200 bytes
	return cacheSize * 200 / (1024 * 1024)
}

func estimateDiskUsage(numChunks int) int {
	// Rough estimate: each chunk metadata ~300 bytes
	return numChunks * 300 / (1024 * 1024)
}
