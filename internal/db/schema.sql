-- Chunks table: stores unique data chunks
CREATE TABLE IF NOT EXISTS chunks (
    fingerprint STRING PRIMARY KEY, -- Blake3 hash of chunk
    storage_location STRING NOT NULL, -- MinIO object key
    size INT NOT NULL,
    creation_time TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_referenced_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Index for quick lookup by last referenced time (for GC/eviction)
CREATE INDEX IF NOT EXISTS idx_chunks_last_referenced_time ON chunks (last_referenced_time);

-- Backup jobs table: stores metadata for each backup job
CREATE TABLE IF NOT EXISTS backup_jobs (
    job_id STRING PRIMARY KEY,
    client_id STRING NOT NULL,
    backup_policy_id STRING,
    start_time TIMESTAMPTZ NOT NULL DEFAULT now(),
    end_time TIMESTAMPTZ,
    status STRING NOT NULL, -- e.g., INITIATED, COMPLETED, FAILED
    source_type STRING,
    source_details STRING,
    files_metadata JSONB -- List of files, chunk fingerprints, etc.
);

-- Indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_backup_jobs_client_id ON backup_jobs (client_id);
CREATE INDEX IF NOT EXISTS idx_backup_jobs_status ON backup_jobs (status); 