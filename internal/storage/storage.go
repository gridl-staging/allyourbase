package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sentinel errors.
var (
	ErrNotFound      = errors.New("object not found")
	ErrAlreadyExists = errors.New("object already exists")
	ErrInvalidBucket = errors.New("invalid bucket name")
	ErrInvalidName   = errors.New("invalid object name")
)

// Backend is the interface for file storage backends.
type Backend interface {
	Put(ctx context.Context, bucket, name string, r io.Reader) (int64, error)
	Get(ctx context.Context, bucket, name string) (io.ReadCloser, error)
	Delete(ctx context.Context, bucket, name string) error
	Exists(ctx context.Context, bucket, name string) (bool, error)
}

// Object represents a stored file's metadata.
type Object struct {
	ID          string    `json:"id"`
	Bucket      string    `json:"bucket"`
	Name        string    `json:"name"`
	Size        int64     `json:"size"`
	ContentType string    `json:"contentType"`
	UserID      *string   `json:"userId,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Service handles file storage operations.
type Service struct {
	pool    *pgxpool.Pool
	backend Backend
	signKey []byte
	logger  *slog.Logger
}

// NewService creates a new storage service.
func NewService(pool *pgxpool.Pool, backend Backend, signKey string, logger *slog.Logger) *Service {
	return &Service{
		pool:    pool,
		backend: backend,
		signKey: []byte(signKey),
		logger:  logger,
	}
}

// Upload stores a file and records its metadata.
func (s *Service) Upload(ctx context.Context, bucket, name, contentType string, userID *string, r io.Reader) (*Object, error) {
	if err := validateBucket(bucket); err != nil {
		return nil, err
	}
	if err := validateName(name); err != nil {
		return nil, err
	}

	size, err := s.backend.Put(ctx, bucket, name, r)
	if err != nil {
		return nil, fmt.Errorf("storing file: %w", err)
	}

	var obj Object
	err = s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_storage_objects (bucket, name, size, content_type, user_id)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (bucket, name) DO UPDATE
		 SET size = EXCLUDED.size, content_type = EXCLUDED.content_type, updated_at = NOW()
		 RETURNING id, bucket, name, size, content_type, user_id, created_at, updated_at`,
		bucket, name, size, contentType, userID,
	).Scan(&obj.ID, &obj.Bucket, &obj.Name, &obj.Size, &obj.ContentType,
		&obj.UserID, &obj.CreatedAt, &obj.UpdatedAt)
	if err != nil {
		// Clean up the stored file on DB error.
		_ = s.backend.Delete(ctx, bucket, name)
		return nil, fmt.Errorf("recording metadata: %w", err)
	}

	s.logger.Info("file uploaded", "bucket", bucket, "name", name, "size", size)
	return &obj, nil
}

// Download retrieves a file's content and metadata.
func (s *Service) Download(ctx context.Context, bucket, name string) (io.ReadCloser, *Object, error) {
	obj, err := s.GetObject(ctx, bucket, name)
	if err != nil {
		return nil, nil, err
	}

	reader, err := s.backend.Get(ctx, bucket, name)
	if err != nil {
		return nil, nil, fmt.Errorf("reading file: %w", err)
	}

	return reader, obj, nil
}

// GetObject returns the metadata for a stored file.
func (s *Service) GetObject(ctx context.Context, bucket, name string) (*Object, error) {
	var obj Object
	err := s.pool.QueryRow(ctx,
		`SELECT id, bucket, name, size, content_type, user_id, created_at, updated_at
		 FROM _ayb_storage_objects WHERE bucket = $1 AND name = $2`,
		bucket, name,
	).Scan(&obj.ID, &obj.Bucket, &obj.Name, &obj.Size, &obj.ContentType,
		&obj.UserID, &obj.CreatedAt, &obj.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying object: %w", err)
	}
	return &obj, nil
}

// DeleteObject removes a file and its metadata.
func (s *Service) DeleteObject(ctx context.Context, bucket, name string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM _ayb_storage_objects WHERE bucket = $1 AND name = $2`,
		bucket, name,
	)
	if err != nil {
		return fmt.Errorf("deleting metadata: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	if err := s.backend.Delete(ctx, bucket, name); err != nil {
		s.logger.Error("failed to delete file from backend", "bucket", bucket, "name", name, "error", err)
	}

	s.logger.Info("file deleted", "bucket", bucket, "name", name)
	return nil
}

// ListObjects lists files in a bucket with pagination.
func (s *Service) ListObjects(ctx context.Context, bucket string, prefix string, limit, offset int) ([]Object, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	// Count total.
	var total int
	countQuery := `SELECT COUNT(*) FROM _ayb_storage_objects WHERE bucket = $1`
	countArgs := []any{bucket}
	if prefix != "" {
		countQuery += ` AND name LIKE $2`
		countArgs = append(countArgs, prefix+"%")
	}
	if err := s.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting objects: %w", err)
	}

	// Fetch page.
	listQuery := `SELECT id, bucket, name, size, content_type, user_id, created_at, updated_at
		FROM _ayb_storage_objects WHERE bucket = $1`
	listArgs := []any{bucket}
	if prefix != "" {
		listQuery += ` AND name LIKE $2`
		listArgs = append(listArgs, prefix+"%")
	}
	listQuery += ` ORDER BY name`
	listQuery += fmt.Sprintf(` LIMIT %d OFFSET %d`, limit, offset)

	rows, err := s.pool.Query(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing objects: %w", err)
	}
	defer rows.Close()

	var objects []Object
	for rows.Next() {
		var obj Object
		if err := rows.Scan(&obj.ID, &obj.Bucket, &obj.Name, &obj.Size, &obj.ContentType,
			&obj.UserID, &obj.CreatedAt, &obj.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning object: %w", err)
		}
		objects = append(objects, obj)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating objects: %w", err)
	}

	return objects, total, nil
}

// SignURL generates a signed URL token for time-limited access.
func (s *Service) SignURL(bucket, name string, expiry time.Duration) string {
	exp := time.Now().Add(expiry).Unix()
	payload := fmt.Sprintf("%s/%s:%d", bucket, name, exp)
	mac := hmac.New(sha256.New, s.signKey)
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("exp=%d&sig=%s", exp, sig)
}

// ValidateSignedURL checks that a signed URL token is valid and not expired.
func (s *Service) ValidateSignedURL(bucket, name, expStr, sig string) bool {
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix() > exp {
		return false
	}
	payload := fmt.Sprintf("%s/%s:%d", bucket, name, exp)
	mac := hmac.New(sha256.New, s.signKey)
	mac.Write([]byte(payload))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expected))
}

func validateBucket(bucket string) error {
	if bucket == "" {
		return fmt.Errorf("%w: bucket name is required", ErrInvalidBucket)
	}
	if len(bucket) > 63 {
		return fmt.Errorf("%w: bucket name too long (max 63)", ErrInvalidBucket)
	}
	for _, c := range bucket {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return fmt.Errorf("%w: bucket name must contain only lowercase letters, digits, hyphens, underscores", ErrInvalidBucket)
		}
	}
	return nil
}

func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: object name is required", ErrInvalidName)
	}
	if len(name) > 1024 {
		return fmt.Errorf("%w: object name too long (max 1024)", ErrInvalidName)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("%w: object name must not contain \"..\"", ErrInvalidName)
	}
	if strings.HasPrefix(name, "/") {
		return fmt.Errorf("%w: object name must not start with \"/\"", ErrInvalidName)
	}
	return nil
}
