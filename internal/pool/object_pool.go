package pool

import (
	"sync"
	"time"
)

// ObjectPool provides a generic object pool for reusing objects
type ObjectPool struct {
	pool        sync.Pool
	maxSize     int
	currentSize int
	mutex       sync.RWMutex
	stats       *PoolStats
}

// PoolStats tracks object pool statistics
type PoolStats struct {
	TotalAllocated int64
	TotalReused    int64
	TotalReturned  int64
	CurrentSize    int
	MaxSize        int
	LastReset      time.Time
}

// NewObjectPool creates a new object pool
func NewObjectPool(newFunc func() interface{}, maxSize int) *ObjectPool {
	return &ObjectPool{
		pool: sync.Pool{
			New: newFunc,
		},
		maxSize: maxSize,
		stats: &PoolStats{
			LastReset: time.Now(),
		},
	}
}

// Get retrieves an object from the pool
func (op *ObjectPool) Get() interface{} {
	op.mutex.Lock()
	op.stats.TotalAllocated++
	op.mutex.Unlock()

	obj := op.pool.Get()

	op.mutex.Lock()
	if obj != nil {
		op.stats.TotalReused++
	}
	op.mutex.Unlock()

	return obj
}

// Put returns an object to the pool
func (op *ObjectPool) Put(obj interface{}) {
	if obj == nil {
		return
	}

	op.mutex.Lock()
	defer op.mutex.Unlock()

	if op.currentSize < op.maxSize {
		op.pool.Put(obj)
		op.currentSize++
		op.stats.TotalReturned++
	}
}

// GetStats returns pool statistics
func (op *ObjectPool) GetStats() *PoolStats {
	op.mutex.RLock()
	defer op.mutex.RUnlock()

	stats := *op.stats // Copy to avoid race conditions
	stats.CurrentSize = op.currentSize
	stats.MaxSize = op.maxSize
	return &stats
}

// Reset clears the pool and resets statistics
func (op *ObjectPool) Reset() {
	op.mutex.Lock()
	defer op.mutex.Unlock()

	// Clear the pool by creating a new one
	op.pool = sync.Pool{
		New: op.pool.New,
	}
	op.currentSize = 0
	op.stats = &PoolStats{
		LastReset: time.Now(),
	}
}

// ChunkPool provides a specialized pool for chunk objects
type ChunkPool struct {
	*ObjectPool
}

// Chunk represents a chunk object that can be pooled
type Chunk struct {
	Data        []byte
	Fingerprint string
	Offset      int64
	Size        int64
}

// NewChunkPool creates a new chunk pool
func NewChunkPool(maxSize int) *ChunkPool {
	return &ChunkPool{
		ObjectPool: NewObjectPool(func() interface{} {
			return &Chunk{
				Data: make([]byte, 0, 8192), // Pre-allocated buffer
			}
		}, maxSize),
	}
}

// GetChunk retrieves a chunk from the pool
func (cp *ChunkPool) GetChunk() *Chunk {
	obj := cp.Get()
	if obj == nil {
		return &Chunk{
			Data: make([]byte, 0, 8192),
		}
	}
	return obj.(*Chunk)
}

// PutChunk returns a chunk to the pool
func (cp *ChunkPool) PutChunk(chunk *Chunk) {
	if chunk == nil {
		return
	}

	// Reset the chunk for reuse
	chunk.Data = chunk.Data[:0] // Clear data but keep capacity
	chunk.Fingerprint = ""
	chunk.Offset = 0
	chunk.Size = 0

	cp.Put(chunk)
}

// MetadataPool provides a specialized pool for chunk metadata objects
type MetadataPool struct {
	*ObjectPool
}

// ChunkMetadata represents chunk metadata that can be pooled
type ChunkMetadata struct {
	Fingerprint        string
	StorageLocation    string
	Size               int64
	CreationTime       time.Time
	LastReferencedTime time.Time
}

// NewMetadataPool creates a new metadata pool
func NewMetadataPool(maxSize int) *MetadataPool {
	return &MetadataPool{
		ObjectPool: NewObjectPool(func() interface{} {
			return &ChunkMetadata{}
		}, maxSize),
	}
}

// GetMetadata retrieves metadata from the pool
func (mp *MetadataPool) GetMetadata() *ChunkMetadata {
	obj := mp.Get()
	if obj == nil {
		return &ChunkMetadata{}
	}
	return obj.(*ChunkMetadata)
}

// PutMetadata returns metadata to the pool
func (mp *MetadataPool) PutMetadata(metadata *ChunkMetadata) {
	if metadata == nil {
		return
	}

	// Reset the metadata for reuse
	metadata.Fingerprint = ""
	metadata.StorageLocation = ""
	metadata.Size = 0
	metadata.CreationTime = time.Time{}
	metadata.LastReferencedTime = time.Time{}

	mp.Put(metadata)
}

// BufferPool provides a specialized pool for byte buffers
type BufferPool struct {
	*ObjectPool
	initialSize int
}

// NewBufferPool creates a new buffer pool
func NewBufferPool(initialSize, maxSize int) *BufferPool {
	return &BufferPool{
		ObjectPool: NewObjectPool(func() interface{} {
			return make([]byte, 0, initialSize)
		}, maxSize),
		initialSize: initialSize,
	}
}

// GetBuffer retrieves a buffer from the pool
func (bp *BufferPool) GetBuffer() []byte {
	obj := bp.Get()
	if obj == nil {
		return make([]byte, 0, bp.initialSize)
	}
	return obj.([]byte)
}

// PutBuffer returns a buffer to the pool
func (bp *BufferPool) PutBuffer(buffer []byte) {
	if buffer == nil {
		return
	}

	// Reset the buffer for reuse
	buffer = buffer[:0] // Clear data but keep capacity
	bp.Put(buffer)
}

// GlobalPools provides access to global object pools
type GlobalPools struct {
	ChunkPool    *ChunkPool
	MetadataPool *MetadataPool
	BufferPool   *BufferPool
}

// NewGlobalPools creates global object pools
func NewGlobalPools() *GlobalPools {
	return &GlobalPools{
		ChunkPool:    NewChunkPool(1000),
		MetadataPool: NewMetadataPool(1000),
		BufferPool:   NewBufferPool(8192, 1000),
	}
}

// GetGlobalPools returns the global pools instance
var globalPools *GlobalPools
var globalPoolsOnce sync.Once

func GetGlobalPools() *GlobalPools {
	globalPoolsOnce.Do(func() {
		globalPools = NewGlobalPools()
	})
	return globalPools
}

// ResetGlobalPools resets all global pools
func ResetGlobalPools() {
	if globalPools != nil {
		globalPools.ChunkPool.Reset()
		globalPools.MetadataPool.Reset()
		globalPools.BufferPool.Reset()
	}
}
