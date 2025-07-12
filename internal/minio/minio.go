package minio

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client wraps a MinIO client
type Client struct {
	client *minio.Client
	bucket string
}

// NewClient creates a new MinIO client
func NewClient(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*Client, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	// Ensure bucket exists
	exists, err := client.BucketExists(context.Background(), bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket existence: %w", err)
	}
	if !exists {
		err = client.MakeBucket(context.Background(), bucket, minio.MakeBucketOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
	}

	return &Client{
		client: client,
		bucket: bucket,
	}, nil
}

// StoreChunk stores a chunk in MinIO using the fingerprint as the object key
func (c *Client) StoreChunk(ctx context.Context, fingerprint string, data []byte) error {
	_, err := c.client.PutObject(ctx, c.bucket, fingerprint, io.NopCloser(bytes.NewReader(data)), int64(len(data)), minio.PutObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to store chunk %s: %w", fingerprint, err)
	}
	return nil
}

// GetChunk retrieves a chunk from MinIO by fingerprint
func (c *Client) GetChunk(ctx context.Context, fingerprint string) ([]byte, error) {
	obj, err := c.client.GetObject(ctx, c.bucket, fingerprint, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get chunk %s: %w", fingerprint, err)
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to read chunk data for %s: %w", fingerprint, err)
	}

	return data, nil
}

// ChunkExists checks if a chunk exists in MinIO
func (c *Client) ChunkExists(ctx context.Context, fingerprint string) (bool, error) {
	_, err := c.client.StatObject(ctx, c.bucket, fingerprint, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("failed to check chunk existence for %s: %w", fingerprint, err)
	}
	return true, nil
}

// GetChunkSize returns the size of a chunk in MinIO
func (c *Client) GetChunkSize(ctx context.Context, fingerprint string) (int64, error) {
	info, err := c.client.StatObject(ctx, c.bucket, fingerprint, minio.StatObjectOptions{})
	if err != nil {
		return 0, fmt.Errorf("failed to get chunk size for %s: %w", fingerprint, err)
	}
	return info.Size, nil
}
