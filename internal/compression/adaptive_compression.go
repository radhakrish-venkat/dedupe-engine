package compression

import (
	"bytes"
	"compress/gzip"
	"compress/lzw"
	"compress/zlib"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/klauspost/compress/s2"
	"github.com/klauspost/compress/zstd"
)

// CompressionAlgorithm represents different compression algorithms
type CompressionAlgorithm string

const (
	AlgorithmNone CompressionAlgorithm = "none"
	AlgorithmGzip CompressionAlgorithm = "gzip"
	AlgorithmZstd CompressionAlgorithm = "zstd"
	AlgorithmS2   CompressionAlgorithm = "s2"
	AlgorithmLZW  CompressionAlgorithm = "lzw"
	AlgorithmZlib CompressionAlgorithm = "zlib"
)

// CompressionResult holds compression results
type CompressionResult struct {
	Algorithm         CompressionAlgorithm
	OriginalSize      int
	CompressedSize    int
	CompressionRatio  float64
	CompressionTime   time.Duration
	DecompressionTime time.Duration
}

// AdaptiveCompressor chooses the best compression algorithm for data
type AdaptiveCompressor struct {
	// Algorithm performance tracking
	algorithmStats map[CompressionAlgorithm]*AlgorithmStats
	statsMutex     sync.RWMutex

	// Configuration
	config *CompressionConfig

	// Worker pool for parallel compression testing
	workerPool chan struct{}
}

// CompressionConfig holds compression configuration
type CompressionConfig struct {
	EnableAdaptiveCompression bool
	MinSizeForCompression     int // Minimum size to consider compression
	MaxCompressionTime        time.Duration
	PreferredAlgorithms       []CompressionAlgorithm
	CompressionLevel          int
	MaxWorkers                int
}

// AlgorithmStats tracks performance statistics for each algorithm
type AlgorithmStats struct {
	TotalCompressions    int64
	TotalBytes           int64
	TotalCompressedBytes int64
	TotalCompressionTime time.Duration
	AverageRatio         float64
	SuccessRate          float64
	LastUsed             time.Time
}

// NewAdaptiveCompressor creates a new adaptive compressor
func NewAdaptiveCompressor(config *CompressionConfig) *AdaptiveCompressor {
	if config == nil {
		config = &CompressionConfig{
			EnableAdaptiveCompression: true,
			MinSizeForCompression:     1024, // 1KB minimum
			MaxCompressionTime:        100 * time.Millisecond,
			PreferredAlgorithms: []CompressionAlgorithm{
				AlgorithmS2,
				AlgorithmZstd,
				AlgorithmGzip,
			},
			CompressionLevel: 6,
			MaxWorkers:       4,
		}
	}

	return &AdaptiveCompressor{
		algorithmStats: make(map[CompressionAlgorithm]*AlgorithmStats),
		config:         config,
		workerPool:     make(chan struct{}, config.MaxWorkers),
	}
}

// CompressAdaptively chooses the best compression algorithm and compresses data
func (ac *AdaptiveCompressor) CompressAdaptively(data []byte) (*CompressionResult, error) {
	if len(data) < ac.config.MinSizeForCompression {
		// Data too small, return uncompressed
		return &CompressionResult{
			Algorithm:        AlgorithmNone,
			OriginalSize:     len(data),
			CompressedSize:   len(data),
			CompressionRatio: 1.0,
		}, nil
	}

	// Analyze data characteristics
	dataType := ac.analyzeDataType(data)

	// Choose best algorithm based on data type and historical performance
	bestAlgorithm := ac.chooseBestAlgorithm(dataType, len(data))

	// Compress with chosen algorithm
	result, err := ac.compressWithAlgorithm(data, bestAlgorithm)
	if err != nil {
		return nil, err
	}

	// Update statistics
	ac.updateStats(bestAlgorithm, result)

	return result, nil
}

// analyzeDataType analyzes the type of data for optimal algorithm selection
func (ac *AdaptiveCompressor) analyzeDataType(data []byte) string {
	if len(data) == 0 {
		return "empty"
	}

	// Calculate entropy
	entropy := ac.calculateEntropy(data)

	// Check for patterns
	hasRepeatingPatterns := ac.hasRepeatingPatterns(data)
	hasNullBytes := ac.hasNullBytes(data)

	// Determine data type based on characteristics
	if entropy < 3.0 {
		return "low_entropy"
	} else if entropy > 7.0 {
		return "high_entropy"
	} else if hasRepeatingPatterns {
		return "repetitive"
	} else if hasNullBytes {
		return "sparse"
	} else {
		return "mixed"
	}
}

// chooseBestAlgorithm selects the best compression algorithm
func (ac *AdaptiveCompressor) chooseBestAlgorithm(dataType string, dataSize int) CompressionAlgorithm {
	ac.statsMutex.RLock()
	defer ac.statsMutex.RUnlock()

	// Algorithm selection based on data type
	var candidates []CompressionAlgorithm

	switch dataType {
	case "low_entropy":
		// Low entropy data - use fast algorithms
		candidates = []CompressionAlgorithm{AlgorithmS2, AlgorithmZstd, AlgorithmGzip}
	case "high_entropy":
		// High entropy data - compression may not help much
		candidates = []CompressionAlgorithm{AlgorithmZstd, AlgorithmS2, AlgorithmNone}
	case "repetitive":
		// Repetitive data - use algorithms good at finding patterns
		candidates = []CompressionAlgorithm{AlgorithmZstd, AlgorithmGzip, AlgorithmLZW}
	case "sparse":
		// Sparse data - use algorithms good with null bytes
		candidates = []CompressionAlgorithm{AlgorithmS2, AlgorithmZstd, AlgorithmGzip}
	default:
		// Mixed data - use balanced algorithms
		candidates = ac.config.PreferredAlgorithms
	}

	// Score algorithms based on historical performance
	bestAlgorithm := AlgorithmNone
	bestScore := 0.0

	for _, algo := range candidates {
		score := ac.calculateAlgorithmScore(algo, dataSize)
		if score > bestScore {
			bestScore = score
			bestAlgorithm = algo
		}
	}

	return bestAlgorithm
}

// calculateAlgorithmScore calculates a score for an algorithm
func (ac *AdaptiveCompressor) calculateAlgorithmScore(algo CompressionAlgorithm, dataSize int) float64 {
	stats, exists := ac.algorithmStats[algo]
	if !exists {
		// No historical data, use default scores
		defaultScores := map[CompressionAlgorithm]float64{
			AlgorithmS2:   0.8,
			AlgorithmZstd: 0.9,
			AlgorithmGzip: 0.7,
			AlgorithmLZW:  0.6,
			AlgorithmZlib: 0.7,
			AlgorithmNone: 0.5,
		}
		return defaultScores[algo]
	}

	// Calculate score based on historical performance
	compressionRatio := stats.AverageRatio
	successRate := stats.SuccessRate

	// Weighted score: 60% compression ratio, 40% success rate
	score := (compressionRatio * 0.6) + (successRate * 0.4)

	return score
}

// compressWithAlgorithm compresses data with a specific algorithm
func (ac *AdaptiveCompressor) compressWithAlgorithm(data []byte, algo CompressionAlgorithm) (*CompressionResult, error) {
	start := time.Now()

	var compressed []byte
	var err error

	switch algo {
	case AlgorithmNone:
		compressed = data
	case AlgorithmGzip:
		compressed, err = ac.compressGzip(data)
	case AlgorithmZstd:
		compressed, err = ac.compressZstd(data)
	case AlgorithmS2:
		compressed, err = ac.compressS2(data)
	case AlgorithmLZW:
		compressed, err = ac.compressLZW(data)
	case AlgorithmZlib:
		compressed, err = ac.compressZlib(data)
	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", algo)
	}

	if err != nil {
		return nil, err
	}

	compressionTime := time.Since(start)

	// Test decompression time
	decompressionStart := time.Now()
	_, err = ac.decompress(data, algo)
	decompressionTime := time.Since(decompressionStart)

	if err != nil {
		return nil, fmt.Errorf("decompression test failed: %w", err)
	}

	compressionRatio := float64(len(compressed)) / float64(len(data))

	return &CompressionResult{
		Algorithm:         algo,
		OriginalSize:      len(data),
		CompressedSize:    len(compressed),
		CompressionRatio:  compressionRatio,
		CompressionTime:   compressionTime,
		DecompressionTime: decompressionTime,
	}, nil
}

// compressGzip compresses data using gzip
func (ac *AdaptiveCompressor) compressGzip(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buf, ac.config.CompressionLevel)
	if err != nil {
		return nil, err
	}

	_, err = writer.Write(data)
	if err != nil {
		return nil, err
	}

	err = writer.Close()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// compressZstd compresses data using zstd
func (ac *AdaptiveCompressor) compressZstd(data []byte) ([]byte, error) {
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevel(ac.config.CompressionLevel)))
	if err != nil {
		return nil, err
	}

	return encoder.EncodeAll(data, nil), nil
}

// compressS2 compresses data using S2
func (ac *AdaptiveCompressor) compressS2(data []byte) ([]byte, error) {
	return s2.Encode(nil, data), nil
}

// compressLZW compresses data using LZW
func (ac *AdaptiveCompressor) compressLZW(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := lzw.NewWriter(&buf, lzw.LSB, 8)

	_, err := writer.Write(data)
	if err != nil {
		return nil, err
	}

	err = writer.Close()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// compressZlib compresses data using zlib
func (ac *AdaptiveCompressor) compressZlib(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer, err := zlib.NewWriterLevel(&buf, ac.config.CompressionLevel)
	if err != nil {
		return nil, err
	}

	_, err = writer.Write(data)
	if err != nil {
		return nil, err
	}

	err = writer.Close()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// decompress decompresses data (for testing)
func (ac *AdaptiveCompressor) decompress(data []byte, algo CompressionAlgorithm) ([]byte, error) {
	switch algo {
	case AlgorithmNone:
		return data, nil
	case AlgorithmGzip:
		return ac.decompressGzip(data)
	case AlgorithmZstd:
		return ac.decompressZstd(data)
	case AlgorithmS2:
		return ac.decompressS2(data)
	case AlgorithmLZW:
		return ac.decompressLZW(data)
	case AlgorithmZlib:
		return ac.decompressZlib(data)
	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", algo)
	}
}

// Decompression methods (simplified for brevity)
func (ac *AdaptiveCompressor) decompressGzip(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

func (ac *AdaptiveCompressor) decompressZstd(data []byte) ([]byte, error) {
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}

	return decoder.DecodeAll(data, nil)
}

func (ac *AdaptiveCompressor) decompressS2(data []byte) ([]byte, error) {
	return s2.Decode(nil, data)
}

func (ac *AdaptiveCompressor) decompressLZW(data []byte) ([]byte, error) {
	reader := lzw.NewReader(bytes.NewReader(data), lzw.LSB, 8)
	defer reader.Close()

	return io.ReadAll(reader)
}

func (ac *AdaptiveCompressor) decompressZlib(data []byte) ([]byte, error) {
	reader, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

// updateStats updates algorithm statistics
func (ac *AdaptiveCompressor) updateStats(algo CompressionAlgorithm, result *CompressionResult) {
	ac.statsMutex.Lock()
	defer ac.statsMutex.Unlock()

	stats, exists := ac.algorithmStats[algo]
	if !exists {
		stats = &AlgorithmStats{}
		ac.algorithmStats[algo] = stats
	}

	stats.TotalCompressions++
	stats.TotalBytes += int64(result.OriginalSize)
	stats.TotalCompressedBytes += int64(result.CompressedSize)
	stats.TotalCompressionTime += result.CompressionTime
	stats.LastUsed = time.Now()

	// Update average ratio
	stats.AverageRatio = float64(stats.TotalCompressedBytes) / float64(stats.TotalBytes)

	// Update success rate (simplified)
	stats.SuccessRate = 0.95 // Assume 95% success rate
}

// Utility methods for data analysis
func (ac *AdaptiveCompressor) calculateEntropy(data []byte) float64 {
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
			entropy -= p * log2(p)
		}
	}

	return entropy
}

func (ac *AdaptiveCompressor) hasRepeatingPatterns(data []byte) bool {
	if len(data) < 8 {
		return false
	}

	// Simple pattern detection
	for i := 0; i < len(data)-8; i++ {
		pattern := data[i : i+4]
		count := 0
		for j := i + 4; j < len(data)-4; j += 4 {
			if bytes.Equal(pattern, data[j:j+4]) {
				count++
			}
		}
		if count > 2 {
			return true
		}
	}

	return false
}

func (ac *AdaptiveCompressor) hasNullBytes(data []byte) bool {
	nullCount := 0
	for _, b := range data {
		if b == 0 {
			nullCount++
		}
	}

	// Consider sparse if more than 10% are null bytes
	return float64(nullCount)/float64(len(data)) > 0.1
}

// log2 calculates log base 2
func log2(x float64) float64 {
	return float64(int(0x5fe6ec85e7de30da)) / float64(1<<52) * (x - 1)
}

// GetStats returns compression statistics
func (ac *AdaptiveCompressor) GetStats() map[CompressionAlgorithm]*AlgorithmStats {
	ac.statsMutex.RLock()
	defer ac.statsMutex.RUnlock()

	stats := make(map[CompressionAlgorithm]*AlgorithmStats)
	for algo, stat := range ac.algorithmStats {
		statsCopy := *stat
		stats[algo] = &statsCopy
	}

	return stats
}
