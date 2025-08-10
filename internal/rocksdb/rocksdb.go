package rocksdb

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cockroachdb/pebble"
)

// ChunkMetadata represents metadata for a chunk stored in RocksDB
type ChunkMetadata struct {
	Fingerprint        string    `json:"fingerprint"`
	StorageLocation    string    `json:"storage_location"`
	Size               int64     `json:"size"`
	CreationTime       time.Time `json:"creation_time"`
	LastReferencedTime time.Time `json:"last_referenced_time"`
	ReferenceCount     int64     `json:"reference_count"`
}

// RocksDBDedupeStore provides fast local deduplication using RocksDB
type RocksDBDedupeStore struct {
	db     *pebble.DB
	dbPath string
}

// NewRocksDBDedupeStore creates a new RocksDB-based deduplication store
func NewRocksDBDedupeStore(dbPath string) (*RocksDBDedupeStore, error) {
	// Ensure directory exists
	if err := ensureDir(dbPath); err != nil {
		return nil, fmt.Errorf("failed to create DB directory: %w", err)
	}

	// Open RocksDB
	db, err := pebble.Open(dbPath, &pebble.Options{
		// Optimize for read-heavy workloads
		L0CompactionThreshold: 4,
		L0StopWritesThreshold: 12,
		// Use compression for space efficiency
		Compression: pebble.SnappyCompression,
		// Cache size for better performance
		Cache: pebble.NewCache(64 << 20), // 64MB cache
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open RocksDB: %w", err)
	}

	return &RocksDBDedupeStore{
		db:     db,
		dbPath: dbPath,
	}, nil
}

// Close closes the RocksDB connection
func (r *RocksDBDedupeStore) Close() error {
	return r.db.Close()
}

// GetChunkMetadata retrieves chunk metadata by fingerprint
func (r *RocksDBDedupeStore) GetChunkMetadata(fingerprint string) (*ChunkMetadata, bool, error) {
	key := []byte(fmt.Sprintf("chunk:%s", fingerprint))

	data, closer, err := r.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to get chunk metadata: %w", err)
	}
	defer closer.Close()

	var metadata ChunkMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	// Update last referenced time
	metadata.LastReferencedTime = time.Now()
	metadata.ReferenceCount++

	// Update the metadata in the background
	go func() {
		if err := r.updateChunkMetadata(&metadata); err != nil {
			log.Printf("Warning: Failed to update chunk metadata: %v", err)
		}
	}()

	return &metadata, true, nil
}

// PutChunkMetadata stores chunk metadata
func (r *RocksDBDedupeStore) PutChunkMetadata(fingerprint string, metadata *ChunkMetadata) error {
	key := []byte(fmt.Sprintf("chunk:%s", fingerprint))

	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	return r.db.Set(key, data, pebble.Sync)
}

// Contains checks if a fingerprint exists (fast check without full metadata)
func (r *RocksDBDedupeStore) Contains(fingerprint string) (bool, error) {
	key := []byte(fmt.Sprintf("chunk:%s", fingerprint))

	_, closer, err := r.db.Get(key)
	if err == pebble.ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check fingerprint: %w", err)
	}
	closer.Close()
	return true, nil
}

// RemoveChunkMetadata removes chunk metadata
func (r *RocksDBDedupeStore) RemoveChunkMetadata(fingerprint string) error {
	key := []byte(fmt.Sprintf("chunk:%s", fingerprint))
	return r.db.Delete(key, pebble.Sync)
}

// GetStats returns statistics about the RocksDB store
func (r *RocksDBDedupeStore) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Count total chunks
	iter := r.db.NewIter(nil)
	defer iter.Close()

	count := 0
	for iter.First(); iter.Valid(); iter.Next() {
		if len(iter.Key()) > 6 && string(iter.Key()[:6]) == "chunk:" {
			count++
		}
	}

	stats["total_chunks"] = count
	stats["db_path"] = r.dbPath

	return stats, nil
}

// Compact performs database compaction for better performance
func (r *RocksDBDedupeStore) Compact() error {
	return r.db.Compact([]byte("chunk:"), []byte("chunk:\xff"))
}

// updateChunkMetadata updates chunk metadata (internal method)
func (r *RocksDBDedupeStore) updateChunkMetadata(metadata *ChunkMetadata) error {
	return r.PutChunkMetadata(metadata.Fingerprint, metadata)
}

// ensureDir ensures the directory exists
func ensureDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0755)
}
