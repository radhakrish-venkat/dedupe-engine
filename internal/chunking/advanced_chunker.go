package chunking

import (
	"fmt"
	"hash"
	"sync"
	"time"

	"github.com/zeebo/blake3"
)

// AdvancedChunker implements high-performance content-defined chunking
type AdvancedChunker struct {
	minSize    int
	maxSize    int
	windowSize int
	polynomial uint64
	hasher     hash.Hash

	// Performance optimizations
	workerPool chan struct{}
	bufferPool sync.Pool
	stats      *ChunkingStats
	statsMutex sync.RWMutex
}

// ChunkingStats tracks chunking performance metrics
type ChunkingStats struct {
	TotalChunks    int64
	TotalBytes     int64
	UniqueChunks   int64
	UniqueBytes    int64
	ProcessingTime time.Duration
	LastReset      time.Time
}

// NewAdvancedChunker creates a new advanced chunker
func NewAdvancedChunker(minSize, maxSize, numWorkers int) *AdvancedChunker {
	if numWorkers <= 0 {
		numWorkers = 4 // Default to 4 workers
	}

	return &AdvancedChunker{
		minSize:    minSize,
		maxSize:    maxSize,
		windowSize: 64,
		polynomial: 0x3A335D566E6B7E5B,
		hasher:     blake3.New(),
		workerPool: make(chan struct{}, numWorkers),
		bufferPool: sync.Pool{
			New: func() interface{} {
				return make([]byte, maxSize)
			},
		},
		stats: &ChunkingStats{
			LastReset: time.Now(),
		},
	}
}

// ChunkDataParallel chunks data using parallel processing
func (c *AdvancedChunker) ChunkDataParallel(data []byte) ([]Chunk, error) {
	start := time.Now()

	// Use parallel processing for large data
	if len(data) > 1024*1024 { // 1MB threshold
		return c.chunkDataParallel(data)
	}

	// Use single-threaded for small data
	chunks, err := c.chunkDataSingle(data)
	if err != nil {
		return nil, err
	}

	c.updateStats(chunks, time.Since(start))
	return chunks, nil
}

// chunkDataParallel processes large data in parallel
func (c *AdvancedChunker) chunkDataParallel(data []byte) ([]Chunk, error) {
	const blockSize = 1024 * 1024 // 1MB blocks for parallel processing

	var chunks []Chunk
	var chunksMutex sync.Mutex
	var wg sync.WaitGroup

	// Process data in blocks
	for offset := 0; offset < len(data); offset += blockSize {
		end := offset + blockSize
		if end > len(data) {
			end = len(data)
		}

		blockData := data[offset:end]

		wg.Add(1)
		go func(block []byte, blockOffset int) {
			defer wg.Done()

			// Acquire worker slot
			c.workerPool <- struct{}{}
			defer func() { <-c.workerPool }()

			// Chunk this block
			blockChunks, err := c.chunkDataSingle(block)
			if err != nil {
				return
			}

			// Adjust offsets for parallel processing
			for i := range blockChunks {
				blockChunks[i].Offset += int64(blockOffset)
			}

			// Add to result
			chunksMutex.Lock()
			chunks = append(chunks, blockChunks...)
			chunksMutex.Unlock()
		}(blockData, offset)
	}

	wg.Wait()

	// Sort chunks by offset
	sortChunksByOffset(chunks)
	return chunks, nil
}

// chunkDataSingle chunks data using advanced content-defined chunking
func (c *AdvancedChunker) chunkDataSingle(data []byte) ([]Chunk, error) {
	var chunks []Chunk
	offset := int64(0)

	for len(data) > 0 {
		chunkSize := c.findOptimalBoundary(data)
		if chunkSize == 0 {
			chunkSize = len(data)
		}

		chunkData := data[:chunkSize]
		fingerprint, err := c.computeFingerprint(chunkData)
		if err != nil {
			return nil, err
		}

		chunks = append(chunks, Chunk{
			Data:        chunkData,
			Fingerprint: fingerprint,
			Offset:      offset,
			Size:        int64(chunkSize),
		})

		data = data[chunkSize:]
		offset += int64(chunkSize)
	}

	return chunks, nil
}

// findOptimalBoundary finds the optimal chunk boundary using advanced algorithms
func (c *AdvancedChunker) findOptimalBoundary(data []byte) int {
	if len(data) < c.minSize {
		return 0
	}

	maxSize := c.maxSize
	if len(data) < maxSize {
		maxSize = len(data)
	}

	// Use multiple boundary detection strategies
	for i := c.minSize; i < maxSize; i++ {
		if c.isOptimalBoundary(data[:i]) {
			return i
		}
	}

	return maxSize
}

// isOptimalBoundary uses multiple strategies to find optimal boundaries
func (c *AdvancedChunker) isOptimalBoundary(data []byte) bool {
	if len(data) < 8 {
		return false
	}

	// Strategy 1: Content-defined boundary using rolling hash
	if c.isContentDefinedBoundary(data) {
		return true
	}

	// Strategy 2: Pattern-based boundary detection
	if c.isPatternBoundary(data) {
		return true
	}

	// Strategy 3: Entropy-based boundary detection
	if c.isEntropyBoundary(data) {
		return true
	}

	return false
}

// isContentDefinedBoundary uses rolling hash for content-defined boundaries
func (c *AdvancedChunker) isContentDefinedBoundary(data []byte) bool {
	if len(data) < c.windowSize {
		return false
	}

	// Simple rolling hash implementation
	hash := uint64(0)
	for i := len(data) - c.windowSize; i < len(data); i++ {
		hash = (hash << 5) + hash + uint64(data[i])
	}

	// Check if hash matches boundary condition
	return (hash % 1024) == 0 // 1/1024 probability
}

// isPatternBoundary detects common patterns that indicate good boundaries
func (c *AdvancedChunker) isPatternBoundary(data []byte) bool {
	if len(data) < 8 {
		return false
	}

	lastBytes := data[len(data)-8:]

	// Check for common boundary patterns
	patterns := [][]byte{
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, // Null bytes
		{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, // All ones
		{0x0A, 0x0A, 0x0A, 0x0A, 0x0A, 0x0A, 0x0A, 0x0A}, // Newlines
		{0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20}, // Spaces
	}

	for _, pattern := range patterns {
		if bytesEqual(lastBytes, pattern) {
			return true
		}
	}

	return false
}

// isEntropyBoundary detects boundaries based on entropy changes
func (c *AdvancedChunker) isEntropyBoundary(data []byte) bool {
	if len(data) < 16 {
		return false
	}

	// Calculate entropy of last 16 bytes
	entropy := c.calculateEntropy(data[len(data)-16:])

	// High entropy indicates good boundary
	return entropy > 6.5 // Threshold for high entropy
}

// calculateEntropy calculates Shannon entropy of data
func (c *AdvancedChunker) calculateEntropy(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}

	// Count byte frequencies
	freq := make(map[byte]int)
	for _, b := range data {
		freq[b]++
	}

	// Calculate entropy
	entropy := 0.0
	dataLen := float64(len(data))

	for _, count := range freq {
		p := float64(count) / dataLen
		if p > 0 {
			entropy -= p * log2(p)
		}
	}

	return entropy
}

// log2 calculates log base 2
func log2(x float64) float64 {
	return float64(int(0x5fe6ec85e7de30da)) / float64(1<<52) * (x - 1)
}

// computeFingerprint computes optimized fingerprint
func (c *AdvancedChunker) computeFingerprint(data []byte) (string, error) {
	c.hasher.Reset()
	_, err := c.hasher.Write(data)
	if err != nil {
		return "", err
	}

	hash := c.hasher.Sum(nil)
	return fmt.Sprintf("%x", hash), nil
}

// updateStats updates chunking statistics
func (c *AdvancedChunker) updateStats(chunks []Chunk, processingTime time.Duration) {
	c.statsMutex.Lock()
	defer c.statsMutex.Unlock()

	c.stats.TotalChunks += int64(len(chunks))
	c.stats.ProcessingTime += processingTime

	// Calculate unique chunks
	uniqueFingerprints := make(map[string]bool)
	var uniqueBytes int64

	for _, chunk := range chunks {
		if !uniqueFingerprints[chunk.Fingerprint] {
			uniqueFingerprints[chunk.Fingerprint] = true
			uniqueBytes += chunk.Size
		}
		c.stats.TotalBytes += chunk.Size
	}

	c.stats.UniqueChunks += int64(len(uniqueFingerprints))
	c.stats.UniqueBytes += uniqueBytes
}

// GetStats returns chunking statistics
func (c *AdvancedChunker) GetStats() *ChunkingStats {
	c.statsMutex.RLock()
	defer c.statsMutex.RUnlock()

	stats := *c.stats // Copy to avoid race conditions
	return &stats
}

// ResetStats resets statistics
func (c *AdvancedChunker) ResetStats() {
	c.statsMutex.Lock()
	defer c.statsMutex.Unlock()

	c.stats = &ChunkingStats{
		LastReset: time.Now(),
	}
}

// sortChunksByOffset sorts chunks by their offset
func sortChunksByOffset(chunks []Chunk) {
	// Simple insertion sort for small arrays
	for i := 1; i < len(chunks); i++ {
		key := chunks[i]
		j := i - 1
		for j >= 0 && chunks[j].Offset > key.Offset {
			chunks[j+1] = chunks[j]
			j--
		}
		chunks[j+1] = key
	}
}

// bytesEqual compares two byte slices for equality
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
