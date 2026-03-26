package backup

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// StoreObject holds metadata about an S3 object.
type StoreObject struct {
	Key  string
	Size int64
}

// Store is the object-store interface used by the backup engine and retention.
type Store interface {
	PutObject(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	GetObject(ctx context.Context, key string) (io.ReadCloser, int64, error)
	HeadObject(ctx context.Context, key string) (int64, error)
	ListObjects(ctx context.Context, prefix string) ([]StoreObject, error)
	DeleteObject(ctx context.Context, key string) error
}

// S3StoreConfig holds connection settings for an AWS S3 (or compatible) store.
type S3StoreConfig struct {
	Bucket     string
	Region     string
	Endpoint   string // custom endpoint for LocalStack / MinIO (optional)
	AccessKey  string
	SecretKey  string
	Encryption string // "" | "AES256" | "aws:kms"
	KMSKeyID   string
	UseSSL     bool
}

// S3Config is an alias for S3StoreConfig used by CLI wiring.
type S3Config = S3StoreConfig

// S3Store implements Store using aws-sdk-go-v2.
type S3Store struct {
	client     *s3.Client
	bucket     string
	encryption string
	kmsKeyID   string
}

// NewS3Store creates an S3Store from the given config.
func NewS3Store(ctx context.Context, cfg S3StoreConfig) (*S3Store, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		),
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	clientOpts := []func(*s3.Options){}
	if cfg.Endpoint != "" {
		// Force path-style for LocalStack / MinIO compatibility.
		ep := cfg.Endpoint
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(ep)
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(awsCfg, clientOpts...)
	return &S3Store{
		client:     client,
		bucket:     cfg.Bucket,
		encryption: cfg.Encryption,
		kmsKeyID:   cfg.KMSKeyID,
	}, nil
}

// PutObject uploads body (size bytes) to the given key.
func (s *S3Store) PutObject(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	}
	if s.encryption == "AES256" {
		input.ServerSideEncryption = types.ServerSideEncryptionAes256
	} else if s.encryption == "aws:kms" {
		input.ServerSideEncryption = types.ServerSideEncryptionAwsKms
		if s.kmsKeyID != "" {
			input.SSEKMSKeyId = aws.String(s.kmsKeyID)
		}
	}
	if _, err := s.client.PutObject(ctx, input); err != nil {
		return fmt.Errorf("s3 PutObject %q: %w", key, err)
	}
	return nil
}

// GetObject downloads an S3 object and returns a ReadCloser plus its size.
func (s *S3Store) GetObject(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("s3 GetObject %q: %w", key, err)
	}
	size := int64(0)
	if out.ContentLength != nil {
		size = *out.ContentLength
	}
	return out.Body, size, nil
}

// HeadObject checks existence and returns the object size.
func (s *S3Store) HeadObject(ctx context.Context, key string) (int64, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return 0, fmt.Errorf("s3 HeadObject %q: %w", key, err)
	}
	if out.ContentLength == nil {
		return 0, nil
	}
	return *out.ContentLength, nil
}

// ListObjects returns all objects with the given prefix.
func (s *S3Store) ListObjects(ctx context.Context, prefix string) ([]StoreObject, error) {
	var objects []StoreObject
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("s3 ListObjectsV2 %q: %w", prefix, err)
		}
		for _, obj := range page.Contents {
			key := ""
			if obj.Key != nil {
				key = *obj.Key
			}
			size := int64(0)
			if obj.Size != nil {
				size = *obj.Size
			}
			objects = append(objects, StoreObject{Key: key, Size: size})
		}
	}
	return objects, nil
}

// DeleteObject removes an object.
func (s *S3Store) DeleteObject(ctx context.Context, key string) error {
	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}); err != nil {
		return fmt.Errorf("s3 DeleteObject %q: %w", key, err)
	}
	return nil
}
