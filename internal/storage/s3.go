package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Config holds configuration for any S3-compatible object store
// (Cloudflare R2, MinIO, DigitalOcean Spaces, AWS S3, Backblaze B2, etc.).
type S3Config struct {
	Endpoint  string // e.g. "ACCOUNT_ID.r2.cloudflarestorage.com", "s3.amazonaws.com", "minio.local:9000"
	Bucket    string // S3 bucket name
	Region    string // e.g. "us-east-1"
	AccessKey string
	SecretKey string
	UseSSL    bool
}

// S3Backend stores files in any S3-compatible object store.
// AYB's bucket concept maps to a key prefix within a single S3 bucket.
// Object key format: {ayb_bucket}/{ayb_name}
type S3Backend struct {
	client *minio.Client
	bucket string
}

// NewS3Backend connects to an S3-compatible object store (R2, MinIO, Spaces, S3, etc.).
func NewS3Backend(ctx context.Context, cfg S3Config) (*S3Backend, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("creating S3 client: %w", err)
	}

	// Verify the bucket exists.
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("checking S3 bucket %q: %w", cfg.Bucket, err)
	}
	if !exists {
		return nil, fmt.Errorf("S3 bucket %q does not exist", cfg.Bucket)
	}

	return &S3Backend{client: client, bucket: cfg.Bucket}, nil
}

func (b *S3Backend) key(aybBucket, name string) string {
	return aybBucket + "/" + name
}

func (b *S3Backend) Put(ctx context.Context, bucket, name string, r io.Reader) (int64, error) {
	key := b.key(bucket, name)
	info, err := b.client.PutObject(ctx, b.bucket, key, r, -1, minio.PutObjectOptions{})
	if err != nil {
		return 0, fmt.Errorf("uploading to S3: %w", err)
	}
	return info.Size, nil
}

func (b *S3Backend) Get(ctx context.Context, bucket, name string) (io.ReadCloser, error) {
	key := b.key(bucket, name)
	obj, err := b.client.GetObject(ctx, b.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting from S3: %w", err)
	}

	// Stat to detect if the object exists (GetObject doesn't error on missing keys).
	_, err = obj.Stat()
	if err != nil {
		obj.Close()
		resp := minio.ToErrorResponse(err)
		if resp.Code == "NoSuchKey" {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("stat S3 object: %w", err)
	}
	return obj, nil
}

func (b *S3Backend) Delete(ctx context.Context, bucket, name string) error {
	key := b.key(bucket, name)
	err := b.client.RemoveObject(ctx, b.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("deleting from S3: %w", err)
	}
	return nil
}

func (b *S3Backend) Exists(ctx context.Context, bucket, name string) (bool, error) {
	key := b.key(bucket, name)
	_, err := b.client.StatObject(ctx, b.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		resp := minio.ToErrorResponse(err)
		if resp.Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("stat S3 object: %w", err)
	}
	return true, nil
}
