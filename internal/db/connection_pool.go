package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

// ConnectionPool manages a pool of database connections
type ConnectionPool struct {
	pool               *sql.DB
	maxConnections     int
	maxIdleConnections int
	connLifetime       time.Duration
	stats              *PoolStats
	statsMutex         sync.RWMutex
}

// PoolStats tracks connection pool statistics
type PoolStats struct {
	TotalConnections  int
	IdleConnections   int
	InUseConnections  int
	WaitCount         int64
	WaitDuration      time.Duration
	MaxIdleClosed     int64
	MaxLifetimeClosed int64
}

// NewConnectionPool creates a new connection pool
func NewConnectionPool(connStr string, maxConnections, maxIdleConnections int) (*ConnectionPool, error) {
	pool, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	pool.SetMaxOpenConns(maxConnections)
	pool.SetMaxIdleConns(maxIdleConnections)
	pool.SetConnMaxLifetime(1 * time.Hour)
	pool.SetConnMaxIdleTime(30 * time.Minute)

	// Test the connection
	if err := pool.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &ConnectionPool{
		pool:               pool,
		maxConnections:     maxConnections,
		maxIdleConnections: maxIdleConnections,
		connLifetime:       1 * time.Hour,
		stats:              &PoolStats{},
	}, nil
}

// Close closes the connection pool
func (cp *ConnectionPool) Close() error {
	return cp.pool.Close()
}

// GetDB returns the underlying database connection
func (cp *ConnectionPool) GetDB() *sql.DB {
	return cp.pool
}

// GetStats returns connection pool statistics
func (cp *ConnectionPool) GetStats() *PoolStats {
	cp.statsMutex.RLock()
	defer cp.statsMutex.RUnlock()

	stats := *cp.stats // Copy to avoid race conditions
	stats.TotalConnections = cp.pool.Stats().MaxOpenConnections
	stats.IdleConnections = cp.pool.Stats().Idle
	stats.InUseConnections = cp.pool.Stats().InUse
	return &stats
}

// BatchProcessor handles batch database operations
type BatchProcessor struct {
	pool        *ConnectionPool
	batchSize   int
	buffer      []*ChunkMetadata
	bufferMutex sync.Mutex
	flushTicker *time.Ticker
	done        chan bool
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(pool *ConnectionPool, batchSize int) *BatchProcessor {
	bp := &BatchProcessor{
		pool:        pool,
		batchSize:   batchSize,
		buffer:      make([]*ChunkMetadata, 0, batchSize),
		flushTicker: time.NewTicker(1 * time.Second), // Flush every second
		done:        make(chan bool),
	}

	// Start background flush goroutine
	go bp.backgroundFlush()

	return bp
}

// Close closes the batch processor
func (bp *BatchProcessor) Close() {
	bp.flushTicker.Stop()
	bp.done <- true
	bp.Flush() // Final flush
}

// AddChunk adds a chunk to the batch buffer
func (bp *BatchProcessor) AddChunk(metadata *ChunkMetadata) error {
	bp.bufferMutex.Lock()
	defer bp.bufferMutex.Unlock()

	bp.buffer = append(bp.buffer, metadata)

	// Flush if buffer is full
	if len(bp.buffer) >= bp.batchSize {
		return bp.flushBuffer()
	}

	return nil
}

// Flush flushes the current buffer
func (bp *BatchProcessor) Flush() error {
	bp.bufferMutex.Lock()
	defer bp.bufferMutex.Unlock()

	return bp.flushBuffer()
}

// flushBuffer flushes the buffer to the database
func (bp *BatchProcessor) flushBuffer() error {
	if len(bp.buffer) == 0 {
		return nil
	}

	// Use COPY command for bulk insert
	tx, err := bp.pool.GetDB().Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare COPY statement
	stmt, err := tx.Prepare(`COPY chunks (fingerprint, storage_location, size, creation_time, last_referenced_time) FROM STDIN`)
	if err != nil {
		return fmt.Errorf("failed to prepare COPY statement: %w", err)
	}
	defer stmt.Close()

	// Write data to COPY stream
	for _, metadata := range bp.buffer {
		_, err := stmt.Exec(
			metadata.Fingerprint,
			metadata.StorageLocation,
			metadata.Size,
			metadata.CreationTime,
			metadata.LastReferencedTime,
		)
		if err != nil {
			return fmt.Errorf("failed to write to COPY stream: %w", err)
		}
	}

	// Close the COPY stream
	if err := stmt.Close(); err != nil {
		return fmt.Errorf("failed to close COPY stream: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Batch inserted %d chunks", len(bp.buffer))

	// Clear buffer
	bp.buffer = bp.buffer[:0]

	return nil
}

// backgroundFlush runs background flush operations
func (bp *BatchProcessor) backgroundFlush() {
	for {
		select {
		case <-bp.flushTicker.C:
			if err := bp.Flush(); err != nil {
				log.Printf("Warning: Background flush failed: %v", err)
			}
		case <-bp.done:
			return
		}
	}
}

// BatchQueryProcessor handles batch query operations
type BatchQueryProcessor struct {
	pool *ConnectionPool
}

// NewBatchQueryProcessor creates a new batch query processor
func NewBatchQueryProcessor(pool *ConnectionPool) *BatchQueryProcessor {
	return &BatchQueryProcessor{
		pool: pool,
	}
}

// GetChunksBatch retrieves multiple chunks by fingerprints
func (bqp *BatchQueryProcessor) GetChunksBatch(ctx context.Context, fingerprints []string) (map[string]*ChunkMetadata, error) {
	if len(fingerprints) == 0 {
		return make(map[string]*ChunkMetadata), nil
	}

	// Build query with placeholders
	query := `SELECT fingerprint, storage_location, size, creation_time, last_referenced_time 
			  FROM chunks WHERE fingerprint IN (`

	args := make([]interface{}, len(fingerprints))
	for i, fp := range fingerprints {
		if i > 0 {
			query += ","
		}
		query += fmt.Sprintf("$%d", i+1)
		args[i] = fp
	}
	query += ")"

	// Execute query
	rows, err := bqp.pool.GetDB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query chunks: %w", err)
	}
	defer rows.Close()

	// Build result map
	result := make(map[string]*ChunkMetadata)
	for rows.Next() {
		var metadata ChunkMetadata
		err := rows.Scan(
			&metadata.Fingerprint,
			&metadata.StorageLocation,
			&metadata.Size,
			&metadata.CreationTime,
			&metadata.LastReferencedTime,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan chunk metadata: %w", err)
		}
		result[metadata.Fingerprint] = &metadata
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return result, nil
}

// UpdateChunksBatch updates multiple chunks
func (bqp *BatchQueryProcessor) UpdateChunksBatch(ctx context.Context, metadataList []*ChunkMetadata) error {
	if len(metadataList) == 0 {
		return nil
	}

	// Use a transaction for batch update
	tx, err := bqp.pool.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare update statement
	stmt, err := tx.PrepareContext(ctx, `
		UPDATE chunks 
		SET storage_location = $2, size = $3, creation_time = $4, last_referenced_time = $5 
		WHERE fingerprint = $1
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare update statement: %w", err)
	}
	defer stmt.Close()

	// Execute updates
	for _, metadata := range metadataList {
		_, err := stmt.ExecContext(ctx,
			metadata.Fingerprint,
			metadata.StorageLocation,
			metadata.Size,
			metadata.CreationTime,
			metadata.LastReferencedTime,
		)
		if err != nil {
			return fmt.Errorf("failed to update chunk %s: %w", metadata.Fingerprint, err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Batch updated %d chunks", len(metadataList))
	return nil
}

// DeleteChunksBatch deletes multiple chunks
func (bqp *BatchQueryProcessor) DeleteChunksBatch(ctx context.Context, fingerprints []string) error {
	if len(fingerprints) == 0 {
		return nil
	}

	// Build delete query
	query := `DELETE FROM chunks WHERE fingerprint IN (`
	args := make([]interface{}, len(fingerprints))
	for i, fp := range fingerprints {
		if i > 0 {
			query += ","
		}
		query += fmt.Sprintf("$%d", i+1)
		args[i] = fp
	}
	query += ")"

	// Execute delete
	result, err := bqp.pool.GetDB().ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to delete chunks: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	log.Printf("Batch deleted %d chunks", rowsAffected)
	return nil
}
