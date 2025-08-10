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
	var storageClient storagepb.StorageServiceClient
	if storageAddr != "" {
		// Create gRPC connection to storage node
		conn, err := grpc.Dial(storageAddr, grpc.WithInsecure())
		if err != nil {
			log.Printf("Warning: Failed to connect to storage node at %s: %v", storageAddr, err)
		} else {
			storageClient = storagepb.NewStorageServiceClient(conn)
			log.Printf("Connected to storage node at %s", storageAddr)
		}
	}

	return &IngestServer{
		chunker:       chunking.NewChunker(64, 8192),            // 64B min, 8KB max
		cache:         cache.NewDeduplicationCache(1000, 10000), // 1000 cache entries, 10000 filter capacity
		backupJobs:    make(map[string]*BackupJobState),
		grpcPort:      grpcPort,
		storageAddr:   storageAddr,
		storageClient: storageClient,
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
	if req.BackupJobId == "" {
		return nil, status.Error(codes.InvalidArgument, "backup_job_id is required")
	}

	// Check if backup job exists
	if s.dbClient == nil {
		return nil, status.Error(codes.Unavailable, "database connection not available")
	}

	backupJob, err := s.dbClient.GetBackupJob(ctx, req.BackupJobId)
	if err != nil {
		log.Printf("Failed to get backup job %s: %v", req.BackupJobId, err)
		return nil, status.Error(codes.Internal, "failed to retrieve backup job")
	}

	if backupJob == nil {
		return nil, status.Error(codes.NotFound, "backup job not found")
	}

	if backupJob.Status != "COMPLETED" {
		return nil, status.Error(codes.FailedPrecondition, "backup job is not completed")
	}

	// Get chunks for this backup job
	chunkFingerprints, err := s.dbClient.GetChunksForBackupJob(ctx, req.BackupJobId)
	if err != nil {
		log.Printf("Failed to get chunks for backup job %s: %v", req.BackupJobId, err)
		return nil, status.Error(codes.Internal, "failed to retrieve backup chunks")
	}

	// Get file metadata for this backup job
	fileMetadata, err := s.dbClient.GetFileMetadataForBackupJob(ctx, req.BackupJobId)
	if err != nil {
		log.Printf("Failed to get file metadata for backup job %s: %v", req.BackupJobId, err)
		return nil, status.Error(codes.Internal, "failed to retrieve file metadata")
	}

	// Create restore job ID
	restoreJobID := fmt.Sprintf("restore-%s-%d", req.BackupJobId, time.Now().Unix())

	log.Printf("Initiated restore job %s for backup %s with %d chunks and %d files",
		restoreJobID, req.BackupJobId, len(chunkFingerprints), len(fileMetadata))

	return &pb.RestoreResponse{
		RestoreJobId: restoreJobID,
		Status:       "INITIATED",
		Message:      fmt.Sprintf("Restore initiated with %d chunks and %d files", len(chunkFingerprints), len(fileMetadata)),
	}, nil
}

// StreamRestoreData handles restore data streaming
func (s *IngestServer) StreamRestoreData(stream pb.BackupService_StreamRestoreDataServer) error {
	// Receive initial request to get restore job ID
	request, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.Internal, "Failed to receive initial request: %v", err)
	}

	restoreJobID := request.RestoreJobId
	if restoreJobID == "" {
		return status.Error(codes.InvalidArgument, "restore_job_id is required")
	}

	log.Printf("Starting restore data stream for job: %s", restoreJobID)

	// For now, we'll implement a simple restore that fetches chunks and reassembles files
	// In a real implementation, you'd query the database for file metadata and chunk fingerprints

	// Simulate fetching and streaming chunks
	// This is a placeholder implementation - in reality, you'd:
	// 1. Query database for file metadata and chunk fingerprints
	// 2. Fetch chunks from storage nodes
	// 3. Reassemble files
	// 4. Stream reassembled data

	// For demonstration, we'll stream some dummy data
	dummyData := []byte("This is restored data from backup")

	// Split into segments and stream
	segmentSize := 10
	for i := 0; i < len(dummyData); i += segmentSize {
		end := i + segmentSize
		if end > len(dummyData) {
			end = len(dummyData)
		}

		segment := dummyData[i:end]
		isLast := end == len(dummyData)

		response := &pb.RestoreDataResponse{
			RestoreJobId:  restoreJobID,
			FilePath:      "/restored/file.txt",
			Data:          segment,
			Offset:        uint64(i),
			IsLastSegment: isLast,
		}

		if err := stream.Send(response); err != nil {
			return status.Errorf(codes.Internal, "Failed to send restore data: %v", err)
		}

		// Small delay to simulate processing
		time.Sleep(10 * time.Millisecond)
	}

	log.Printf("Completed restore data stream for job: %s", restoreJobID)
	return nil
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
