package fbmigrate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
)

// StorageFileInfo holds metadata about a file discovered during storage scanning.
type StorageFileInfo struct {
	Bucket   string
	Path     string // relative path within the bucket
	FullPath string // absolute filesystem path
	Size     int64
}

// scanStorageExport walks the storage export directory and returns all files
// grouped by bucket. The expected directory layout is:
//
//	<export-dir>/<bucket-name>/<file-path>
//
// Top-level directories are treated as bucket names.
func scanStorageExport(exportPath string) (map[string][]StorageFileInfo, error) {
	entries, err := os.ReadDir(exportPath)
	if err != nil {
		return nil, fmt.Errorf("reading storage export directory: %w", err)
	}

	buckets := make(map[string][]StorageFileInfo)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue // skip non-directory entries at top level
		}

		bucketName := entry.Name()
		bucketPath := filepath.Join(exportPath, bucketName)

		err := filepath.Walk(bucketPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // skip inaccessible files
			}
			if info.IsDir() {
				return nil
			}

			relPath, err := filepath.Rel(bucketPath, path)
			if err != nil {
				return nil // skip if can't determine relative path
			}

			buckets[bucketName] = append(buckets[bucketName], StorageFileInfo{
				Bucket:   bucketName,
				Path:     relPath,
				FullPath: path,
				Size:     info.Size(),
			})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scanning bucket %s: %w", bucketName, err)
		}
	}

	return buckets, nil
}

// normalizeFirebaseBucketName converts a Firebase/GCS bucket name to an
// AYB-compatible bucket name (lowercase, alphanumeric with hyphens/underscores).
func normalizeFirebaseBucketName(name string) string {
	name = strings.ToLower(name)
	var sb strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			sb.WriteRune(c)
		} else if c == '.' || c == ' ' {
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

// migrateStorage copies files from a Firebase Cloud Storage export directory
// to AYB's local storage layout.
func (m *Migrator) migrateStorage(phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "Storage files", Index: phaseIdx, Total: totalPhases}

	buckets, err := scanStorageExport(m.opts.StorageExportPath)
	if err != nil {
		return err
	}

	// Count total files.
	var totalFiles int
	for _, files := range buckets {
		totalFiles += len(files)
	}

	if totalFiles == 0 {
		m.progress.StartPhase(phase, 0)
		m.progress.CompletePhase(phase, 0, 0)
		fmt.Fprintln(m.output, "No storage files found (skipping)")
		return nil
	}

	m.progress.StartPhase(phase, totalFiles)
	start := time.Now()

	fmt.Fprintln(m.output, "Migrating storage files...")

	destPath := m.opts.StoragePath
	if destPath == "" {
		destPath = filepath.Join(".", "ayb_storage")
	}

	if err := os.MkdirAll(destPath, 0755); err != nil {
		return fmt.Errorf("creating storage directory: %w", err)
	}

	// Sort bucket names for deterministic processing order.
	bucketNames := make([]string, 0, len(buckets))
	for name := range buckets {
		bucketNames = append(bucketNames, name)
	}
	sort.Strings(bucketNames)

	processed := 0
	for _, bucketName := range bucketNames {
		files := buckets[bucketName]
		normalized := normalizeFirebaseBucketName(bucketName)
		bucketDir := filepath.Join(destPath, normalized)

		if err := os.MkdirAll(bucketDir, 0755); err != nil {
			return fmt.Errorf("creating bucket directory %s: %w", normalized, err)
		}

		copied := 0
		for _, f := range files {
			destFile := filepath.Join(bucketDir, f.Path)

			if err := os.MkdirAll(filepath.Dir(destFile), 0755); err != nil {
				m.stats.Errors = append(m.stats.Errors,
					fmt.Sprintf("creating directory for %s/%s: %v", bucketName, f.Path, err))
				processed++
				m.progress.Progress(phase, processed, totalFiles)
				continue
			}

			bytes, err := copyFileFS(f.FullPath, destFile)
			if err != nil {
				m.stats.Errors = append(m.stats.Errors,
					fmt.Sprintf("copying %s/%s: %v", bucketName, f.Path, err))
				processed++
				m.progress.Progress(phase, processed, totalFiles)
				continue
			}

			copied++
			m.stats.StorageFiles++
			m.stats.StorageBytes += bytes
			processed++
			m.progress.Progress(phase, processed, totalFiles)
		}

		if m.verbose {
			fmt.Fprintf(m.output, "  %s: %d files copied\n", bucketName, copied)
		}
	}

	m.progress.CompletePhase(phase, totalFiles, time.Since(start))
	fmt.Fprintf(m.output, "  %d files migrated (%s)\n",
		m.stats.StorageFiles, migrate.FormatBytes(m.stats.StorageBytes))
	return nil
}

// copyFileFS copies a file from src to dst, returning bytes written.
func copyFileFS(src, dst string) (int64, error) {
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
