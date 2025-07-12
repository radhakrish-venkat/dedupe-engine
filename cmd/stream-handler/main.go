package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/radhakrishnan.venkat/dedupe-engine/pkg/api"
)

func main() {
	// Parse command line flags
	ingestAddr := flag.String("ingest-addr", "localhost:50051", "Address of the Ingest Node")
	filePath := flag.String("file", "", "Path to the file to backup")
	clientID := flag.String("client-id", "test-client", "Client ID for the backup")
	flag.Parse()

	if *filePath == "" {
		log.Fatal("Please specify a file path with -file")
	}

	// Open the file
	file, err := os.Open(*filePath)
	if err != nil {
		log.Fatalf("Failed to open file %s: %v", *filePath, err)
	}
	defer file.Close()

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		log.Fatalf("Failed to get file info: %v", err)
	}

	log.Printf("Starting backup of file: %s (size: %d bytes)", *filePath, fileInfo.Size())

	// Connect to Ingest Node
	conn, err := grpc.Dial(*ingestAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to Ingest Node: %v", err)
	}
	defer conn.Close()

	client := pb.NewBackupServiceClient(conn)

	// Create backup stream
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	stream, err := client.StreamBackup(ctx)
	if err != nil {
		log.Fatalf("Failed to create backup stream: %v", err)
	}

	// Send backup start message
	backupJobID := fmt.Sprintf("backup-%d", time.Now().Unix())
	startMsg := &pb.BackupRequest{
		RequestType: &pb.BackupRequest_StartBackup{
			StartBackup: &pb.BackupStart{
				ClientId:        *clientID,
				BackupJobId:     backupJobID,
				BackupPolicyId:  "default-policy",
				EncryptionKeyId: "",
				Timestamp:       time.Now().Unix(),
				SourceType:      "filesystem",
				SourceDetails:   fmt.Sprintf(`{"path": "%s"}`, *filePath),
			},
		},
	}

	if err := stream.Send(startMsg); err != nil {
		log.Fatalf("Failed to send backup start message: %v", err)
	}

	log.Printf("Started backup job: %s", backupJobID)

	// Read file in chunks and send
	buffer := make([]byte, 64*1024) // 64KB chunks
	offset := int64(0)
	segmentCount := 0

	for {
		n, err := file.Read(buffer)
		log.Printf("Read %d bytes, err: %v", n, err)
		if n == 0 {
			break
		}

		chunkData := buffer[:n]
		// Check if this will be the last segment
		isLastSegment := err == io.EOF || (offset+int64(n)) >= fileInfo.Size()
		log.Printf("Sending segment %d: size=%d, offset=%d, isLast=%v", segmentCount+1, len(chunkData), offset, isLastSegment)

		segmentMsg := &pb.BackupRequest{
			RequestType: &pb.BackupRequest_FileSegment{
				FileSegment: &pb.FileSegment{
					FilePath:      *filePath,
					FileSize:      uint64(fileInfo.Size()),
					Data:          chunkData,
					Offset:        uint64(offset),
					IsLastSegment: isLastSegment,
				},
			},
		}

		if err := stream.Send(segmentMsg); err != nil {
			log.Fatalf("Failed to send file segment: %v", err)
		}

		offset += int64(n)
		segmentCount++

		if segmentCount%100 == 0 {
			log.Printf("Sent %d segments, offset: %d", segmentCount, offset)
		}

		// If we've read the entire file, break
		if offset >= fileInfo.Size() {
			break
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("Error reading file: %v", err)
		}
	}

	// Send backup end message
	endMsg := &pb.BackupRequest{
		RequestType: &pb.BackupRequest_EndBackup{
			EndBackup: &pb.BackupEnd{
				BackupJobId: backupJobID,
				Status:      "COMPLETED",
				Summary:     fmt.Sprintf("Backed up %s in %d segments", *filePath, segmentCount),
			},
		},
	}

	if err := stream.Send(endMsg); err != nil {
		log.Fatalf("Failed to send backup end message: %v", err)
	}

	// Close the send stream
	if err := stream.CloseSend(); err != nil {
		log.Fatalf("Failed to close send stream: %v", err)
	}

	log.Printf("Finished sending file data, waiting for responses...")

	// Receive responses from Ingest Node
	for {
		response, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("Failed to receive response: %v", err)
		}

		switch response.ResponseType.(type) {
		case *pb.BackupResponse_StatusUpdate:
			status := response.GetStatusUpdate()
			log.Printf("Status: %s - %s (processed: %d, deduplicated: %d bytes)",
				status.BackupJobId, status.Message, status.BytesProcessed, status.BytesDeduplicated)

		case *pb.BackupResponse_ErrorMessage:
			error := response.GetErrorMessage()
			log.Printf("Error: %s - %s", error.ErrorCode, error.ErrorMessage)
		}
	}

	log.Printf("Backup completed successfully!")
}
