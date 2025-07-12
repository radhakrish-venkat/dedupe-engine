package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/radhakrishnan.venkat/dedupe-engine/internal/cache"
	"github.com/radhakrishnan.venkat/dedupe-engine/internal/chunking"
	"github.com/radhakrishnan.venkat/dedupe-engine/internal/db"
	pb "github.com/radhakrishnan.venkat/dedupe-engine/pkg/api"
	storagepb "github.com/radhakrishnan.venkat/dedupe-engine/pkg/api"
)

// IngestServer implements the BackupService
type IngestServer struct {
	pb.UnimplementedBackupServiceServer

	// Components
	chunker       *chunking.Chunker
	cache         *cache.DeduplicationCache
	dbClient      *db.DB
	storageClient *storagepb.StorageServiceClient

	// Backup state
	backupJobs  map[string]*BackupJobState
	backupMutex sync.RWMutex

	// Configuration
	grpcPort    string
	storageAddr string
}

// BackupJobState tracks the state of an active backup job
type BackupJobState struct {
	JobID             string
	ClientID          string
	StartTime         time.Time
	Status            string
	FilesProcessed    int
	ChunksProcessed   int
	BytesProcessed    int64
	BytesDeduplicated int64
	FileBuffer        map[string][]byte           // file path -> accumulated data
	FileChunks        map[string][]chunking.Chunk // file path -> chunks
}

// NewIngestServer creates a new IngestServer instance
func NewIngestServer(grpcPort, storageAddr string) *IngestServer {
	return &IngestServer{
		chunker:     chunking.NewChunker(64, 8192),            // 64B min, 8KB max
		cache:       cache.NewDeduplicationCache(1000, 10000), // 1000 cache entries, 10000 filter capacity
		backupJobs:  make(map[string]*BackupJobState),
		grpcPort:    grpcPort,
		storageAddr: storageAddr,
	}
}

// StreamBackup handles bidirectional streaming backup requests
func (s *IngestServer) StreamBackup(stream pb.BackupService_StreamBackupServer) error {
	var currentJob *BackupJobState
	var currentFile string

	for {
		request, err := stream.Recv()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return status.Errorf(codes.Internal, "Failed to receive request: %v", err)
		}

		log.Printf("Received request type: %T", request.RequestType)

		switch req := request.RequestType.(type) {
		case *pb.BackupRequest_StartBackup:
			// Handle backup start
			startReq := req.StartBackup
			log.Printf("Starting backup job: %s", startReq.BackupJobId)
			currentJob = &BackupJobState{
				JobID:      startReq.BackupJobId,
				ClientID:   startReq.ClientId,
				StartTime:  time.Unix(startReq.Timestamp, 0),
				Status:     "INITIATED",
				FileBuffer: make(map[string][]byte),
				FileChunks: make(map[string][]chunking.Chunk),
			}

			s.backupMutex.Lock()
			s.backupJobs[startReq.BackupJobId] = currentJob
			s.backupMutex.Unlock()

			// Send status update
			statusResp := &pb.BackupResponse{
				ResponseType: &pb.BackupResponse_StatusUpdate{
					StatusUpdate: &pb.BackupStatus{
						BackupJobId:       startReq.BackupJobId,
						Message:           "Backup initiated successfully",
						BytesProcessed:    uint64(currentJob.BytesProcessed),
						BytesDeduplicated: uint64(currentJob.BytesDeduplicated),
					},
				},
			}
			if err := stream.Send(statusResp); err != nil {
				return status.Errorf(codes.Internal, "Failed to send status: %v", err)
			}

			log.Printf("Started backup job: %s for client: %s", startReq.BackupJobId, startReq.ClientId)

		case *pb.BackupRequest_FileSegment:
			// Handle file segment
			segment := req.FileSegment
			currentFile = segment.FilePath
			log.Printf("Received file segment: %s, size: %d, offset: %d, isLast: %v",
				segment.FilePath, len(segment.Data), segment.Offset, segment.IsLastSegment)

			if currentJob == nil {
				return status.Error(codes.FailedPrecondition, "No active backup job")
			}

			// Accumulate file data
			if currentJob.FileBuffer[currentFile] == nil {
				currentJob.FileBuffer[currentFile] = make([]byte, 0)
			}
			currentJob.FileBuffer[currentFile] = append(currentJob.FileBuffer[currentFile], segment.Data...)

			// Process complete files
			if segment.IsLastSegment {
				log.Printf("Processing complete file: %s", currentFile)
				if err := s.processFile(currentJob, currentFile, stream); err != nil {
					return status.Errorf(codes.Internal, "Failed to process file: %v", err)
				}
			}

		case *pb.BackupRequest_EndBackup:
			// Handle backup end
			endReq := req.EndBackup
			log.Printf("Ending backup job: %s", endReq.BackupJobId)
			if currentJob == nil {
				return status.Error(codes.FailedPrecondition, "No active backup job")
			}

			currentJob.Status = endReq.Status

			// Send final status
			finalStatus := &pb.BackupResponse{
				ResponseType: &pb.BackupResponse_StatusUpdate{
					StatusUpdate: &pb.BackupStatus{
						BackupJobId: endReq.BackupJobId,
						Message: fmt.Sprintf("Backup completed. Processed %d files, %d chunks",
							currentJob.FilesProcessed, currentJob.ChunksProcessed),
						BytesProcessed:    uint64(currentJob.BytesProcessed),
						BytesDeduplicated: uint64(currentJob.BytesDeduplicated),
					},
				},
			}
			if err := stream.Send(finalStatus); err != nil {
				return status.Errorf(codes.Internal, "Failed to send final status: %v", err)
			}

			log.Printf("Completed backup job: %s", endReq.BackupJobId)
		}
	}

	return nil
}

// processFile processes a complete file by chunking and deduplicating
func (s *IngestServer) processFile(job *BackupJobState, filePath string, stream pb.BackupService_StreamBackupServer) error {
	fileData := job.FileBuffer[filePath]

	// Chunk the file
	chunks, err := s.chunker.ChunkData(fileData)
	if err != nil {
		return fmt.Errorf("failed to chunk file %s: %w", filePath, err)
	}

	job.FileChunks[filePath] = chunks
	job.FilesProcessed++

	log.Printf("Processing file: %s (%d bytes, %d chunks)", filePath, len(fileData), len(chunks))

	// Process each chunk
	for i, chunk := range chunks {
		job.ChunksProcessed++
		job.BytesProcessed += chunk.Size

		// Check if chunk already exists (deduplication)
		if _, exists := s.cache.GetChunkMetadata(chunk.Fingerprint); exists {
			job.BytesDeduplicated += chunk.Size
			log.Printf("  Chunk %d: DEDUPLICATED (fingerprint: %s)", i, chunk.Fingerprint[:16])
			continue
		}

		// Check database for existing chunk
		if s.dbClient != nil {
			if dbMetadata, err := s.dbClient.GetChunkMetadataByFingerprint(context.Background(), chunk.Fingerprint); err == nil && dbMetadata != nil {
				job.BytesDeduplicated += chunk.Size
				s.cache.PutChunkMetadata(chunk.Fingerprint, &cache.ChunkMetadata{
					Fingerprint:        dbMetadata.Fingerprint,
					StorageLocation:    dbMetadata.StorageLocation,
					Size:               int64(dbMetadata.Size),
					CreationTime:       dbMetadata.CreationTime,
					LastReferencedTime: dbMetadata.LastReferencedTime,
				})
				log.Printf("  Chunk %d: DEDUPLICATED (from DB, fingerprint: %s)", i, chunk.Fingerprint[:16])
				continue
			}
		}

		// Store unique chunk
		if err := s.storeUniqueChunk(chunk); err != nil {
			return fmt.Errorf("failed to store chunk %d: %w", i, err)
		}

		log.Printf("  Chunk %d: STORED (fingerprint: %s)", i, chunk.Fingerprint[:16])
	}

	// Send progress update
	statusResp := &pb.BackupResponse{
		ResponseType: &pb.BackupResponse_StatusUpdate{
			StatusUpdate: &pb.BackupStatus{
				BackupJobId:       job.JobID,
				CurrentFile:       filePath,
				BytesProcessed:    uint64(job.BytesProcessed),
				BytesDeduplicated: uint64(job.BytesDeduplicated),
				Message:           fmt.Sprintf("Processed file: %s", filePath),
			},
		},
	}
	if err := stream.Send(statusResp); err != nil {
		return fmt.Errorf("failed to send status: %v", err)
	}

	return nil
}

// storeUniqueChunk stores a unique chunk via the Data Storage Node
func (s *IngestServer) storeUniqueChunk(chunk chunking.Chunk) error {
	// For now, we'll simulate storage since we don't have a running Data Storage Node
	// In a real implementation, you'd make a gRPC call to the Data Storage Node

	// Create metadata for the chunk
	metadata := &cache.ChunkMetadata{
		Fingerprint:        chunk.Fingerprint,
		StorageLocation:    fmt.Sprintf("minio://dedupe-chunks/%s", chunk.Fingerprint),
		Size:               chunk.Size,
		CreationTime:       time.Now(),
		LastReferencedTime: time.Now(),
	}

	// Add to cache
	s.cache.PutChunkMetadata(chunk.Fingerprint, metadata)

	// Store in database if available
	if s.dbClient != nil {
		dbMetadata := &db.ChunkMetadata{
			Fingerprint:        metadata.Fingerprint,
			StorageLocation:    metadata.StorageLocation,
			Size:               int(metadata.Size),
			CreationTime:       metadata.CreationTime,
			LastReferencedTime: metadata.LastReferencedTime,
		}
		if err := s.dbClient.InsertChunkMetadata(context.Background(), dbMetadata); err != nil {
			log.Printf("Warning: Failed to store chunk metadata in DB: %v", err)
		}
	}

	return nil
}

// InitiateRestore handles restore initiation requests
func (s *IngestServer) InitiateRestore(ctx context.Context, req *pb.RestoreRequest) (*pb.RestoreResponse, error) {
	// TODO: Implement restore logic
	return &pb.RestoreResponse{
		RestoreJobId: fmt.Sprintf("restore-%d", time.Now().Unix()),
		Status:       "INITIATED",
		Message:      "Restore initiated (not yet implemented)",
	}, nil
}

// StreamRestoreData handles restore data streaming
func (s *IngestServer) StreamRestoreData(stream pb.BackupService_StreamRestoreDataServer) error {
	// TODO: Implement restore data streaming
	return status.Error(codes.Unimplemented, "Restore data streaming not yet implemented")
}

func main() {
	// Get configuration from environment variables
	grpcPort := getEnv("GRPC_PORT", "50051")
	storageAddr := getEnv("STORAGE_NODE_ADDR", "localhost:50052")
	cockroachAddr := getEnv("COCKROACHDB_ADDR", "")

	log.Printf("Starting Ingest Node on port %s", grpcPort)

	// Create server
	server := NewIngestServer(grpcPort, storageAddr)

	// Initialize database client if address provided
	if cockroachAddr != "" {
		dbClient, err := db.NewDB(fmt.Sprintf("postgres://root@%s/dedupe_engine?sslmode=disable", cockroachAddr))
		if err != nil {
			log.Printf("Warning: Failed to connect to CockroachDB: %v", err)
		} else {
			server.dbClient = dbClient
			log.Printf("Connected to CockroachDB at %s", cockroachAddr)
		}
	}

	// Create gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterBackupServiceServer(grpcServer, server)

	log.Printf("Ingest Node ready to accept connections")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
