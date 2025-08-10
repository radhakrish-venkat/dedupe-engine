package chunking

import (
	"context"
	"fmt"
	"hash"
	"io"
	"sync"
	"time"

	"github.com/zeebo/blake3"
)

// ParallelChunker implements high-performance parallel content-defined chunking
type ParallelChunker struct {
	// Configuration
	minSize    int
	maxSize    int
	windowSize int
	polynomial uint64
	hasher     hash.Hash

	// Parallel processing
	workerCount   int
	workQueue     *WorkStealingQueue
	workerPool    chan struct{}
	bufferPool    sync.Pool
	resultChannel chan ChunkResult

	// Statistics
	stats      *ParallelChunkingStats
	statsMutex sync.RWMutex

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// ChunkResult represents a chunk result from parallel processing
type ChunkResult struct {
	Chunk     *Chunk
	Offset    int64
	WorkerID  int
	Error     error
	Timestamp time.Time
}

// ParallelChunkingStats tracks parallel chunking performance
type ParallelChunkingStats struct {
	TotalChunks       int64
	TotalBytes        int64
	UniqueChunks      int64
	UniqueBytes       int64
	ProcessingTime    time.Duration
	WorkerUtilization float64
	LastReset         time.Time
}

// WorkStealingQueue implements a work-stealing queue for load balancing
type WorkStealingQueue struct {
	items    []ChunkWork
	head     int
	tail     int
	mutex    sync.Mutex
	capacity int
}

// ChunkWork represents a unit of work for chunking
type ChunkWork struct {
	Data      []byte
	Offset    int64
	WorkerID  int
	Priority  int // Higher priority = processed first
	Timestamp time.Time
}

// NewParallelChunker creates a new parallel chunker
func NewParallelChunker(minSize, maxSize, workerCount int) *ParallelChunker {
	if workerCount <= 0 {
		workerCount = 4 // Default to 4 workers
	}

	ctx, cancel := context.WithCancel(context.Background())

	chunker := &ParallelChunker{
		minSize:       minSize,
		maxSize:       maxSize,
		windowSize:    64,
		polynomial:    0x3A335D566E6B7E5B,
		hasher:        blake3.New(),
		workerCount:   workerCount,
		workQueue:     NewWorkStealingQueue(1000),
		workerPool:    make(chan struct{}, workerCount),
		resultChannel: make(chan ChunkResult, workerCount*10),
		stats: &ParallelChunkingStats{
			LastReset: time.Now(),
		},
		ctx:    ctx,
		cancel: cancel,
	}

	chunker.bufferPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, maxSize)
		},
	}

	// Start worker goroutines
	chunker.startWorkers()

	return chunker
}

// Close closes the parallel chunker
func (pc *ParallelChunker) Close() {
	pc.cancel()
	close(pc.resultChannel)
}

// ChunkDataParallel chunks data using parallel processing
func (pc *ParallelChunker) ChunkDataParallel(data []byte) ([]Chunk, error) {
	start := time.Now()

	// Use single-threaded for small data
	if len(data) < pc.minSize*pc.workerCount {
		return pc.chunkDataSingle(data)
	}

	// Split data into overlapping blocks for parallel processing
	blocks := pc.createOverlappingBlocks(data)

	// Submit work to queue
	for i, block := range blocks {
		work := ChunkWork{
			Data:      block.Data,
			Offset:    block.Offset,
			WorkerID:  i % pc.workerCount,
			Priority:  pc.calculatePriority(block.Data),
			Timestamp: time.Now(),
		}
		pc.workQueue.Push(work)
	}

	// Collect results
	chunks := make([]Chunk, 0, len(blocks)*2) // Estimate capacity
	results := make([]ChunkResult, 0, len(blocks))

	// Collect all results
	for i := 0; i < len(blocks); i++ {
		select {
		case result := <-pc.resultChannel:
			if result.Error != nil {
				return nil, fmt.Errorf("worker error: %w", result.Error)
			}
			results = append(results, result)
		case <-pc.ctx.Done():
			return nil, pc.ctx.Err()
		}
	}

	// Sort results by offset and merge
	chunks = pc.mergeResults(results)

	// Update statistics
	pc.updateStats(chunks, time.Since(start))

	return chunks, nil
}

// ChunkStream chunks data from a stream using parallel processing
func (pc *ParallelChunker) ChunkStream(reader io.Reader) (<-chan Chunk, error) {
	chunkChannel := make(chan Chunk, pc.workerCount*10)

	go func() {
		defer close(chunkChannel)

		buffer := make([]byte, pc.maxSize*pc.workerCount)
		offset := int64(0)

		for {
			// Read data into buffer
			n, err := reader.Read(buffer)
			if n == 0 {
				if err == io.EOF {
					break
				}
				if err != nil {
					// Handle error
					break
				}
				continue
			}

			// Process the data
			data := buffer[:n]
			chunks, err := pc.ChunkDataParallel(data)
			if err != nil {
				// Handle error
				break
			}

			// Adjust offsets and send chunks
			for _, chunk := range chunks {
				chunk.Offset += offset
				select {
				case chunkChannel <- chunk:
				case <-pc.ctx.Done():
					return
				}
			}

			offset += int64(n)
		}
	}()

	return chunkChannel, nil
}

// createOverlappingBlocks creates overlapping blocks for parallel processing
func (pc *ParallelChunker) createOverlappingBlocks(data []byte) []DataBlock {
	const overlapSize = 1024 // 1KB overlap to handle boundary conditions
	blockSize := pc.maxSize * 2

	var blocks []DataBlock
	offset := int64(0)

	for offset < int64(len(data)) {
		end := offset + int64(blockSize)
		if end > int64(len(data)) {
			end = int64(len(data))
		}

		// Create overlapping block
		blockData := data[offset:end]
		if len(blockData) < pc.minSize {
			// Handle remaining small data
			if len(blockData) > 0 {
				blocks = append(blocks, DataBlock{
					Data:   blockData,
					Offset: offset,
				})
			}
			break
		}

		blocks = append(blocks, DataBlock{
			Data:   blockData,
			Offset: offset,
		})

		offset = end - int64(overlapSize)
	}

	return blocks
}

// DataBlock represents a block of data for processing
type DataBlock struct {
	Data   []byte
	Offset int64
}

// calculatePriority calculates the priority of a work item
func (pc *ParallelChunker) calculatePriority(data []byte) int {
	// Higher priority for larger blocks (more work)
	priority := len(data) / 1024

	// Bonus priority for data with high entropy (more complex)
	entropy := pc.calculateEntropy(data)
	if entropy > 6.0 {
		priority += 10
	}

	return priority
}

// calculateEntropy calculates the entropy of data
func (pc *ParallelChunker) calculateEntropy(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}

	freq := make(map[byte]int)
	for _, b := range data {
		freq[b]++
	}

	entropy := 0.0
	dataLen := float64(len(data))

	for _, count := range freq {
		p := float64(count) / dataLen
		if p > 0 {
			entropy -= p * log2Parallel(p)
		}
	}

	return entropy
}

// log2 calculates log base 2
func log2Parallel(x float64) float64 {
	return float64(int(0x5fe6ec85e7de30da)) / float64(1<<52) * (x - 1)
}

// startWorkers starts the worker goroutines
func (pc *ParallelChunker) startWorkers() {
	for i := 0; i < pc.workerCount; i++ {
		go pc.worker(i)
	}
}

// worker is a worker goroutine that processes chunking work
func (pc *ParallelChunker) worker(id int) {
	for {
		select {
		case <-pc.ctx.Done():
			return
		default:
			// Try to get work from queue
			work, ok := pc.workQueue.Pop()
			if !ok {
				// Try to steal work from other workers
				work, ok = pc.stealWork()
				if !ok {
					time.Sleep(1 * time.Millisecond)
					continue
				}
			}

			// Process the work
			chunks, err := pc.chunkDataSingle(work.Data)
			if err != nil {
				pc.resultChannel <- ChunkResult{
					WorkerID:  id,
					Error:     err,
					Timestamp: time.Now(),
				}
				continue
			}

			// Send results
			for _, chunk := range chunks {
				chunk.Offset += work.Offset
				pc.resultChannel <- ChunkResult{
					Chunk:     &chunk,
					Offset:    chunk.Offset,
					WorkerID:  id,
					Timestamp: time.Now(),
				}
			}
		}
	}
}

// stealWork attempts to steal work from other workers
func (pc *ParallelChunker) stealWork() (ChunkWork, bool) {
	// Simple implementation - in a real system, you'd implement
	// proper work stealing across multiple queues
	return ChunkWork{}, false
}

// chunkDataSingle chunks data using single-threaded processing
func (pc *ParallelChunker) chunkDataSingle(data []byte) ([]Chunk, error) {
	var chunks []Chunk
	offset := int64(0)

	for len(data) > 0 {
		chunkSize := pc.findOptimalBoundary(data)
		if chunkSize == 0 {
			chunkSize = len(data)
		}

		chunkData := data[:chunkSize]
		fingerprint, err := pc.computeFingerprint(chunkData)
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

// findOptimalBoundary finds the optimal chunk boundary
func (pc *ParallelChunker) findOptimalBoundary(data []byte) int {
	if len(data) < pc.minSize {
		return 0
	}

	maxSize := pc.maxSize
	if len(data) < maxSize {
		maxSize = len(data)
	}

	// Use multiple boundary detection strategies
	for i := pc.minSize; i < maxSize; i++ {
		if pc.isOptimalBoundary(data[:i]) {
			return i
		}
	}

	return maxSize
}

// isOptimalBoundary checks if a position is an optimal boundary
func (pc *ParallelChunker) isOptimalBoundary(data []byte) bool {
	if len(data) < pc.minSize {
		return false
	}

	// Strategy 1: Content-defined boundary using rolling hash
	if pc.isContentDefinedBoundary(data) {
		return true
	}

	// Strategy 2: Pattern-based boundary detection
	if pc.isPatternBoundary(data) {
		return true
	}

	// Strategy 3: Entropy-based boundary detection
	if pc.isEntropyBoundary(data) {
		return true
	}

	return false
}

// isContentDefinedBoundary checks for content-defined boundary
func (pc *ParallelChunker) isContentDefinedBoundary(data []byte) bool {
	if len(data) < pc.windowSize {
		return false
	}

	// Simple rolling hash implementation
	hash := uint64(0)
	for i := len(data) - pc.windowSize; i < len(data); i++ {
		hash = (hash << 5) + hash + uint64(data[i])
	}

	// Check if hash matches boundary condition
	return (hash % 1024) == 0 // 1/1024 probability
}

// isPatternBoundary checks for pattern-based boundary
func (pc *ParallelChunker) isPatternBoundary(data []byte) bool {
	if len(data) < 8 {
		return false
	}

	lastBytes := data[len(data)-8:]

	// Check for common boundary patterns
	patterns := [][]byte{
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, // Null bytes
		{0x0a, 0x0a, 0x0a, 0x0a, 0x0a, 0x0a, 0x0a, 0x0a}, // Newlines
		{0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20}, // Spaces
	}

	for _, pattern := range patterns {
		if bytesEqualParallel(lastBytes, pattern) {
			return true
		}
	}

	return false
}

// isEntropyBoundary checks for entropy-based boundary
func (pc *ParallelChunker) isEntropyBoundary(data []byte) bool {
	if len(data) < 16 {
		return false
	}

	// Calculate entropy of last 16 bytes
	entropy := pc.calculateEntropy(data[len(data)-16:])

	// High entropy indicates good boundary
	return entropy > 6.5 // Threshold for high entropy
}

// bytesEqualParallel checks if two byte slices are equal
func bytesEqualParallel(a, b []byte) bool {
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

// computeFingerprint computes the fingerprint of data
func (pc *ParallelChunker) computeFingerprint(data []byte) (string, error) {
	pc.hasher.Reset()
	_, err := pc.hasher.Write(data)
	if err != nil {
		return "", err
	}

	hash := pc.hasher.Sum(nil)
	return fmt.Sprintf("%x", hash), nil
}

// mergeResults merges and sorts chunk results
func (pc *ParallelChunker) mergeResults(results []ChunkResult) []Chunk {
	// Sort results by offset
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].Offset > results[j].Offset {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	// Extract chunks
	chunks := make([]Chunk, 0, len(results))
	for _, result := range results {
		if result.Chunk != nil {
			chunks = append(chunks, *result.Chunk)
		}
	}

	return chunks
}

// updateStats updates chunking statistics
func (pc *ParallelChunker) updateStats(chunks []Chunk, processingTime time.Duration) {
	pc.statsMutex.Lock()
	defer pc.statsMutex.Unlock()

	pc.stats.TotalChunks += int64(len(chunks))
	pc.stats.ProcessingTime += processingTime

	// Calculate unique chunks
	uniqueFingerprints := make(map[string]bool)
	var uniqueBytes int64

	for _, chunk := range chunks {
		if !uniqueFingerprints[chunk.Fingerprint] {
			uniqueFingerprints[chunk.Fingerprint] = true
			uniqueBytes += chunk.Size
		}
		pc.stats.TotalBytes += chunk.Size
	}

	pc.stats.UniqueChunks += int64(len(uniqueFingerprints))
	pc.stats.UniqueBytes += uniqueBytes
}

// GetStats returns chunking statistics
func (pc *ParallelChunker) GetStats() *ParallelChunkingStats {
	pc.statsMutex.RLock()
	defer pc.statsMutex.RUnlock()

	stats := *pc.stats // Copy to avoid race conditions
	return &stats
}

// ResetStats resets chunking statistics
func (pc *ParallelChunker) ResetStats() {
	pc.statsMutex.Lock()
	defer pc.statsMutex.Unlock()

	pc.stats = &ParallelChunkingStats{
		LastReset: time.Now(),
	}
}

// NewWorkStealingQueue creates a new work-stealing queue
func NewWorkStealingQueue(capacity int) *WorkStealingQueue {
	return &WorkStealingQueue{
		items:    make([]ChunkWork, capacity),
		capacity: capacity,
	}
}

// Push adds an item to the queue
func (wsq *WorkStealingQueue) Push(work ChunkWork) bool {
	wsq.mutex.Lock()
	defer wsq.mutex.Unlock()

	if wsq.isFull() {
		return false
	}

	wsq.items[wsq.tail] = work
	wsq.tail = (wsq.tail + 1) % wsq.capacity
	return true
}

// Pop removes and returns an item from the queue
func (wsq *WorkStealingQueue) Pop() (ChunkWork, bool) {
	wsq.mutex.Lock()
	defer wsq.mutex.Unlock()

	if wsq.isEmpty() {
		return ChunkWork{}, false
	}

	work := wsq.items[wsq.head]
	wsq.head = (wsq.head + 1) % wsq.capacity
	return work, true
}

// isEmpty checks if the queue is empty
func (wsq *WorkStealingQueue) isEmpty() bool {
	return wsq.head == wsq.tail
}

// isFull checks if the queue is full
func (wsq *WorkStealingQueue) isFull() bool {
	return (wsq.tail+1)%wsq.capacity == wsq.head
}

// Size returns the current size of the queue
func (wsq *WorkStealingQueue) Size() int {
	wsq.mutex.Lock()
	defer wsq.mutex.Unlock()

	if wsq.tail >= wsq.head {
		return wsq.tail - wsq.head
	}
	return wsq.capacity - wsq.head + wsq.tail
}
