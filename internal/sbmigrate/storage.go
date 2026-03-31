package sbmigrate

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
)

// StorageObject represents a file from Supabase's storage.objects table.
type StorageObject struct {
	ID        string
	BucketID  string
	Name      string
	Size      int64
	MimeType  string
	CreatedAt time.Time
}

// StorageBucket represents a bucket from Supabase's storage.buckets table.
type StorageBucket struct {
	ID     string
	Name   string
	Public bool
}

type storageBucketObjects struct {
	bucket  StorageBucket
	objects []StorageObject
}

// listStorageBuckets queries storage.buckets from the Supabase source database.
func (m *Migrator) listStorageBuckets(ctx context.Context) ([]StorageBucket, error) {
	// Check if storage schema exists.
	var exists bool
	err := m.source.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'storage' AND table_name = 'buckets'
		)
	`).Scan(&exists)
	if err != nil || !exists {
		return nil, nil // no storage schema = no buckets
	}

	hasPublic, err := m.sourceColumnExists(ctx, "storage", "buckets", "public")
	if err != nil {
		return nil, fmt.Errorf("checking storage.buckets.public column: %w", err)
	}

	query := `SELECT id, name, false FROM storage.buckets ORDER BY name`
	if hasPublic {
		query = `SELECT id, name, COALESCE(public, false) FROM storage.buckets ORDER BY name`
	}

	rows, err := m.source.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying storage.buckets: %w", err)
	}
	defer rows.Close()

	var buckets []StorageBucket
	for rows.Next() {
		var b StorageBucket
		if err := rows.Scan(&b.ID, &b.Name, &b.Public); err != nil {
			return nil, fmt.Errorf("scanning bucket: %w", err)
		}
		buckets = append(buckets, b)
	}
	return buckets, rows.Err()
}

// listStorageObjects queries storage.objects from the Supabase source database
// for a given bucket.
func (m *Migrator) listStorageObjects(ctx context.Context, bucketID string) ([]StorageObject, error) {
	rows, err := m.source.QueryContext(ctx, `
		SELECT id, bucket_id, name, COALESCE(metadata->>'size', '0')::bigint,
		       COALESCE(metadata->>'mimetype', 'application/octet-stream'),
		       created_at
		FROM storage.objects
		WHERE bucket_id = $1
		ORDER BY name
	`, bucketID)
	if err != nil {
		return nil, fmt.Errorf("querying storage.objects: %w", err)
	}
	defer rows.Close()

	var objects []StorageObject
	for rows.Next() {
		var o StorageObject
		if err := rows.Scan(&o.ID, &o.BucketID, &o.Name, &o.Size, &o.MimeType, &o.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning storage object: %w", err)
		}
		objects = append(objects, o)
	}
	return objects, rows.Err()
}

func (m *Migrator) migrateStorage(ctx context.Context, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "Storage files", Index: phaseIdx, Total: totalPhases}

	buckets, err := m.listStorageBuckets(ctx)
	if err != nil {
		return fmt.Errorf("listing buckets: %w", err)
	}

	if len(buckets) == 0 {
		m.completeEmptyStoragePhase(phase)
		return nil
	}

	allBuckets, totalObjects, err := m.loadStorageBucketsWithObjects(ctx, buckets)
	if err != nil {
		return err
	}

	start := m.startStoragePhase(phase, totalObjects)
	destPath, err := m.prepareStorageDestinationRoot()
	if err != nil {
		return err
	}

	processed := 0
	for _, bucketObjects := range allBuckets {
		if err := m.copyStorageBucket(phase, totalObjects, &processed, destPath, bucketObjects); err != nil {
			return err
		}
	}

	m.progress.CompletePhase(phase, totalObjects, time.Since(start))
	fmt.Fprintf(m.output, "  %d files migrated (%s)\n",
		m.stats.StorageFiles, migrate.FormatBytes(m.stats.StorageBytes))
	return nil
}

func (m *Migrator) completeEmptyStoragePhase(phase migrate.Phase) {
	m.progress.StartPhase(phase, 0)
	m.progress.CompletePhase(phase, 0, 0)
	fmt.Fprintln(m.output, "No storage buckets found (skipping)")
}

func (m *Migrator) loadStorageBucketsWithObjects(
	ctx context.Context,
	buckets []StorageBucket,
) ([]storageBucketObjects, int, error) {
	allBuckets := make([]storageBucketObjects, 0, len(buckets))
	totalObjects := 0

	for _, bucket := range buckets {
		objects, err := m.listStorageObjects(ctx, bucket.ID)
		if err != nil {
			return nil, 0, fmt.Errorf("listing objects in bucket %s: %w", bucket.Name, err)
		}
		allBuckets = append(allBuckets, storageBucketObjects{bucket: bucket, objects: objects})
		totalObjects += len(objects)
	}

	return allBuckets, totalObjects, nil
}

func (m *Migrator) startStoragePhase(phase migrate.Phase, totalObjects int) time.Time {
	m.progress.StartPhase(phase, totalObjects)
	fmt.Fprintln(m.output, "Migrating storage files...")
	return time.Now()
}

func (m *Migrator) prepareStorageDestinationRoot() (string, error) {
	destPath := m.opts.StoragePath
	if destPath == "" {
		destPath = filepath.Join(".", "ayb_storage")
	}

	if err := os.MkdirAll(destPath, 0755); err != nil {
		return "", fmt.Errorf("creating storage directory: %w", err)
	}

	return destPath, nil
}

func (m *Migrator) copyStorageBucket(
	phase migrate.Phase,
	totalObjects int,
	processed *int,
	destPath string,
	bucketObjects storageBucketObjects,
) error {
	bucket := bucketObjects.bucket
	objects := bucketObjects.objects
	if len(objects) == 0 {
		if m.verbose {
			fmt.Fprintf(m.output, "  %s: 0 files\n", bucket.Name)
		}
		return nil
	}

	bucketName := normalizeBucketName(bucket.Name)
	bucketDir := filepath.Join(destPath, bucketName)
	if err := os.MkdirAll(bucketDir, 0755); err != nil {
		return fmt.Errorf("creating bucket directory %s: %w", bucketName, err)
	}

	copied := 0
	for _, obj := range objects {
		srcFile := filepath.Join(m.opts.StorageExportPath, bucket.Name, obj.Name)
		destFile := filepath.Join(bucketDir, obj.Name)
		if !isStoragePathWithinBucket(destFile, bucketDir) {
			m.recordStorageObjectError(phase, processed, totalObjects,
				fmt.Sprintf("skipping %s/%s: path traversal detected", bucket.Name, obj.Name))
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destFile), 0755); err != nil {
			m.recordStorageObjectError(phase, processed, totalObjects,
				fmt.Sprintf("creating directory for %s/%s: %v", bucket.Name, obj.Name, err))
			continue
		}

		bytes, err := copyFile(srcFile, destFile)
		if err != nil {
			m.recordStorageObjectError(phase, processed, totalObjects,
				fmt.Sprintf("copying %s/%s: %v", bucket.Name, obj.Name, err))
			continue
		}

		copied++
		m.stats.StorageFiles++
		m.stats.StorageBytes += bytes
		(*processed)++
		m.progress.Progress(phase, *processed, totalObjects)
	}

	if m.verbose {
		fmt.Fprintf(m.output, "  %s: %d files copied\n", bucket.Name, copied)
	}
	return nil
}

func isStoragePathWithinBucket(destFile, bucketDir string) bool {
	cleanDest := filepath.Clean(destFile)
	cleanBucket := filepath.Clean(bucketDir)
	return strings.HasPrefix(cleanDest, cleanBucket+string(filepath.Separator)) || cleanDest == cleanBucket
}

func (m *Migrator) recordStorageObjectError(
	phase migrate.Phase,
	processed *int,
	totalObjects int,
	message string,
) {
	m.stats.Errors = append(m.stats.Errors, message)
	(*processed)++
	m.progress.Progress(phase, *processed, totalObjects)
}

// copyFile copies a file from src to dst, returning bytes copied.
func copyFile(src, dst string) (int64, error) {
	sf, err := os.Open(src)
	if err != nil {
		return 0, fmt.Errorf("opening source: %w", err)
	}
	defer sf.Close()

	df, err := os.Create(dst)
	if err != nil {
		return 0, fmt.Errorf("creating destination: %w", err)
	}
	defer df.Close()

	n, err := io.Copy(df, sf)
	if err != nil {
		return 0, fmt.Errorf("copying data: %w", err)
	}

	if err := df.Sync(); err != nil {
		return n, fmt.Errorf("syncing file: %w", err)
	}

	return n, nil
}

// normalizeBucketName converts a Supabase bucket name to an AYB-compatible name.
// AYB bucket names: lowercase, letters/digits/hyphens/underscores, max 63 chars.
func normalizeBucketName(name string) string {
	name = strings.ToLower(name)
	var sb strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			sb.WriteRune(c)
		} else if c == ' ' || c == '.' {
			sb.WriteRune('-')
		}
	}
	result := sb.String()
	if len(result) > 63 {
		result = result[:63]
	}
	if result == "" {
		result = "default"
	}
	return result
}
