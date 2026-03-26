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

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/allyourbase/ayb/internal/observability"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sentinel errors.
var (
	ErrNotFound         = errors.New("object not found")
	ErrAlreadyExists    = errors.New("object already exists")
	ErrInvalidBucket    = errors.New("invalid bucket name")
	ErrInvalidName      = errors.New("invalid object name")
	ErrPermissionDenied = errors.New("permission denied")
	ErrBucketNotFound   = errors.New("bucket not found")
	ErrBucketNotEmpty   = errors.New("bucket has objects")
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
	pool              *pgxpool.Pool
	backend           Backend
	signKey           []byte
	logger            *slog.Logger
	defaultQuotaBytes int64
	eventHandlers     []StorageEventHandler
}

// NewService creates a new storage service.
func NewService(pool *pgxpool.Pool, backend Backend, signKey string, logger *slog.Logger, defaultQuotaBytes int64) *Service {
	return &Service{
		pool:              pool,
		backend:           backend,
		signKey:           []byte(signKey),
		logger:            logger,
		defaultQuotaBytes: defaultQuotaBytes,
	}
}

// RegisterEventHandler adds a handler that will be notified of storage events.
func (s *Service) RegisterEventHandler(h StorageEventHandler) {
	s.eventHandlers = append(s.eventHandlers, h)
}

// dispatchEvent notifies all registered handlers of a storage event.
// Handler errors are logged but not propagated to the caller.
func (s *Service) dispatchEvent(ctx context.Context, event StorageEvent) {
	for _, h := range s.eventHandlers {
		if err := h.OnStorageEvent(ctx, event); err != nil {
			s.logger.Error("storage event handler failed",
				"bucket", event.Bucket,
				"name", event.Name,
				"operation", event.Operation,
				"error", err,
			)
		}
	}
}

// Upload stores a file and records its metadata.
func (s *Service) Upload(ctx context.Context, bucket, name, contentType string, userID *string, r io.Reader) (*Object, error) {
	ctx, span := otel.Tracer("ayb/storage").Start(ctx, "storage.upload",
		trace.WithAttributes(
			attribute.String("storage.bucket", bucket),
			attribute.String("storage.object", name),
		),
	)
	defer span.End()
	if err := validateBucket(bucket); err != nil {
		observability.RecordSpanError(span, err)
		return nil, err
	}
	if err := validateName(name); err != nil {
		observability.RecordSpanError(span, err)
		return nil, err
	}

	size, err := s.backend.Put(ctx, bucket, name, r)
	if err != nil {
		observability.RecordSpanError(span, err)
		return nil, fmt.Errorf("storing file: %w", err)
	}

	q, done, err := s.withRLS(ctx)
	if err != nil {
		_ = s.backend.Delete(ctx, bucket, name)
		observability.RecordSpanError(span, err)
		return nil, fmt.Errorf("setting storage rls context: %w", err)
	}

	var obj Object
	err = q.QueryRow(ctx,
		`INSERT INTO _ayb_storage_objects (bucket, name, size, content_type, user_id)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (bucket, name) DO UPDATE
		 SET size = EXCLUDED.size, content_type = EXCLUDED.content_type, updated_at = NOW()
		 RETURNING id, bucket, name, size, content_type, user_id, created_at, updated_at`,
		bucket, name, size, contentType, userID,
	).Scan(&obj.ID, &obj.Bucket, &obj.Name, &obj.Size, &obj.ContentType,
		&obj.UserID, &obj.CreatedAt, &obj.UpdatedAt)
	if err != nil {
		_ = done(err)
		// Clean up the stored file on DB error.
		_ = s.backend.Delete(ctx, bucket, name)
		if isPermissionDenied(err) {
			observability.RecordSpanError(span, ErrPermissionDenied)
			return nil, ErrPermissionDenied
		}
		observability.RecordSpanError(span, err)
		return nil, fmt.Errorf("recording metadata: %w", err)
	}
	if err := done(nil); err != nil {
		_ = s.backend.Delete(ctx, bucket, name)
		observability.RecordSpanError(span, err)
		return nil, fmt.Errorf("recording metadata: %w", err)
	}

	s.logger.Info("file uploaded", "bucket", bucket, "name", name, "size", size)

	s.dispatchEvent(ctx, StorageEvent{
		Bucket:      bucket,
		Name:        name,
		Operation:   OperationUpload,
		Size:        size,
		ContentType: contentType,
	})

	return &obj, nil
}

// Download retrieves a file's content and metadata.
func (s *Service) Download(ctx context.Context, bucket, name string) (io.ReadCloser, *Object, error) {
	ctx, span := otel.Tracer("ayb/storage").Start(ctx, "storage.download",
		trace.WithAttributes(
			attribute.String("storage.bucket", bucket),
			attribute.String("storage.object", name),
		),
	)
	defer span.End()
	obj, err := s.GetObject(ctx, bucket, name)
	if err != nil {
		observability.RecordSpanError(span, err)
		return nil, nil, err
	}

	reader, err := s.backend.Get(ctx, bucket, name)
	if err != nil {
		observability.RecordSpanError(span, err)
		return nil, nil, fmt.Errorf("reading file: %w", err)
	}

	return reader, obj, nil
}

// GetObject returns the metadata for a stored file.
func (s *Service) GetObject(ctx context.Context, bucket, name string) (*Object, error) {
	q, done, err := s.withRLS(ctx)
	if err != nil {
		return nil, fmt.Errorf("setting storage rls context: %w", err)
	}

	var obj Object
	err = q.QueryRow(ctx,
		`SELECT id, bucket, name, size, content_type, user_id, created_at, updated_at
		 FROM _ayb_storage_objects WHERE bucket = $1 AND name = $2`,
		bucket, name,
	).Scan(&obj.ID, &obj.Bucket, &obj.Name, &obj.Size, &obj.ContentType,
		&obj.UserID, &obj.CreatedAt, &obj.UpdatedAt)
	if err != nil {
		_ = done(err)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if isPermissionDenied(err) {
			return nil, ErrPermissionDenied
		}
		return nil, fmt.Errorf("querying object: %w", err)
	}
	if err := done(nil); err != nil {
		return nil, fmt.Errorf("querying object: %w", err)
	}
	return &obj, nil
}

// DeleteObject removes a file and its metadata.
func (s *Service) DeleteObject(ctx context.Context, bucket, name string) error {
	ctx, span := otel.Tracer("ayb/storage").Start(ctx, "storage.delete",
		trace.WithAttributes(
			attribute.String("storage.bucket", bucket),
			attribute.String("storage.object", name),
		),
	)
	defer span.End()
	if err := validateBucket(bucket); err != nil {
		observability.RecordSpanError(span, err)
		return err
	}
	if err := validateName(name); err != nil {
		observability.RecordSpanError(span, err)
		return err
	}

	q, done, err := s.withRLS(ctx)
	if err != nil {
		observability.RecordSpanError(span, err)
		return fmt.Errorf("setting storage rls context: %w", err)
	}

	tag, err := q.Exec(ctx,
		`DELETE FROM _ayb_storage_objects WHERE bucket = $1 AND name = $2`,
		bucket, name,
	)
	if err != nil {
		_ = done(err)
		if isPermissionDenied(err) {
			observability.RecordSpanError(span, ErrPermissionDenied)
			return ErrPermissionDenied
		}
		observability.RecordSpanError(span, err)
		return fmt.Errorf("deleting metadata: %w", err)
	}
	if tag.RowsAffected() == 0 {
		_ = done(nil)
		observability.RecordSpanError(span, ErrNotFound)
		return ErrNotFound
	}
	if err := done(nil); err != nil {
		observability.RecordSpanError(span, err)
		return fmt.Errorf("deleting metadata: %w", err)
	}

	if err := s.backend.Delete(ctx, bucket, name); err != nil {
		s.logger.Error("failed to delete file from backend", "bucket", bucket, "name", name, "error", err)
	}

	s.logger.Info("file deleted", "bucket", bucket, "name", name)

	s.dispatchEvent(ctx, StorageEvent{
		Bucket:    bucket,
		Name:      name,
		Operation: OperationDelete,
	})

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

	q, done, err := s.withRLS(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("setting storage rls context: %w", err)
	}

	// Count total.
	var total int
	countQuery := `SELECT COUNT(*) FROM _ayb_storage_objects WHERE bucket = $1`
	countArgs := []any{bucket}
	if prefix != "" {
		countQuery += ` AND name LIKE $2 ESCAPE '\'`
		countArgs = append(countArgs, escapeLikePrefix(prefix)+"%")
	}
	if err := q.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		_ = done(err)
		if isPermissionDenied(err) {
			return nil, 0, ErrPermissionDenied
		}
		return nil, 0, fmt.Errorf("counting objects: %w", err)
	}

	// Fetch page.
	listQuery := `SELECT id, bucket, name, size, content_type, user_id, created_at, updated_at
		FROM _ayb_storage_objects WHERE bucket = $1`
	listArgs := []any{bucket}
	if prefix != "" {
		listQuery += ` AND name LIKE $2 ESCAPE '\'`
		listArgs = append(listArgs, escapeLikePrefix(prefix)+"%")
	}
	listQuery += ` ORDER BY name`
	listQuery += fmt.Sprintf(` LIMIT %d OFFSET %d`, limit, offset)

	rows, err := q.Query(ctx, listQuery, listArgs...)
	if err != nil {
		_ = done(err)
		if isPermissionDenied(err) {
			return nil, 0, ErrPermissionDenied
		}
		return nil, 0, fmt.Errorf("listing objects: %w", err)
	}
	defer rows.Close()

	var objects []Object
	for rows.Next() {
		var obj Object
		if err := rows.Scan(&obj.ID, &obj.Bucket, &obj.Name, &obj.Size, &obj.ContentType,
			&obj.UserID, &obj.CreatedAt, &obj.UpdatedAt); err != nil {
			_ = done(err)
			return nil, 0, fmt.Errorf("scanning object: %w", err)
		}
		objects = append(objects, obj)
	}
	if err := rows.Err(); err != nil {
		_ = done(err)
		return nil, 0, fmt.Errorf("iterating objects: %w", err)
	}
	if err := done(nil); err != nil {
		return nil, 0, fmt.Errorf("listing objects: %w", err)
	}

	return objects, total, nil
}

func isPermissionDenied(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42501"
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

// escapeLikePrefix escapes SQL LIKE metacharacters (%, _, \) in a user-provided
// prefix so they are treated as literal characters. The result should be used with
// the ESCAPE '\' clause.
func escapeLikePrefix(prefix string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(prefix)
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
