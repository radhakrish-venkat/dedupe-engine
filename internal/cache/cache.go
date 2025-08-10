package cache

import (
	"container/list"
	"sync"
	"time"
)

// ChunkMetadata represents metadata for a chunk
type ChunkMetadata struct {
	Fingerprint        string
	StorageLocation    string
	Size               int64
	CreationTime       time.Time
	LastReferencedTime time.Time
}

// LRUCache implements a thread-safe LRU cache for chunk metadata
type LRUCache struct {
	capacity int
	cache    map[string]*list.Element
	list     *list.List
	mutex    sync.RWMutex
}

// cacheEntry represents an entry in the LRU cache
type cacheEntry struct {
	key   string
	value *ChunkMetadata
}

// NewLRUCache creates a new LRU cache with the specified capacity
func NewLRUCache(capacity int) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		cache:    make(map[string]*list.Element),
		list:     list.New(),
	}
}

// Get retrieves a value from the cache
func (c *LRUCache) Get(key string) (*ChunkMetadata, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if element, exists := c.cache[key]; exists {
		c.list.MoveToFront(element)
		entry := element.Value.(*cacheEntry)
		entry.value.LastReferencedTime = time.Now()
		return entry.value, true
	}
	return nil, false
}

// Put adds a value to the cache
func (c *LRUCache) Put(key string, value *ChunkMetadata) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if element, exists := c.cache[key]; exists {
		c.list.MoveToFront(element)
		entry := element.Value.(*cacheEntry)
		entry.value = value
		entry.value.LastReferencedTime = time.Now()
		return
	}

	entry := &cacheEntry{
		key:   key,
		value: value,
	}
	element := c.list.PushFront(entry)
	c.cache[key] = element

	// Remove oldest entry if capacity exceeded
	if c.list.Len() > c.capacity {
		oldest := c.list.Back()
		if oldest != nil {
			c.list.Remove(oldest)
			delete(c.cache, oldest.Value.(*cacheEntry).key)
		}
	}
}

// Remove removes a key from the cache
func (c *LRUCache) Remove(key string) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if element, exists := c.cache[key]; exists {
		c.list.Remove(element)
		delete(c.cache, key)
		return true
	}
	return false
}

// Size returns the current number of items in the cache
func (c *LRUCache) Size() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.list.Len()
}

// Clear removes all items from the cache
func (c *LRUCache) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.cache = make(map[string]*list.Element)
	c.list.Init()
}

// SimpleCuckooFilter implements a basic Cuckoo filter for fast fingerprint lookups
// DEPRECATED: Use CuckooFilter instead for proper implementation
type SimpleCuckooFilter struct {
	*CuckooFilter // Embed the proper implementation
}

// NewSimpleCuckooFilter creates a new Cuckoo filter (now uses proper implementation)
func NewSimpleCuckooFilter(capacity int) *SimpleCuckooFilter {
	// Use 0.01 false positive rate for good accuracy
	cf := NewCuckooFilter(capacity, 0.01)
	return &SimpleCuckooFilter{CuckooFilter: cf}
}

// Add adds a fingerprint to the filter
func (cf *SimpleCuckooFilter) Add(fingerprint string) bool {
	return cf.CuckooFilter.Add(fingerprint)
}

// Contains checks if a fingerprint might be in the filter
func (cf *SimpleCuckooFilter) Contains(fingerprint string) bool {
	return cf.CuckooFilter.Contains(fingerprint)
}

// Remove removes a fingerprint from the filter
func (cf *SimpleCuckooFilter) Remove(fingerprint string) bool {
	return cf.CuckooFilter.Remove(fingerprint)
}

// DeduplicationCache combines LRU cache and Cuckoo filter for efficient deduplication
type DeduplicationCache struct {
	lruCache     *LRUCache
	cuckooFilter *SimpleCuckooFilter
	mutex        sync.RWMutex
}

// NewDeduplicationCache creates a new deduplication cache
func NewDeduplicationCache(cacheCapacity, filterCapacity int) *DeduplicationCache {
	return &DeduplicationCache{
		lruCache:     NewLRUCache(cacheCapacity),
		cuckooFilter: NewSimpleCuckooFilter(filterCapacity),
	}
}

// GetChunkMetadata retrieves chunk metadata from the cache
func (dc *DeduplicationCache) GetChunkMetadata(fingerprint string) (*ChunkMetadata, bool) {
	return dc.lruCache.Get(fingerprint)
}

// PutChunkMetadata adds chunk metadata to the cache
func (dc *DeduplicationCache) PutChunkMetadata(fingerprint string, metadata *ChunkMetadata) {
	dc.lruCache.Put(fingerprint, metadata)
	dc.cuckooFilter.Add(fingerprint)
}

// MightContain checks if a fingerprint might be in the cache (fast check)
func (dc *DeduplicationCache) MightContain(fingerprint string) bool {
	return dc.cuckooFilter.Contains(fingerprint)
}

// RemoveChunkMetadata removes chunk metadata from the cache
func (dc *DeduplicationCache) RemoveChunkMetadata(fingerprint string) bool {
	dc.cuckooFilter.Remove(fingerprint)
	return dc.lruCache.Remove(fingerprint)
}

// Size returns the current size of the LRU cache
func (dc *DeduplicationCache) Size() int {
	return dc.lruCache.Size()
}

// Clear removes all items from the cache
func (dc *DeduplicationCache) Clear() {
	dc.lruCache.Clear()
	// Note: Cuckoo filter doesn't have a simple clear method in this implementation
}
