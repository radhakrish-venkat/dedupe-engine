package main

import (
	"fmt"
	"log"
	"os"

	"github.com/radhakrishnan.venkat/dedupe-engine/internal/chunking"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run cmd/test-chunking/main.go <file_path>")
	}

	filePath := os.Args[1]

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("Failed to open file %s: %v", filePath, err)
	}
	defer file.Close()

	// Create chunker
	chunker := chunking.NewChunker(64, 1024) // 64B min, 1KB max

	// Chunk the file
	chunks, err := chunker.ChunkFile(file)
	if err != nil {
		log.Fatalf("Failed to chunk file: %v", err)
	}

	fmt.Printf("File: %s\n", filePath)
	fmt.Printf("Total chunks: %d\n", len(chunks))
	fmt.Printf("Total size: %d bytes\n", getTotalSize(chunks))
	fmt.Printf("Deduplication ratio: %.2f%%\n", calculateDeduplicationRatio(chunks))

	fmt.Println("\nChunk details:")
	for i, chunk := range chunks {
		fmt.Printf("  Chunk %d: size=%d, fingerprint=%s\n", i, chunk.Size, chunk.Fingerprint[:16]+"...")
	}
}

func getTotalSize(chunks []chunking.Chunk) int64 {
	var total int64
	for _, chunk := range chunks {
		total += chunk.Size
	}
	return total
}

func calculateDeduplicationRatio(chunks []chunking.Chunk) float64 {
	if len(chunks) == 0 {
		return 0
	}

	uniqueFingerprints := make(map[string]bool)
	for _, chunk := range chunks {
		uniqueFingerprints[chunk.Fingerprint] = true
	}

	originalSize := getTotalSize(chunks)
	uniqueSize := int64(len(uniqueFingerprints)) * chunks[0].Size // Approximate

	if originalSize == 0 {
		return 0
	}

	return float64(originalSize-uniqueSize) / float64(originalSize) * 100
}
