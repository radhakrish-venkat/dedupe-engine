package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/radhakrishnan.venkat/dedupe-engine/pkg/api"
)

func main() {
	fmt.Println("Testing Data Storage Node")
	fmt.Println("========================")

	// Connect to Data Storage Node
	conn, err := grpc.Dial("localhost:50052", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to Data Storage Node: %v", err)
	}
	defer conn.Close()

	client := pb.NewStorageServiceClient(conn)

	// Test data
	testData := []byte("This is test chunk data for deduplication testing.")
	fingerprint := "test-fingerprint-123"

	fmt.Printf("Testing with fingerprint: %s\n", fingerprint)
	fmt.Printf("Data size: %d bytes\n", len(testData))

	// Test StoreChunk
	fmt.Println("\n1. Testing StoreChunk:")
	storeReq := &pb.StoreChunkRequest{
		Fingerprint: fingerprint,
		ChunkData:   testData,
		Size:        int64(len(testData)),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	storeResp, err := client.StoreChunk(ctx, storeReq)
	if err != nil {
		log.Fatalf("Failed to store chunk: %v", err)
	}

	if storeResp.Success {
		fmt.Printf("  ✓ Chunk stored successfully\n")
		fmt.Printf("  Storage location: %s\n", storeResp.StorageLocation)
		fmt.Printf("  Storage node ID: %s\n", storeResp.StorageNodeId)
	} else {
		fmt.Printf("  ✗ Failed to store chunk: %s\n", storeResp.ErrorMessage)
	}

	// Test GetChunk
	fmt.Println("\n2. Testing GetChunk:")
	getReq := &pb.GetChunkRequest{
		Fingerprint: fingerprint,
	}

	getResp, err := client.GetChunk(ctx, getReq)
	if err != nil {
		log.Fatalf("Failed to get chunk: %v", err)
	}

	if getResp.Found {
		fmt.Printf("  ✓ Chunk retrieved successfully\n")
		fmt.Printf("  Retrieved size: %d bytes\n", getResp.Size)
		fmt.Printf("  Data matches: %t\n", string(getResp.ChunkData) == string(testData))
	} else {
		fmt.Printf("  ✗ Chunk not found: %s\n", getResp.ErrorMessage)
	}

	// Test non-existent chunk
	fmt.Println("\n3. Testing GetChunk (non-existent):")
	getReq.Fingerprint = "non-existent-fingerprint"
	getResp, err = client.GetChunk(ctx, getReq)
	if err != nil {
		log.Fatalf("Failed to get non-existent chunk: %v", err)
	}

	if !getResp.Found {
		fmt.Printf("  ✓ Correctly identified non-existent chunk\n")
	} else {
		fmt.Printf("  ✗ Incorrectly found non-existent chunk\n")
	}

	fmt.Println("\nData Storage Node testing completed!")
}
