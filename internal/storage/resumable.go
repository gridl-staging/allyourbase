// Package storage resumable.go implements resumable upload sessions using the TUS 1.0.0 protocol, with database-backed session management and temporary file staging. It handles chunk appending, finalization to backend storage, and cleanup of expired uploads with RLS support.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
)

// Decision: we implement the TUS 1.0.0 core protocol directly in this package.
// This keeps the implementation small, avoids adding a large tusd dependency, and
// preserves first-class ownership/RLS integration via the existing Service API.

const (
	resumableUploadStatusActive     = "active"
	resumableUploadStatusFinalizing = "finalizing"
)

const resumableUploadTTL = 24 * time.Hour

var (
	ErrResumableUploadNotFound       = errors.New("resumable upload not found")
	ErrResumableUploadOffsetMismatch = errors.New("resumable upload offset mismatch")
	ErrResumableUploadExpired        = errors.New("resumable upload has expired")
	ErrResumableUploadChunkTooLarge  = errors.New("resumable upload chunk exceeds declared size")
	ErrResumableUploadInvalidState   = errors.New("resumable upload is not in an active state")
	ErrResumableUploadForbidden      = errors.New("resumable upload forbidden: not the upload owner")
)

// enforceUploadOwnership checks that the caller (identified by callerUserID) is
// the owner of the upload. A nil callerUserID (admin) bypasses the check.
// Ownerless uploads (UserID is nil) are reserved for admin-only access.
func enforceUploadOwnership(upload *ResumableUpload, callerUserID *string) error {
	if callerUserID == nil {
		return nil // admin bypass
	}
	if upload.UserID == nil {
		return ErrResumableUploadForbidden
	}
	if *upload.UserID != *callerUserID {
		return ErrResumableUploadForbidden
	}
	return nil
}

// ResumableUpload describes resumable upload session state stored in
// _ayb_storage_uploads.
type ResumableUpload struct {
	ID           string    `json:"id"`
	Bucket       string    `json:"bucket"`
	Name         string    `json:"name"`
	Path         string    `json:"-"`
	ContentType  string    `json:"contentType"`
	UserID       *string   `json:"userId,omitempty"`
	TotalSize    int64     `json:"totalSize"`
	UploadedSize int64     `json:"uploadedSize"`
	Status       string    `json:"status"`
	ExpiresAt    time.Time `json:"expiresAt"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type resumableRow interface {
	Scan(dest ...any) error
}

type resumableQueryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

const resumableUploadSelectQuery = `
SELECT id, bucket, name, path, content_type, user_id,
       total_size, uploaded_size, status, expires_at, created_at, updated_at
  FROM _ayb_storage_uploads
 WHERE id = $1`

func scanResumableUpload(row resumableRow) (*ResumableUpload, error) {
	var upload ResumableUpload
	if err := row.Scan(
		&upload.ID, &upload.Bucket, &upload.Name, &upload.Path, &upload.ContentType,
		&upload.UserID, &upload.TotalSize, &upload.UploadedSize, &upload.Status,
		&upload.ExpiresAt, &upload.CreatedAt, &upload.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrResumableUploadNotFound
		}
		return nil, fmt.Errorf("querying resumable upload: %w", err)
	}
	return &upload, nil
}

func getResumableUpload(ctx context.Context, q resumableQueryer, id string, lock bool) (*ResumableUpload, error) {
	query := resumableUploadSelectQuery
	if lock {
		query = query + " FOR UPDATE"
	}
	row := q.QueryRow(ctx, query, id)
	return scanResumableUpload(row)
}

// appends a chunk from src to the file at path starting at offset, limited to remaining bytes. It validates offset consistency and truncates the file if necessary. Returns the number of bytes written or an error if the chunk would exceed remaining bytes.
func appendChunk(path string, offset int64, remaining int64, src io.Reader) (int64, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		return 0, fmt.Errorf("opening resumable file: %w", err)
	}
	defer f.Close()

	if offset < 0 {
		return 0, fmt.Errorf("offset must not be negative")
	}

	existing, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, fmt.Errorf("seeking resumable file: %w", err)
	}
	if existing < offset {
		return 0, ErrResumableUploadOffsetMismatch
	}
	if existing != offset {
		if err := f.Truncate(offset); err != nil {
			return 0, fmt.Errorf("rewinding resumable file: %w", err)
		}
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return 0, fmt.Errorf("positioning resumable file: %w", err)
	}

	if remaining <= 0 {
		return 0, ErrResumableUploadInvalidState
	}

	limited := io.LimitReader(src, remaining+1)
	written, err := io.Copy(f, limited)
	if err != nil {
		return written, fmt.Errorf("writing resumable chunk: %w", err)
	}
	if written > remaining {
		return written, ErrResumableUploadChunkTooLarge
	}
	return written, nil
}

// CreateResumableUpload creates a new resumable session record and returns it.
func (s *Service) CreateResumableUpload(ctx context.Context, bucket, name, contentType string, userID *string, totalSize int64) (*ResumableUpload, error) {
	if err := validateBucket(bucket); err != nil {
		return nil, err
	}
	if err := validateName(name); err != nil {
		return nil, err
	}
	if totalSize <= 0 {
		return nil, fmt.Errorf("upload length must be greater than 0")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if s.pool == nil {
		return nil, fmt.Errorf("database pool is not configured")
	}

	f, err := os.CreateTemp("", "ayb-resumable-*")
	if err != nil {
		return nil, fmt.Errorf("creating resumable temp file: %w", err)
	}
	tempPath := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(tempPath)
		return nil, fmt.Errorf("closing resumable temp file: %w", err)
	}
	expiresAt := time.Now().Add(resumableUploadTTL)

	query := `
		INSERT INTO _ayb_storage_uploads
		(bucket, name, path, content_type, user_id, total_size, uploaded_size, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, 0, $7, $8)
		RETURNING id, bucket, name, path, content_type, user_id, total_size,
		          uploaded_size, status, expires_at, created_at, updated_at`

	var upload ResumableUpload
	err = s.pool.QueryRow(
		ctx, query,
		bucket, name, tempPath, contentType, userID, totalSize,
		resumableUploadStatusActive, expiresAt,
	).Scan(
		&upload.ID, &upload.Bucket, &upload.Name, &upload.Path, &upload.ContentType, &upload.UserID,
		&upload.TotalSize, &upload.UploadedSize, &upload.Status, &upload.ExpiresAt,
		&upload.CreatedAt, &upload.UpdatedAt,
	)
	if err != nil {
		_ = os.Remove(tempPath)
		return nil, fmt.Errorf("creating resumable upload: %w", err)
	}

	return &upload, nil
}

// GetResumableUpload returns resumable session metadata by ID.
// callerUserID enforces ownership: non-nil means the caller must own the upload,
// nil means admin bypass. Returns ErrResumableUploadExpired if the session has expired.
func (s *Service) GetResumableUpload(ctx context.Context, id string, callerUserID *string) (*ResumableUpload, error) {
	if id == "" {
		return nil, ErrResumableUploadNotFound
	}
	if s.pool == nil {
		return nil, fmt.Errorf("database pool is not configured")
	}
	upload, err := getResumableUpload(ctx, s.pool, id, false)
	if err != nil {
		return nil, err
	}
	if time.Now().After(upload.ExpiresAt) {
		return nil, ErrResumableUploadExpired
	}
	if err := enforceUploadOwnership(upload, callerUserID); err != nil {
		return nil, err
	}
	return upload, nil
}

// AppendResumableUpload writes one chunk into a resumable upload session.
// callerUserID enforces ownership (nil = admin bypass).
// The bool return indicates whether the upload is now ready to finalize.
func (s *Service) AppendResumableUpload(ctx context.Context, id string, offset int64, callerUserID *string, src io.Reader) (*ResumableUpload, bool, error) {
	if id == "" {
		return nil, false, ErrResumableUploadNotFound
	}
	if s.pool == nil {
		return nil, false, fmt.Errorf("database pool is not configured")
	}
	if src == nil {
		return nil, false, fmt.Errorf("upload data is required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("begin upload tx: %w", err)
	}
	defer tx.Rollback(ctx)

	upload, err := getResumableUpload(ctx, tx, id, true)
	if err != nil {
		return nil, false, err
	}

	if err := enforceUploadOwnership(upload, callerUserID); err != nil {
		return nil, false, err
	}
	if time.Now().After(upload.ExpiresAt) {
		return nil, false, ErrResumableUploadExpired
	}
	if upload.Status != resumableUploadStatusActive {
		return nil, false, ErrResumableUploadInvalidState
	}
	if offset != upload.UploadedSize {
		return nil, false, ErrResumableUploadOffsetMismatch
	}

	remaining := upload.TotalSize - upload.UploadedSize
	written, err := appendChunk(upload.Path, offset, remaining, src)
	if err != nil {
		return nil, false, err
	}
	upload.UploadedSize += written

	status := resumableUploadStatusActive
	shouldFinalize := false
	if upload.UploadedSize == upload.TotalSize {
		status = resumableUploadStatusFinalizing
		shouldFinalize = true
	}

	_, err = tx.Exec(ctx,
		`UPDATE _ayb_storage_uploads
		 SET uploaded_size = $1, status = $2, updated_at = NOW(), expires_at = NOW() + make_interval(secs => $3)
		 WHERE id = $4`,
		upload.UploadedSize, status, int64(resumableUploadTTL.Seconds()), upload.ID)
	if err != nil {
		return nil, false, fmt.Errorf("updating resumable upload progress: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit resumable upload tx: %w", err)
	}

	upload.Status = status
	return upload, shouldFinalize, nil
}

// moves a completed resumable upload's temporary file to backend storage and records it in the database. It applies row-level security constraints and cleans up the backend object if the database insert fails.
func (s *Service) finalizeUploadObject(ctx context.Context, upload *ResumableUpload) (*Object, error) {
	f, err := os.Open(upload.Path)
	if err != nil {
		return nil, fmt.Errorf("opening resumable file: %w", err)
	}
	defer f.Close()

	size, err := s.backend.Put(ctx, upload.Bucket, upload.Name, f)
	if err != nil {
		return nil, err
	}

	q, done, err := s.withRLS(ctx)
	if err != nil {
		_ = s.backend.Delete(ctx, upload.Bucket, upload.Name)
		return nil, err
	}

	var obj Object
	err = q.QueryRow(ctx,
		`INSERT INTO _ayb_storage_objects (bucket, name, size, content_type, user_id)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (bucket, name) DO UPDATE
		 SET size = EXCLUDED.size, content_type = EXCLUDED.content_type, user_id = EXCLUDED.user_id, updated_at = NOW()
		 RETURNING id, bucket, name, size, content_type, user_id, created_at, updated_at`,
		upload.Bucket, upload.Name, size, upload.ContentType, upload.UserID,
	).Scan(&obj.ID, &obj.Bucket, &obj.Name, &obj.Size, &obj.ContentType, &obj.UserID, &obj.CreatedAt, &obj.UpdatedAt)
	if err != nil {
		_ = s.backend.Delete(ctx, upload.Bucket, upload.Name)
		_ = done(err)
		if isPermissionDenied(err) {
			return nil, ErrPermissionDenied
		}
		return nil, fmt.Errorf("recording resumable object: %w", err)
	}
	if err := done(nil); err != nil {
		_ = s.backend.Delete(ctx, upload.Bucket, upload.Name)
		return nil, fmt.Errorf("recording resumable object: %w", err)
	}

	return &obj, nil
}

// FinalizeResumableUpload moves a completed resumable upload into bucket storage.
// callerUserID enforces ownership (nil = admin bypass).
// Uses FOR UPDATE SKIP LOCKED to prevent concurrent finalize race conditions — the
// lock is held for the duration of the finalize rather than released before the work.
func (s *Service) FinalizeResumableUpload(ctx context.Context, id string, callerUserID *string) (*Object, error) {
	if id == "" {
		return nil, ErrResumableUploadNotFound
	}
	if s.pool == nil {
		return nil, fmt.Errorf("database pool is not configured")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin upload-finalize tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// SKIP LOCKED: if another goroutine is already finalizing this upload,
	// we get no rows rather than blocking, preventing double-finalize.
	row := tx.QueryRow(ctx, resumableUploadSelectQuery+" FOR UPDATE SKIP LOCKED", id)
	upload, err := scanResumableUpload(row)
	if err != nil {
		return nil, err
	}

	if err := enforceUploadOwnership(upload, callerUserID); err != nil {
		return nil, err
	}
	if upload.Status != resumableUploadStatusFinalizing {
		return nil, ErrResumableUploadInvalidState
	}
	if upload.UploadedSize != upload.TotalSize {
		return nil, ErrResumableUploadInvalidState
	}
	if time.Now().After(upload.ExpiresAt) {
		return nil, ErrResumableUploadExpired
	}

	// Finalize: move temp file to backend and create object record.
	// The lock tx stays open to prevent concurrent finalize attempts.
	obj, err := s.finalizeUploadObject(ctx, upload)
	if err != nil {
		// Reset status so the client can retry.
		_, _ = tx.Exec(ctx, `UPDATE _ayb_storage_uploads
			SET status = $1, updated_at = NOW()
			WHERE id = $2`, resumableUploadStatusActive, id)
		_ = tx.Commit(ctx)
		return nil, err
	}

	// Delete the upload record within the lock tx.
	if _, err := tx.Exec(ctx, `DELETE FROM _ayb_storage_uploads WHERE id = $1`, id); err != nil {
		return nil, fmt.Errorf("removing resumable upload: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit finalize tx: %w", err)
	}

	if err := os.Remove(upload.Path); err != nil && !os.IsNotExist(err) {
		s.logger.Warn("failed to remove resumable temp file", "path", upload.Path, "error", err)
	}

	return obj, nil
}

// CleanupExpiredResumableUploads deletes stale resumable sessions and their temp files.
// It returns the number of stale session rows removed.
func (s *Service) CleanupExpiredResumableUploads(ctx context.Context) (int, error) {
	if s.pool == nil {
		return 0, fmt.Errorf("database pool is not configured")
	}

	rows, err := s.pool.Query(ctx, `SELECT path FROM _ayb_storage_uploads WHERE expires_at < NOW()`)
	if err != nil {
		return 0, fmt.Errorf("querying expired resumable uploads: %w", err)
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return 0, fmt.Errorf("scanning expired resumable upload: %w", err)
		}
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("reading expired resumable uploads: %w", err)
	}

	tag, err := s.pool.Exec(ctx, `DELETE FROM _ayb_storage_uploads WHERE expires_at < NOW()`)
	if err != nil {
		return 0, fmt.Errorf("deleting expired resumable uploads: %w", err)
	}

	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			s.logger.Warn("failed to remove resumable temp file", "path", path, "error", err)
		}
	}

	return int(tag.RowsAffected()), nil
}
