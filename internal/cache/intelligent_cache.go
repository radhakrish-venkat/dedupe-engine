package cache

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/radhakrishnan.venkat/dedupe-engine/internal/db"
)

// IntelligentCache implements predictive cache warming
type IntelligentCache struct {
	// Core cache components
	dedupeCache *DeduplicationCache
	dbClient    DBClient

	// Predictive components
	accessPatterns *AccessPatternAnalyzer
	predictor      *ChunkPredictor
	warmingQueue   *WarmingQueue

	// Configuration
	config *IntelligentCacheConfig

	// Statistics
	stats      *IntelligentCacheStats
	statsMutex sync.RWMutex

	// Context for background operations
	ctx    context.Context
	cancel context.CancelFunc
}

// IntelligentCacheConfig holds configuration for intelligent cache
type IntelligentCacheConfig struct {
	// Warming settings
	WarmingEnabled   bool
	WarmingThreshold float64 // Minimum confidence for warming
	MaxWarmingChunks int
	WarmingLookback  time.Duration
	WarmingInterval  time.Duration

	// Prediction settings
	PredictionWindow    time.Duration
	MinAccessCount      int
	ConfidenceThreshold float64

	// Performance settings
	MaxConcurrentWarming int
	WarmingTimeout       time.Duration
}

// IntelligentCacheStats tracks intelligent cache performance
type IntelligentCacheStats struct {
	TotalAccesses      int64
	CacheHits          int64
	CacheMisses        int64
	WarmingPredictions int64
	WarmingHits        int64
	WarmingMisses      int64
	PredictionAccuracy float64
	LastReset          time.Time
}

// AccessPatternAnalyzer analyzes access patterns
type AccessPatternAnalyzer struct {
	patterns map[string]*AccessPattern
	mutex    sync.RWMutex
	config   *IntelligentCacheConfig
}

// AccessPattern represents a chunk access pattern
type AccessPattern struct {
	Fingerprint    string
	AccessCount    int
	LastAccess     time.Time
	AccessTimes    []time.Time
	RelatedChunks  map[string]float64 // fingerprint -> correlation score
	Frequency      float64
	Predictability float64
}

// ChunkPredictor predicts which chunks will be accessed
type ChunkPredictor struct {
	patterns    *AccessPatternAnalyzer
	config      *IntelligentCacheConfig
	predictions map[string]*Prediction
	mutex       sync.RWMutex
}

// Prediction represents a chunk access prediction
type Prediction struct {
	Fingerprint string
	Confidence  float64
	Reason      string
	Timestamp   time.Time
	ExpiresAt   time.Time
}

// WarmingQueue manages cache warming operations
type WarmingQueue struct {
	items    []WarmingItem
	mutex    sync.Mutex
	capacity int
}

// WarmingItem represents a cache warming operation
type WarmingItem struct {
	Fingerprint string
	Priority    float64
	Timestamp   time.Time
	Reason      string
}

// DBClient interface for database operations
type DBClient interface {
	GetChunkMetadataByFingerprint(ctx context.Context, fingerprint string) (*db.ChunkMetadata, error)
	InsertChunkMetadata(ctx context.Context, metadata *db.ChunkMetadata) error
}

// NewIntelligentCache creates a new intelligent cache
func NewIntelligentCache(dedupeCache *DeduplicationCache, dbClient DBClient, config *IntelligentCacheConfig) *IntelligentCache {
	if config == nil {
		config = &IntelligentCacheConfig{
			WarmingEnabled:       true,
			WarmingThreshold:     0.7,
			MaxWarmingChunks:     100,
			WarmingLookback:      1 * time.Hour,
			WarmingInterval:      30 * time.Second,
			PredictionWindow:     5 * time.Minute,
			MinAccessCount:       3,
			ConfidenceThreshold:  0.6,
			MaxConcurrentWarming: 4,
			WarmingTimeout:       10 * time.Second,
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	ic := &IntelligentCache{
		dedupeCache: dedupeCache,
		dbClient:    dbClient,
		config:      config,
		stats: &IntelligentCacheStats{
			LastReset: time.Now(),
		},
		ctx:    ctx,
		cancel: cancel,
	}

	// Initialize components
	ic.accessPatterns = NewAccessPatternAnalyzer(config)
	ic.predictor = NewChunkPredictor(ic.accessPatterns, config)
	ic.warmingQueue = NewWarmingQueue(config.MaxWarmingChunks)

	// Start background processes
	ic.startBackgroundProcesses()

	return ic
}

// Close closes the intelligent cache
func (ic *IntelligentCache) Close() {
	ic.cancel()
}

// GetChunkMetadata gets chunk metadata with intelligent warming
func (ic *IntelligentCache) GetChunkMetadata(fingerprint string) (*ChunkMetadata, bool) {
	ic.statsMutex.Lock()
	ic.stats.TotalAccesses++
	ic.statsMutex.Unlock()

	// Record access pattern
	ic.accessPatterns.RecordAccess(fingerprint)

	// Try to get from cache first
	metadata, exists := ic.dedupeCache.GetChunkMetadata(fingerprint)
	if exists {
		ic.statsMutex.Lock()
		ic.stats.CacheHits++
		ic.statsMutex.Unlock()
		return metadata, true
	}

	ic.statsMutex.Lock()
	ic.stats.CacheMisses++
	ic.statsMutex.Unlock()

	// Try to get from database
	if ic.dbClient != nil {
		ctx, cancel := context.WithTimeout(ic.ctx, ic.config.WarmingTimeout)
		defer cancel()

		dbMetadata, err := ic.dbClient.GetChunkMetadataByFingerprint(ctx, fingerprint)
		if err == nil && dbMetadata != nil {
			// Convert db.ChunkMetadata to cache.ChunkMetadata
			cacheMetadata := &ChunkMetadata{
				Fingerprint:        dbMetadata.Fingerprint,
				StorageLocation:    dbMetadata.StorageLocation,
				Size:               int64(dbMetadata.Size),
				CreationTime:       dbMetadata.CreationTime,
				LastReferencedTime: dbMetadata.LastReferencedTime,
			}
			// Add to cache
			ic.dedupeCache.PutChunkMetadata(fingerprint, cacheMetadata)
			return cacheMetadata, true
		}
	}

	// Trigger predictive warming for related chunks
	go ic.triggerPredictiveWarming(fingerprint)

	return nil, false
}

// AddChunkMetadata adds chunk metadata to cache
func (ic *IntelligentCache) AddChunkMetadata(fingerprint string, metadata *ChunkMetadata) {
	ic.dedupeCache.PutChunkMetadata(fingerprint, metadata)
}

// triggerPredictiveWarming triggers warming for related chunks
func (ic *IntelligentCache) triggerPredictiveWarming(accessedFingerprint string) {
	if !ic.config.WarmingEnabled {
		return
	}

	// Get predictions for related chunks
	predictions := ic.predictor.PredictRelatedChunks(accessedFingerprint)

	// Add high-confidence predictions to warming queue
	for _, prediction := range predictions {
		if prediction.Confidence >= ic.config.WarmingThreshold {
			ic.warmingQueue.Add(WarmingItem{
				Fingerprint: prediction.Fingerprint,
				Priority:    prediction.Confidence,
				Timestamp:   time.Now(),
				Reason:      prediction.Reason,
			})
		}
	}
}

// startBackgroundProcesses starts background warming and analysis
func (ic *IntelligentCache) startBackgroundProcesses() {
	// Start warming worker
	go ic.warmingWorker()

	// Start pattern analysis worker
	go ic.patternAnalysisWorker()

	// Start prediction worker
	go ic.predictionWorker()
}

// warmingWorker processes cache warming operations
func (ic *IntelligentCache) warmingWorker() {
	ticker := time.NewTicker(ic.config.WarmingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ic.ctx.Done():
			return
		case <-ticker.C:
			ic.processWarmingQueue()
		}
	}
}

// processWarmingQueue processes items in the warming queue
func (ic *IntelligentCache) processWarmingQueue() {
	items := ic.warmingQueue.GetItems()
	if len(items) == 0 {
		return
	}

	// Sort by priority
	sort.Slice(items, func(i, j int) bool {
		return items[i].Priority > items[j].Priority
	})

	// Process top items
	processed := 0
	for _, item := range items {
		if processed >= ic.config.MaxConcurrentWarming {
			break
		}

		// Check if already in cache
		if _, exists := ic.dedupeCache.GetChunkMetadata(item.Fingerprint); exists {
			continue
		}

		// Try to warm from database
		if ic.warmChunk(item.Fingerprint) {
			processed++
		}
	}
}

// warmChunk warms a chunk from the database
func (ic *IntelligentCache) warmChunk(fingerprint string) bool {
	if ic.dbClient == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(ic.ctx, ic.config.WarmingTimeout)
	defer cancel()

	dbMetadata, err := ic.dbClient.GetChunkMetadataByFingerprint(ctx, fingerprint)
	if err != nil || dbMetadata == nil {
		return false
	}

	// Convert db.ChunkMetadata to cache.ChunkMetadata
	cacheMetadata := &ChunkMetadata{
		Fingerprint:        dbMetadata.Fingerprint,
		StorageLocation:    dbMetadata.StorageLocation,
		Size:               int64(dbMetadata.Size),
		CreationTime:       dbMetadata.CreationTime,
		LastReferencedTime: dbMetadata.LastReferencedTime,
	}

	// Add to cache
	ic.dedupeCache.PutChunkMetadata(fingerprint, cacheMetadata)

	ic.statsMutex.Lock()
	ic.stats.WarmingPredictions++
	ic.statsMutex.Unlock()

	return true
}

// patternAnalysisWorker analyzes access patterns
func (ic *IntelligentCache) patternAnalysisWorker() {
	ticker := time.NewTicker(ic.config.WarmingInterval * 2)
	defer ticker.Stop()

	for {
		select {
		case <-ic.ctx.Done():
			return
		case <-ticker.C:
			ic.accessPatterns.AnalyzePatterns()
		}
	}
}

// predictionWorker updates predictions
func (ic *IntelligentCache) predictionWorker() {
	ticker := time.NewTicker(ic.config.WarmingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ic.ctx.Done():
			return
		case <-ticker.C:
			ic.predictor.UpdatePredictions()
		}
	}
}

// GetStats returns intelligent cache statistics
func (ic *IntelligentCache) GetStats() *IntelligentCacheStats {
	ic.statsMutex.RLock()
	defer ic.statsMutex.RUnlock()

	stats := *ic.stats // Copy to avoid race conditions

	// Calculate hit rate
	if stats.TotalAccesses > 0 {
		stats.PredictionAccuracy = float64(stats.CacheHits) / float64(stats.TotalAccesses)
	}

	return &stats
}

// ResetStats resets intelligent cache statistics
func (ic *IntelligentCache) ResetStats() {
	ic.statsMutex.Lock()
	defer ic.statsMutex.Unlock()

	ic.stats = &IntelligentCacheStats{
		LastReset: time.Now(),
	}
}

// NewAccessPatternAnalyzer creates a new access pattern analyzer
func NewAccessPatternAnalyzer(config *IntelligentCacheConfig) *AccessPatternAnalyzer {
	return &AccessPatternAnalyzer{
		patterns: make(map[string]*AccessPattern),
		config:   config,
	}
}

// RecordAccess records a chunk access
func (apa *AccessPatternAnalyzer) RecordAccess(fingerprint string) {
	apa.mutex.Lock()
	defer apa.mutex.Unlock()

	now := time.Now()
	pattern, exists := apa.patterns[fingerprint]

	if !exists {
		pattern = &AccessPattern{
			Fingerprint:   fingerprint,
			RelatedChunks: make(map[string]float64),
		}
		apa.patterns[fingerprint] = pattern
	}

	pattern.AccessCount++
	pattern.LastAccess = now
	pattern.AccessTimes = append(pattern.AccessTimes, now)

	// Keep only recent access times
	cutoff := now.Add(-apa.config.WarmingLookback)
	var recentTimes []time.Time
	for _, t := range pattern.AccessTimes {
		if t.After(cutoff) {
			recentTimes = append(recentTimes, t)
		}
	}
	pattern.AccessTimes = recentTimes

	// Update frequency
	if len(pattern.AccessTimes) > 1 {
		pattern.Frequency = float64(len(pattern.AccessTimes)) / apa.config.WarmingLookback.Seconds()
	}
}

// AnalyzePatterns analyzes access patterns
func (apa *AccessPatternAnalyzer) AnalyzePatterns() {
	apa.mutex.Lock()
	defer apa.mutex.Unlock()

	now := time.Now()
	cutoff := now.Add(-apa.config.WarmingLookback)

	// Clean up old patterns
	for fp, pattern := range apa.patterns {
		if pattern.LastAccess.Before(cutoff) {
			delete(apa.patterns, fp)
		}
	}

	// Analyze correlations
	for fp1, pattern1 := range apa.patterns {
		if pattern1.AccessCount < apa.config.MinAccessCount {
			continue
		}

		for fp2, pattern2 := range apa.patterns {
			if fp1 == fp2 {
				continue
			}

			if pattern2.AccessCount < apa.config.MinAccessCount {
				continue
			}

			// Calculate correlation
			correlation := apa.calculateCorrelation(pattern1, pattern2)
			if correlation > apa.config.ConfidenceThreshold {
				pattern1.RelatedChunks[fp2] = correlation
			}
		}

		// Calculate predictability
		pattern1.Predictability = apa.calculatePredictability(pattern1)
	}
}

// calculateCorrelation calculates correlation between two patterns
func (apa *AccessPatternAnalyzer) calculateCorrelation(p1, p2 *AccessPattern) float64 {
	if len(p1.AccessTimes) < 2 || len(p2.AccessTimes) < 2 {
		return 0
	}

	// Simple time-based correlation
	// Count accesses within time windows
	windowSize := 1 * time.Minute
	correlation := 0.0
	totalWindows := 0

	start := time.Now().Add(-apa.config.WarmingLookback)
	for t := start; t.Before(time.Now()); t = t.Add(windowSize) {
		end := t.Add(windowSize)

		count1 := apa.countAccessesInWindow(p1.AccessTimes, t, end)
		count2 := apa.countAccessesInWindow(p2.AccessTimes, t, end)

		if count1 > 0 && count2 > 0 {
			correlation += 1.0
		}
		totalWindows++
	}

	if totalWindows > 0 {
		return correlation / float64(totalWindows)
	}
	return 0
}

// countAccessesInWindow counts accesses in a time window
func (apa *AccessPatternAnalyzer) countAccessesInWindow(times []time.Time, start, end time.Time) int {
	count := 0
	for _, t := range times {
		if t.After(start) && t.Before(end) {
			count++
		}
	}
	return count
}

// calculatePredictability calculates how predictable a pattern is
func (apa *AccessPatternAnalyzer) calculatePredictability(pattern *AccessPattern) float64 {
	if len(pattern.AccessTimes) < 2 {
		return 0
	}

	// Calculate time intervals between accesses
	var intervals []float64
	for i := 1; i < len(pattern.AccessTimes); i++ {
		interval := pattern.AccessTimes[i].Sub(pattern.AccessTimes[i-1]).Seconds()
		intervals = append(intervals, interval)
	}

	// Calculate coefficient of variation (lower = more predictable)
	if len(intervals) == 0 {
		return 0
	}

	mean := 0.0
	for _, interval := range intervals {
		mean += interval
	}
	mean /= float64(len(intervals))

	variance := 0.0
	for _, interval := range intervals {
		variance += math.Pow(interval-mean, 2)
	}
	variance /= float64(len(intervals))

	if mean == 0 {
		return 0
	}

	cv := math.Sqrt(variance) / mean
	// Convert to predictability (0-1, higher is more predictable)
	return math.Max(0, 1-cv)
}

// NewChunkPredictor creates a new chunk predictor
func NewChunkPredictor(patterns *AccessPatternAnalyzer, config *IntelligentCacheConfig) *ChunkPredictor {
	return &ChunkPredictor{
		patterns:    patterns,
		config:      config,
		predictions: make(map[string]*Prediction),
	}
}

// PredictRelatedChunks predicts which chunks will be accessed next
func (cp *ChunkPredictor) PredictRelatedChunks(fingerprint string) []*Prediction {
	cp.mutex.RLock()
	defer cp.mutex.RUnlock()

	cp.patterns.mutex.RLock()
	pattern, exists := cp.patterns.patterns[fingerprint]
	cp.patterns.mutex.RUnlock()

	if !exists {
		return nil
	}

	var predictions []*Prediction
	now := time.Now()

	for relatedFp, correlation := range pattern.RelatedChunks {
		confidence := correlation * pattern.Predictability
		if confidence >= cp.config.ConfidenceThreshold {
			predictions = append(predictions, &Prediction{
				Fingerprint: relatedFp,
				Confidence:  confidence,
				Reason:      fmt.Sprintf("correlated with %s (%.2f)", fingerprint, correlation),
				Timestamp:   now,
				ExpiresAt:   now.Add(cp.config.PredictionWindow),
			})
		}
	}

	// Sort by confidence
	sort.Slice(predictions, func(i, j int) bool {
		return predictions[i].Confidence > predictions[j].Confidence
	})

	return predictions
}

// UpdatePredictions updates predictions
func (cp *ChunkPredictor) UpdatePredictions() {
	cp.mutex.Lock()
	defer cp.mutex.Unlock()

	now := time.Now()

	// Clean up expired predictions
	for fp, prediction := range cp.predictions {
		if now.After(prediction.ExpiresAt) {
			delete(cp.predictions, fp)
		}
	}
}

// NewWarmingQueue creates a new warming queue
func NewWarmingQueue(capacity int) *WarmingQueue {
	return &WarmingQueue{
		items:    make([]WarmingItem, 0, capacity),
		capacity: capacity,
	}
}

// Add adds an item to the warming queue
func (wq *WarmingQueue) Add(item WarmingItem) {
	wq.mutex.Lock()
	defer wq.mutex.Unlock()

	// Check if already in queue
	for i, existing := range wq.items {
		if existing.Fingerprint == item.Fingerprint {
			// Update priority if new item has higher priority
			if item.Priority > existing.Priority {
				wq.items[i] = item
			}
			return
		}
	}

	// Add new item
	if len(wq.items) < wq.capacity {
		wq.items = append(wq.items, item)
	} else {
		// Replace lowest priority item
		lowestIndex := 0
		lowestPriority := wq.items[0].Priority
		for i, existing := range wq.items {
			if existing.Priority < lowestPriority {
				lowestPriority = existing.Priority
				lowestIndex = i
			}
		}
		if item.Priority > lowestPriority {
			wq.items[lowestIndex] = item
		}
	}
}

// GetItems returns all items in the queue
func (wq *WarmingQueue) GetItems() []WarmingItem {
	wq.mutex.Lock()
	defer wq.mutex.Unlock()

	items := make([]WarmingItem, len(wq.items))
	copy(items, wq.items)
	return items
}
