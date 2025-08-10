package dedupe

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/radhakrishnan.venkat/dedupe-engine/internal/cache"
	"github.com/radhakrishnan.venkat/dedupe-engine/internal/db"
	"github.com/radhakrishnan.venkat/dedupe-engine/internal/rocksdb"
)

// HybridDedupeEngine provides production-ready deduplication
type HybridDedupeEngine struct {
	// Hot data cache (small, fast)
	hotCache *cache.DeduplicationCache

	// Persistent local storage (RocksDB)
	localStore *rocksdb.RocksDBDedupeStore

	// Centralized metadata (CockroachDB)
	centralDB *db.DB

	// Configuration
	config *Config

	// Statistics
	stats      *Stats
	statsMutex sync.RWMutex
}

// Config holds configuration for the hybrid dedupe engine
type Config struct {
	HotCacheSize    int    // Number of entries in hot cache
	RocksDBPath     string // Path for RocksDB storage
	EnableCentralDB bool   // Whether to use CockroachDB
	SyncToCentralDB bool   // Whether to sync to CockroachDB
	BatchSize       int    // Batch size for central DB operations
}

// Stats holds performance statistics
type Stats struct {
	HotCacheHits   int64
	LocalStoreHits int64
	CentralDBHits  int64
	Misses         int64
	TotalRequests  int64
	LastReset      time.Time
}

// NewHybridDedupeEngine creates a new hybrid deduplication engine
func NewHybridDedupeEngine(config *Config, centralDB *db.DB) (*HybridDedupeEngine, error) {
	// Create hot cache for frequently accessed data
	hotCache := cache.NewDeduplicationCache(config.HotCacheSize, config.HotCacheSize*10)

	// Create RocksDB store for persistent local storage
	localStore, err := rocksdb.NewRocksDBDedupeStore(config.RocksDBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create RocksDB store: %w", err)
	}

	engine := &HybridDedupeEngine{
		hotCache:   hotCache,
		localStore: localStore,
		centralDB:  centralDB,
		config:     config,
		stats: &Stats{
			LastReset: time.Now(),
		},
	}

	// Start background tasks
	go engine.backgroundTasks()

	return engine, nil
}

// Close closes the dedupe engine
func (e *HybridDedupeEngine) Close() error {
	return e.localStore.Close()
}

// GetChunkMetadata retrieves chunk metadata using the hybrid approach
func (e *HybridDedupeEngine) GetChunkMetadata(ctx context.Context, fingerprint string) (*cache.ChunkMetadata, bool, error) {
	e.statsMutex.Lock()
	e.stats.TotalRequests++
	e.statsMutex.Unlock()

	// 1. Check hot cache first (fastest)
	if metadata, exists := e.hotCache.GetChunkMetadata(fingerprint); exists {
		e.statsMutex.Lock()
		e.stats.HotCacheHits++
		e.statsMutex.Unlock()
		return metadata, true, nil
	}

	// 2. Check local RocksDB store
	if metadata, exists, err := e.localStore.GetChunkMetadata(fingerprint); err == nil && exists {
		e.statsMutex.Lock()
		e.stats.LocalStoreHits++
		e.statsMutex.Unlock()

		// Add to hot cache for future access
		e.hotCache.PutChunkMetadata(fingerprint, &cache.ChunkMetadata{
			Fingerprint:        metadata.Fingerprint,
			StorageLocation:    metadata.StorageLocation,
			Size:               metadata.Size,
			CreationTime:       metadata.CreationTime,
			LastReferencedTime: metadata.LastReferencedTime,
		})

		return &cache.ChunkMetadata{
			Fingerprint:        metadata.Fingerprint,
			StorageLocation:    metadata.StorageLocation,
			Size:               metadata.Size,
			CreationTime:       metadata.CreationTime,
			LastReferencedTime: metadata.LastReferencedTime,
		}, true, nil
	}

	// 3. Check centralized database (if enabled)
	if e.config.EnableCentralDB && e.centralDB != nil {
		if dbMetadata, err := e.centralDB.GetChunkMetadataByFingerprint(ctx, fingerprint); err == nil && dbMetadata != nil {
			e.statsMutex.Lock()
			e.stats.CentralDBHits++
			e.statsMutex.Unlock()

			// Convert to cache metadata
			cacheMetadata := &cache.ChunkMetadata{
				Fingerprint:        dbMetadata.Fingerprint,
				StorageLocation:    dbMetadata.StorageLocation,
				Size:               int64(dbMetadata.Size),
				CreationTime:       dbMetadata.CreationTime,
				LastReferencedTime: dbMetadata.LastReferencedTime,
			}

			// Store in both local storage and hot cache
			e.localStore.PutChunkMetadata(fingerprint, &rocksdb.ChunkMetadata{
				Fingerprint:        cacheMetadata.Fingerprint,
				StorageLocation:    cacheMetadata.StorageLocation,
				Size:               cacheMetadata.Size,
				CreationTime:       cacheMetadata.CreationTime,
				LastReferencedTime: cacheMetadata.LastReferencedTime,
				ReferenceCount:     1,
			})
			e.hotCache.PutChunkMetadata(fingerprint, cacheMetadata)

			return cacheMetadata, true, nil
		}
	}

	// 4. Not found
	e.statsMutex.Lock()
	e.stats.Misses++
	e.statsMutex.Unlock()

	return nil, false, nil
}

// PutChunkMetadata stores chunk metadata in the hybrid system
func (e *HybridDedupeEngine) PutChunkMetadata(ctx context.Context, fingerprint string, metadata *cache.ChunkMetadata) error {
	// 1. Store in hot cache
	e.hotCache.PutChunkMetadata(fingerprint, metadata)

	// 2. Store in local RocksDB
	rocksMetadata := &rocksdb.ChunkMetadata{
		Fingerprint:        metadata.Fingerprint,
		StorageLocation:    metadata.StorageLocation,
		Size:               metadata.Size,
		CreationTime:       metadata.CreationTime,
		LastReferencedTime: metadata.LastReferencedTime,
		ReferenceCount:     1,
	}

	if err := e.localStore.PutChunkMetadata(fingerprint, rocksMetadata); err != nil {
		return fmt.Errorf("failed to store in RocksDB: %w", err)
	}

	// 3. Store in centralized DB (if enabled and sync is on)
	if e.config.EnableCentralDB && e.config.SyncToCentralDB && e.centralDB != nil {
		dbMetadata := &db.ChunkMetadata{
			Fingerprint:        metadata.Fingerprint,
			StorageLocation:    metadata.StorageLocation,
			Size:               int(metadata.Size),
			CreationTime:       metadata.CreationTime,
			LastReferencedTime: metadata.LastReferencedTime,
		}

		// Use background goroutine to avoid blocking
		go func() {
			if err := e.centralDB.InsertChunkMetadata(context.Background(), dbMetadata); err != nil {
				log.Printf("Warning: Failed to sync to central DB: %v", err)
			}
		}()
	}

	return nil
}

// Contains checks if a fingerprint exists (fast check)
func (e *HybridDedupeEngine) Contains(ctx context.Context, fingerprint string) (bool, error) {
	// 1. Check hot cache first
	if e.hotCache.MightContain(fingerprint) {
		if _, exists := e.hotCache.GetChunkMetadata(fingerprint); exists {
			return true, nil
		}
	}

	// 2. Check local store
	if exists, err := e.localStore.Contains(fingerprint); err == nil && exists {
		return true, nil
	}

	// 3. Check central DB (if enabled)
	if e.config.EnableCentralDB && e.centralDB != nil {
		if metadata, err := e.centralDB.GetChunkMetadataByFingerprint(ctx, fingerprint); err == nil && metadata != nil {
			return true, nil
		}
	}

	return false, nil
}

// GetStats returns current statistics
func (e *HybridDedupeEngine) GetStats() *Stats {
	e.statsMutex.RLock()
	defer e.statsMutex.RUnlock()

	stats := *e.stats // Copy to avoid race conditions
	return &stats
}

// GetHitRate returns the current hit rate
func (e *HybridDedupeEngine) GetHitRate() float64 {
	stats := e.GetStats()
	if stats.TotalRequests == 0 {
		return 0
	}

	hits := stats.HotCacheHits + stats.LocalStoreHits + stats.CentralDBHits
	return float64(hits) / float64(stats.TotalRequests) * 100
}

// backgroundTasks runs background maintenance tasks
func (e *HybridDedupeEngine) backgroundTasks() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		// Compact RocksDB periodically
		if err := e.localStore.Compact(); err != nil {
			log.Printf("Warning: Failed to compact RocksDB: %v", err)
		}

		// Log statistics
		stats := e.GetStats()
		log.Printf("Dedupe Engine Stats - Hit Rate: %.2f%%, Hot: %d, Local: %d, Central: %d, Misses: %d",
			e.GetHitRate(), stats.HotCacheHits, stats.LocalStoreHits, stats.CentralDBHits, stats.Misses)
	}
}

// GetLocalStoreStats returns RocksDB statistics
func (e *HybridDedupeEngine) GetLocalStoreStats() (map[string]interface{}, error) {
	return e.localStore.GetStats()
}
