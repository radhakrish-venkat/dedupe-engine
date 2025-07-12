package main

import (
	"fmt"
	"time"

	"github.com/radhakrishnan.venkat/dedupe-engine/internal/cache"
)

func main() {
	fmt.Println("Testing Cache Functionality")
	fmt.Println("==========================")

	// Test LRU Cache
	fmt.Println("\n1. Testing LRU Cache:")
	lruCache := cache.NewLRUCache(3)

	// Add some test data
	metadata1 := &cache.ChunkMetadata{
		Fingerprint:        "fingerprint1",
		StorageLocation:    "location1",
		Size:               1024,
		CreationTime:       time.Now(),
		LastReferencedTime: time.Now(),
	}

	lruCache.Put("key1", metadata1)
	fmt.Printf("  Added key1, cache size: %d\n", lruCache.Size())

	// Test retrieval
	retrieved, exists := lruCache.Get("key1")
	if exists {
		fmt.Printf("  Retrieved key1: %s\n", retrieved.Fingerprint)
	} else {
		fmt.Println("  Failed to retrieve key1")
	}

	// Test capacity
	lruCache.Put("key2", &cache.ChunkMetadata{Fingerprint: "fingerprint2"})
	lruCache.Put("key3", &cache.ChunkMetadata{Fingerprint: "fingerprint3"})
	lruCache.Put("key4", &cache.ChunkMetadata{Fingerprint: "fingerprint4"})
	fmt.Printf("  After adding 4 items, cache size: %d\n", lruCache.Size())

	// Test eviction
	_, exists = lruCache.Get("key1")
	if !exists {
		fmt.Println("  ✓ key1 was correctly evicted (LRU working)")
	} else {
		fmt.Println("  ✗ key1 was not evicted (LRU not working)")
	}

	// Test Deduplication Cache
	fmt.Println("\n2. Testing Deduplication Cache:")
	dc := cache.NewDeduplicationCache(10, 100)

	// Add metadata
	dc.PutChunkMetadata("test-fingerprint", metadata1)
	fmt.Printf("  Added metadata, cache size: %d\n", dc.Size())

	// Test retrieval
	retrieved, exists = dc.GetChunkMetadata("test-fingerprint")
	if exists {
		fmt.Printf("  Retrieved metadata: %s\n", retrieved.Fingerprint)
	} else {
		fmt.Println("  Failed to retrieve metadata")
	}

	// Test Cuckoo filter
	if dc.MightContain("test-fingerprint") {
		fmt.Println("  ✓ Cuckoo filter correctly identifies fingerprint")
	} else {
		fmt.Println("  ✗ Cuckoo filter failed to identify fingerprint")
	}

	// Test non-existent fingerprint
	if !dc.MightContain("non-existent-fingerprint") {
		fmt.Println("  ✓ Cuckoo filter correctly identifies non-existent fingerprint")
	} else {
		fmt.Println("  ✗ Cuckoo filter incorrectly identified non-existent fingerprint")
	}

	// Test removal
	if dc.RemoveChunkMetadata("test-fingerprint") {
		fmt.Println("  ✓ Successfully removed metadata")
	} else {
		fmt.Println("  ✗ Failed to remove metadata")
	}

	// Verify it's gone
	_, exists = dc.GetChunkMetadata("test-fingerprint")
	if !exists {
		fmt.Println("  ✓ Metadata correctly removed")
	} else {
		fmt.Println("  ✗ Metadata still exists after removal")
	}

	fmt.Println("\nCache testing completed!")
}
