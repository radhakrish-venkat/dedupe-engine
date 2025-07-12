package chunking

import (
	"strings"
	"testing"
)

func TestChunker(t *testing.T) {
	chunker := NewChunker(1024, 8192) // 1KB min, 8KB max

	// Test data
	testData := []byte("This is a test file with some content that should be chunked properly.")

	chunks, err := chunker.ChunkData(testData)
	if err != nil {
		t.Fatalf("Failed to chunk data: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("Expected at least one chunk")
	}

	// Verify each chunk has a fingerprint
	for i, chunk := range chunks {
		if chunk.Fingerprint == "" {
			t.Errorf("Chunk %d has empty fingerprint", i)
		}
		if len(chunk.Data) == 0 {
			t.Errorf("Chunk %d has empty data", i)
		}
		t.Logf("Chunk %d: size=%d, fingerprint=%s", i, chunk.Size, chunk.Fingerprint)
	}
}

func TestChunkFile(t *testing.T) {
	chunker := NewChunker(1024, 8192)

	// Create a test reader
	testData := strings.Repeat("This is test data that will be chunked. ", 100)
	reader := strings.NewReader(testData)

	chunks, err := chunker.ChunkFile(reader)
	if err != nil {
		t.Fatalf("Failed to chunk file: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("Expected at least one chunk")
	}

	t.Logf("Created %d chunks from test data", len(chunks))
}

func TestGenerateRandomChunk(t *testing.T) {
	size := 1024
	data, err := GenerateRandomChunk(size)
	if err != nil {
		t.Fatalf("Failed to generate random chunk: %v", err)
	}

	if len(data) != size {
		t.Errorf("Expected chunk size %d, got %d", size, len(data))
	}

	// Verify it's not all zeros
	allZeros := true
	for _, b := range data {
		if b != 0 {
			allZeros = false
			break
		}
	}
	if allZeros {
		t.Error("Generated chunk is all zeros")
	}
}
