package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/radhakrishnan.venkat/dedupe-engine/internal/minio"
	pb "github.com/radhakrishnan.venkat/dedupe-engine/pkg/api"
)

type server struct {
	pb.UnimplementedStorageServiceServer
	minioClient *minio.Client
	nodeID      string
}

func (s *server) StoreChunk(ctx context.Context, req *pb.StoreChunkRequest) (*pb.StoreChunkResponse, error) {
	if req.Fingerprint == "" {
		return nil, status.Error(codes.InvalidArgument, "fingerprint is required")
	}
	if len(req.ChunkData) == 0 {
		return nil, status.Error(codes.InvalidArgument, "chunk_data is required")
	}

	// Store chunk in MinIO
	err := s.minioClient.StoreChunk(ctx, req.Fingerprint, req.ChunkData)
	if err != nil {
		log.Printf("Failed to store chunk %s: %v", req.Fingerprint, err)
		return &pb.StoreChunkResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.StoreChunkResponse{
		StorageLocation: req.Fingerprint, // Use fingerprint as object key
		StorageNodeId:   s.nodeID,
		Success:         true,
	}, nil
}

func (s *server) GetChunk(ctx context.Context, req *pb.GetChunkRequest) (*pb.GetChunkResponse, error) {
	if req.Fingerprint == "" {
		return nil, status.Error(codes.InvalidArgument, "fingerprint is required")
	}

	// Check if chunk exists
	exists, err := s.minioClient.ChunkExists(ctx, req.Fingerprint)
	if err != nil {
		log.Printf("Failed to check chunk existence for %s: %v", req.Fingerprint, err)
		return &pb.GetChunkResponse{
			Found:        false,
			ErrorMessage: err.Error(),
		}, nil
	}

	if !exists {
		return &pb.GetChunkResponse{
			Found: false,
		}, nil
	}

	// Get chunk from MinIO
	data, err := s.minioClient.GetChunk(ctx, req.Fingerprint)
	if err != nil {
		log.Printf("Failed to get chunk %s: %v", req.Fingerprint, err)
		return &pb.GetChunkResponse{
			Found:        false,
			ErrorMessage: err.Error(),
		}, nil
	}

	size, err := s.minioClient.GetChunkSize(ctx, req.Fingerprint)
	if err != nil {
		log.Printf("Failed to get chunk size for %s: %v", req.Fingerprint, err)
		return &pb.GetChunkResponse{
			Found:        false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.GetChunkResponse{
		ChunkData: data,
		Size:      size,
		Found:     true,
	}, nil
}

func main() {
	// Get configuration from environment variables
	minioEndpoint := getEnv("MINIO_ENDPOINT", "localhost:9000")
	minioAccessKey := getEnv("MINIO_ACCESS_KEY", "minioadmin")
	minioSecretKey := getEnv("MINIO_SECRET_KEY", "minioadmin")
	minioBucket := getEnv("MINIO_BUCKET", "dedupe-chunks")
	grpcPort := getEnv("GRPC_PORT", "50052")
	nodeID := getEnv("NODE_ID", "data-storage-node-1")

	// Initialize MinIO client
	minioClient, err := minio.NewClient(minioEndpoint, minioAccessKey, minioSecretKey, minioBucket, false)
	if err != nil {
		log.Fatalf("Failed to create MinIO client: %v", err)
	}

	// Create gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterStorageServiceServer(s, &server{
		minioClient: minioClient,
		nodeID:      nodeID,
	})

	log.Printf("Data Storage Node starting on port %s", grpcPort)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
