package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"time"

	_ "github.com/lib/pq"
)

//go:embed schema.sql
var schemaFS embed.FS

// DB wraps a CockroachDB connection
type DB struct {
	conn *sql.DB
}

// NewDB creates a new DB client and initializes the schema if needed
func NewDB(connStr string) (*DB, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	if err := initializeSchema(db); err != nil {
		return nil, err
	}
	return &DB{conn: db}, nil
}

// initializeSchema runs the schema.sql file to ensure tables exist
func initializeSchema(db *sql.DB) error {
	schema, err := fs.ReadFile(schemaFS, "schema.sql")
	if err != nil {
		return fmt.Errorf("failed to read schema.sql: %w", err)
	}
	_, err = db.Exec(string(schema))
	if err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}
	return nil
}

// --- Chunks CRUD ---
func (db *DB) GetChunkMetadataByFingerprint(ctx context.Context, fingerprint string) (*ChunkMetadata, error) {
	row := db.conn.QueryRowContext(ctx, `SELECT fingerprint, storage_location, size, creation_time, last_referenced_time FROM chunks WHERE fingerprint = $1`, fingerprint)
	var meta ChunkMetadata
	err := row.Scan(&meta.Fingerprint, &meta.StorageLocation, &meta.Size, &meta.CreationTime, &meta.LastReferencedTime)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

func (db *DB) InsertChunkMetadata(ctx context.Context, meta *ChunkMetadata) error {
	_, err := db.conn.ExecContext(ctx, `INSERT INTO chunks (fingerprint, storage_location, size, creation_time, last_referenced_time) VALUES ($1, $2, $3, $4, $5)`,
		meta.Fingerprint, meta.StorageLocation, meta.Size, meta.CreationTime, meta.LastReferencedTime)
	return err
}

func (db *DB) UpdateChunkMetadata(ctx context.Context, meta *ChunkMetadata) error {
	_, err := db.conn.ExecContext(ctx, `UPDATE chunks SET storage_location = $2, size = $3, creation_time = $4, last_referenced_time = $5 WHERE fingerprint = $1`,
		meta.Fingerprint, meta.StorageLocation, meta.Size, meta.CreationTime, meta.LastReferencedTime)
	return err
}

// --- Backup Jobs CRUD ---
func (db *DB) CreateBackupJob(ctx context.Context, job *BackupJob) error {
	_, err := db.conn.ExecContext(ctx, `INSERT INTO backup_jobs (job_id, client_id, backup_policy_id, start_time, end_time, status, source_type, source_details, files_metadata) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		job.JobID, job.ClientID, job.BackupPolicyID, job.StartTime, job.EndTime, job.Status, job.SourceType, job.SourceDetails, job.FilesMetadata)
	return err
}

func (db *DB) UpdateBackupJobStatus(ctx context.Context, jobID, status string, endTime *time.Time) error {
	_, err := db.conn.ExecContext(ctx, `UPDATE backup_jobs SET status = $2, end_time = $3 WHERE job_id = $1`, jobID, status, endTime)
	return err
}

func (db *DB) AddFileMetadataToJob(ctx context.Context, jobID string, filesMeta interface{}) error {
	_, err := db.conn.ExecContext(ctx, `UPDATE backup_jobs SET files_metadata = $2 WHERE job_id = $1`, jobID, filesMeta)
	return err
}

// --- Data Types ---
type ChunkMetadata struct {
	Fingerprint        string
	StorageLocation    string
	Size               int
	CreationTime       time.Time
	LastReferencedTime time.Time
}

type BackupJob struct {
	JobID          string
	ClientID       string
	BackupPolicyID string
	StartTime      time.Time
	EndTime        *time.Time
	Status         string
	SourceType     string
	SourceDetails  string
	FilesMetadata  interface{} // Use a struct or map for real implementation
}
