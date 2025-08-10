package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Shopify/sarama"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/radhakrishnan.venkat/dedupe-engine/internal/db"
	"github.com/radhakrishnan.venkat/dedupe-engine/internal/minio"
	pb "github.com/radhakrishnan.venkat/dedupe-engine/pkg/api"
)

// GarbageCollectionService handles the mark-and-sweep garbage collection
type GarbageCollectionService struct {
	dbClient      *db.DB
	minioClient   *minio.Client
	storageClient pb.StorageServiceClient
	kafkaConsumer sarama.Consumer
	kafkaProducer sarama.SyncProducer
	config        *Config
}

// Config holds the service configuration
type Config struct {
	CockroachAddr  string
	MinIOEndpoint  string
	MinIOAccessKey string
	MinIOSecretKey string
	MinIOBucket    string
	KafkaBrokers   []string
	StorageAddr    string
	GCInterval     time.Duration
	BatchSize      int
}

// GCMessage represents a Kafka message for garbage collection triggers
type GCMessage struct {
	BackupJobID string    `json:"backup_job_id"`
	Action      string    `json:"action"` // "DELETE", "EXPIRE"
	Timestamp   time.Time `json:"timestamp"`
}

// NewGarbageCollectionService creates a new GC service
func NewGarbageCollectionService(config *Config) (*GarbageCollectionService, error) {
	// Initialize database client
	dbClient, err := db.NewDB(fmt.Sprintf("postgres://root@%s/dedupe_engine?sslmode=disable", config.CockroachAddr))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Initialize MinIO client
	minioClient, err := minio.NewClient(config.MinIOEndpoint, config.MinIOAccessKey, config.MinIOSecretKey, config.MinIOBucket, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	// Initialize storage client
	var storageClient pb.StorageServiceClient
	if config.StorageAddr != "" {
		conn, err := grpc.Dial(config.StorageAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Printf("Warning: Failed to connect to storage node: %v", err)
		} else {
			storageClient = pb.NewStorageServiceClient(conn)
		}
	}

	// Initialize Kafka consumer
	kafkaConfig := sarama.NewConfig()
	kafkaConfig.Consumer.Return.Errors = true
	consumer, err := sarama.NewConsumer(config.KafkaBrokers, kafkaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka consumer: %w", err)
	}

	// Initialize Kafka producer
	producerConfig := sarama.NewConfig()
	producerConfig.Producer.RequiredAcks = sarama.WaitForAll
	producerConfig.Producer.Retry.Max = 5
	producerConfig.Producer.Return.Successes = true
	producer, err := sarama.NewSyncProducer(config.KafkaBrokers, producerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	return &GarbageCollectionService{
		dbClient:      dbClient,
		minioClient:   minioClient,
		storageClient: storageClient,
		kafkaConsumer: consumer,
		kafkaProducer: producer,
		config:        config,
	}, nil
}

// Start starts the garbage collection service
func (gc *GarbageCollectionService) Start(ctx context.Context) error {
	log.Printf("Starting Garbage Collection Service...")

	// Start Kafka consumer for GC triggers
	go gc.startKafkaConsumer(ctx)

	// Start periodic GC
	go gc.startPeriodicGC(ctx)

	// Wait for context cancellation
	<-ctx.Done()
	return nil
}

// startKafkaConsumer listens for GC trigger messages
func (gc *GarbageCollectionService) startKafkaConsumer(ctx context.Context) {
	partitionConsumer, err := gc.kafkaConsumer.ConsumePartition("dedupe.gc_triggers", 0, sarama.OffsetNewest)
	if err != nil {
		log.Printf("Failed to start Kafka consumer: %v", err)
		return
	}
	defer partitionConsumer.Close()

	for {
		select {
		case msg := <-partitionConsumer.Messages():
			var gcMsg GCMessage
			if err := json.Unmarshal(msg.Value, &gcMsg); err != nil {
				log.Printf("Failed to unmarshal GC message: %v", err)
				continue
			}

			log.Printf("Received GC trigger: %s for backup job %s", gcMsg.Action, gcMsg.BackupJobID)

			// Trigger GC for this backup job
			if err := gc.performGC(ctx, gcMsg.BackupJobID); err != nil {
				log.Printf("Failed to perform GC for backup job %s: %v", gcMsg.BackupJobID, err)
			}

		case err := <-partitionConsumer.Errors():
			log.Printf("Kafka consumer error: %v", err)

		case <-ctx.Done():
			return
		}
	}
}

// startPeriodicGC runs periodic garbage collection
func (gc *GarbageCollectionService) startPeriodicGC(ctx context.Context) {
	ticker := time.NewTicker(gc.config.GCInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			log.Printf("Running periodic garbage collection...")
			if err := gc.performPeriodicGC(ctx); err != nil {
				log.Printf("Failed to perform periodic GC: %v", err)
			}

		case <-ctx.Done():
			return
		}
	}
}

// performPeriodicGC performs garbage collection on expired backup jobs
func (gc *GarbageCollectionService) performPeriodicGC(ctx context.Context) error {
	// Get expired backup jobs (jobs older than retention period)
	// This is a simplified implementation - in reality, you'd check retention policies
	expiredJobs, err := gc.getExpiredBackupJobs(ctx)
	if err != nil {
		return fmt.Errorf("failed to get expired backup jobs: %w", err)
	}

	for _, jobID := range expiredJobs {
		log.Printf("Processing expired backup job: %s", jobID)
		if err := gc.performGC(ctx, jobID); err != nil {
			log.Printf("Failed to perform GC for expired job %s: %v", jobID, err)
		}
	}

	return nil
}

// getExpiredBackupJobs gets backup jobs that have expired (simplified)
func (gc *GarbageCollectionService) getExpiredBackupJobs(ctx context.Context) ([]string, error) {
	// This is a placeholder - in reality, you'd query based on retention policies
	// For now, we'll return an empty list
	return []string{}, nil
}

// performGC performs garbage collection for a specific backup job
func (gc *GarbageCollectionService) performGC(ctx context.Context, backupJobID string) error {
	log.Printf("Starting GC for backup job: %s", backupJobID)

	// Step 1: Mark Phase - Get all live chunks
	liveChunks, err := gc.markPhase(ctx, backupJobID)
	if err != nil {
		return fmt.Errorf("mark phase failed: %w", err)
	}

	// Step 2: Sweep Phase - Remove unreferenced chunks
	if err := gc.sweepPhase(ctx, liveChunks); err != nil {
		return fmt.Errorf("sweep phase failed: %w", err)
	}

	log.Printf("Completed GC for backup job: %s", backupJobID)
	return nil
}

// markPhase identifies all chunks that are still referenced by active backup jobs
func (gc *GarbageCollectionService) markPhase(ctx context.Context, backupJobID string) (map[string]bool, error) {
	// Get all chunks for this backup job
	chunkFingerprints, err := gc.dbClient.GetChunksForBackupJob(ctx, backupJobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get chunks for backup job: %w", err)
	}

	// Create a set of live chunks
	liveChunks := make(map[string]bool)
	for _, fingerprint := range chunkFingerprints {
		liveChunks[fingerprint] = true
	}

	log.Printf("Mark phase: Found %d live chunks for backup job %s", len(liveChunks), backupJobID)
	return liveChunks, nil
}

// sweepPhase removes chunks that are not in the live set
func (gc *GarbageCollectionService) sweepPhase(ctx context.Context, liveChunks map[string]bool) error {
	// Get all chunks from the database
	allChunks, err := gc.getAllChunks(ctx)
	if err != nil {
		return fmt.Errorf("failed to get all chunks: %w", err)
	}

	// Find unreferenced chunks
	var unreferencedChunks []string
	for _, chunk := range allChunks {
		if !liveChunks[chunk.Fingerprint] {
			unreferencedChunks = append(unreferencedChunks, chunk.Fingerprint)
		}
	}

	log.Printf("Sweep phase: Found %d unreferenced chunks to delete", len(unreferencedChunks))

	// Delete unreferenced chunks in batches
	batchSize := gc.config.BatchSize
	for i := 0; i < len(unreferencedChunks); i += batchSize {
		end := i + batchSize
		if end > len(unreferencedChunks) {
			end = len(unreferencedChunks)
		}

		batch := unreferencedChunks[i:end]
		if err := gc.deleteChunkBatch(ctx, batch); err != nil {
			log.Printf("Failed to delete chunk batch: %v", err)
		}

		log.Printf("Deleted batch %d/%d chunks", end, len(unreferencedChunks))
	}

	return nil
}

// getAllChunks gets all chunks from the database
func (gc *GarbageCollectionService) getAllChunks(ctx context.Context) ([]db.ChunkMetadata, error) {
	// This is a placeholder - in reality, you'd implement a method to get all chunks
	// For now, return empty list
	return []db.ChunkMetadata{}, nil
}

// deleteChunkBatch deletes a batch of chunks
func (gc *GarbageCollectionService) deleteChunkBatch(ctx context.Context, fingerprints []string) error {
	for _, fingerprint := range fingerprints {
		// Delete from MinIO
		if err := gc.minioClient.DeleteChunk(ctx, fingerprint); err != nil {
			log.Printf("Failed to delete chunk %s from MinIO: %v", fingerprint, err)
		}

		// Delete from database
		if err := gc.deleteChunkFromDB(ctx, fingerprint); err != nil {
			log.Printf("Failed to delete chunk %s from database: %v", fingerprint, err)
		}
	}

	return nil
}

// deleteChunkFromDB deletes a chunk from the database
func (gc *GarbageCollectionService) deleteChunkFromDB(ctx context.Context, fingerprint string) error {
	// This is a placeholder - implement actual database deletion
	return nil
}

func main() {
	// Get configuration from environment variables
	config := &Config{
		CockroachAddr:  getEnv("COCKROACHDB_ADDR", "localhost:26257"),
		MinIOEndpoint:  getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinIOAccessKey: getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		MinIOSecretKey: getEnv("MINIO_SECRET_KEY", "minioadmin"),
		MinIOBucket:    getEnv("MINIO_BUCKET", "dedupe-chunks"),
		KafkaBrokers:   []string{getEnv("KAFKA_BROKERS", "localhost:9092")},
		StorageAddr:    getEnv("STORAGE_NODE_ADDR", "localhost:50052"),
		GCInterval:     1 * time.Hour, // Run GC every hour
		BatchSize:      100,           // Process 100 chunks at a time
	}

	// Create and start the garbage collection service
	gc, err := NewGarbageCollectionService(config)
	if err != nil {
		log.Fatalf("Failed to create garbage collection service: %v", err)
	}

	ctx := context.Background()
	if err := gc.Start(ctx); err != nil {
		log.Fatalf("Garbage collection service failed: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
