package cache

import (
	"testing"
	"time"
)

func TestLRUCache(t *testing.T) {
	cache := NewLRUCache(3)

	// Test putting and getting
	metadata1 := &ChunkMetadata{
		Fingerprint:        "fingerprint1",
		StorageLocation:    "location1",
		Size:               1024,
		CreationTime:       time.Now(),
		LastReferencedTime: time.Now(),
	}

	cache.Put("key1", metadata1)

	// Test get
	retrieved, exists := cache.Get("key1")
	if !exists {
		t.Fatal("Expected to find key1 in cache")
	}
	if retrieved.Fingerprint != "fingerprint1" {
		t.Errorf("Expected fingerprint1, got %s", retrieved.Fingerprint)
	}

	// Test non-existent key
	_, exists = cache.Get("nonexistent")
	if exists {
		t.Fatal("Expected key to not exist")
	}

	// Test capacity
	cache.Put("key2", &ChunkMetadata{Fingerprint: "fingerprint2"})
	cache.Put("key3", &ChunkMetadata{Fingerprint: "fingerprint3"})
	cache.Put("key4", &ChunkMetadata{Fingerprint: "fingerprint4"}) // Should evict key1

	if cache.Size() != 3 {
		t.Errorf("Expected cache size 3, got %d", cache.Size())
	}

	// key1 should be evicted
	_, exists = cache.Get("key1")
	if exists {
		t.Fatal("Expected key1 to be evicted")
	}
}

func TestDeduplicationCache(t *testing.T) {
	dc := NewDeduplicationCache(10, 100)

	metadata := &ChunkMetadata{
		Fingerprint:        "test-fingerprint",
		StorageLocation:    "test-location",
		Size:               2048,
		CreationTime:       time.Now(),
		LastReferencedTime: time.Now(),
	}

	// Test putting metadata
	dc.PutChunkMetadata("test-fingerprint", metadata)

	// Test getting metadata
	retrieved, exists := dc.GetChunkMetadata("test-fingerprint")
	if !exists {
		t.Fatal("Expected to find metadata in cache")
	}
	if retrieved.Fingerprint != "test-fingerprint" {
		t.Errorf("Expected test-fingerprint, got %s", retrieved.Fingerprint)
	}

	// Test might contain
	if !dc.MightContain("test-fingerprint") {
		t.Fatal("Expected fingerprint to be in filter")
	}

	// Test removing
	if !dc.RemoveChunkMetadata("test-fingerprint") {
		t.Fatal("Expected to successfully remove metadata")
	}

	// Test it's gone
	_, exists = dc.GetChunkMetadata("test-fingerprint")
	if exists {
		t.Fatal("Expected metadata to be removed")
	}
}
