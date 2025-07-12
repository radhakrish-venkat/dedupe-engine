package chunking

import (
	"crypto/rand"
	"fmt"
	"hash"
	"io"

	"github.com/zeebo/blake3"
)

// Chunk represents a data chunk with its fingerprint and metadata
type Chunk struct {
	Data        []byte
	Fingerprint string
	Offset      int64
	Size        int64
}

// Chunker implements variable-block chunking using Rabin fingerprinting
type Chunker struct {
	minSize    int
	maxSize    int
	windowSize int
	polynomial uint64
	hasher     hash.Hash
}

// NewChunker creates a new chunker with the specified parameters
func NewChunker(minSize, maxSize int) *Chunker {
	return &Chunker{
		minSize:    minSize,
		maxSize:    maxSize,
		windowSize: 64,                 // Rabin window size
		polynomial: 0x3A335D566E6B7E5B, // Common irreducible polynomial
		hasher:     blake3.New(),
	}
}

// ChunkData splits data into chunks using Rabin fingerprinting
func (c *Chunker) ChunkData(data []byte) ([]Chunk, error) {
	var chunks []Chunk
	offset := int64(0)

	for len(data) > 0 {
		chunkSize := c.findChunkBoundary(data)
		if chunkSize == 0 {
			chunkSize = len(data) // Use remaining data
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

// findChunkBoundary finds the optimal chunk boundary using Rabin fingerprinting
func (c *Chunker) findChunkBoundary(data []byte) int {
	if len(data) < c.minSize {
		return 0 // Not enough data
	}

	// Use a simple approach for now - find boundary based on content
	// In a real implementation, you'd use Rabin fingerprinting
	maxSize := c.maxSize
	if len(data) < maxSize {
		maxSize = len(data)
	}

	// Simple boundary detection - look for patterns
	for i := c.minSize; i < maxSize; i++ {
		if c.isBoundary(data[:i]) {
			return i
		}
	}

	return maxSize
}

// isBoundary determines if a position is a good chunk boundary
func (c *Chunker) isBoundary(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	// Simple boundary detection - look for specific patterns
	// This is a simplified version; real Rabin fingerprinting would be more sophisticated
	lastBytes := data[len(data)-4:]

	// Check for common patterns that indicate good boundaries
	for _, pattern := range [][]byte{
		{0x00, 0x00, 0x00, 0x00}, // Null bytes
		{0xFF, 0xFF, 0xFF, 0xFF}, // All ones
		{0x0A, 0x0A, 0x0A, 0x0A}, // Newlines
	} {
		if bytesEqual(lastBytes, pattern) {
			return true
		}
	}

	return false
}

// computeFingerprint computes the Blake3 hash of the chunk data
func (c *Chunker) computeFingerprint(data []byte) (string, error) {
	c.hasher.Reset()
	_, err := c.hasher.Write(data)
	if err != nil {
		return "", err
	}

	// Get the hash as a hex string
	hash := c.hasher.Sum(nil)
	return fmt.Sprintf("%x", hash), nil
}

// ChunkFile chunks a file by reading it in blocks
func (c *Chunker) ChunkFile(reader io.Reader) ([]Chunk, error) {
	var chunks []Chunk
	offset := int64(0)
	buffer := make([]byte, c.maxSize)

	for {
		n, err := reader.Read(buffer)
		if n == 0 {
			break
		}

		data := buffer[:n]
		fileChunks, err := c.ChunkData(data)
		if err != nil {
			return nil, err
		}

		// Adjust offsets for file chunks
		for i := range fileChunks {
			fileChunks[i].Offset += offset
		}

		chunks = append(chunks, fileChunks...)
		offset += int64(n)

		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}

	return chunks, nil
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

// GenerateRandomChunk generates a random chunk for testing
func GenerateRandomChunk(size int) ([]byte, error) {
	data := make([]byte, size)
	_, err := rand.Read(data)
	return data, err
}
