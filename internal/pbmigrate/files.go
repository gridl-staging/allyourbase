// Package pbmigrate Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar19_03_go_code_quality_refactoring/allyourbase_dev/internal/pbmigrate/files.go.
package pbmigrate

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type fileCopyTotals struct {
	files       int
	bytes       int64
	warnings    int
	failedFiles []string // "collection/relpath" for each failure
}

// TODO: Document Migrator.migrateFiles.
func (m *Migrator) migrateFiles(ctx context.Context, collections []PBCollection) error {
	_ = ctx
	fmt.Fprintln(m.output, "Migrating files...")

	sourceStoragePath := filepath.Join(m.opts.SourcePath, "storage")
	if _, err := os.Stat(sourceStoragePath); os.IsNotExist(err) {
		fmt.Fprintln(m.output, "  No storage directory found (skipping)")
		fmt.Fprintln(m.output, "")
		return nil
	}

	// Find collections with file fields
	fileCollections := getCollectionsWithFiles(collections)
	if len(fileCollections) == 0 {
		if m.verbose {
			fmt.Fprintln(m.output, "  No collections with file fields")
		}
		fmt.Fprintln(m.output, "")
		return nil
	}

	storagePath, err := m.resolveTargetStoragePath()
	if err != nil {
		return err
	}

	// Create storage directory if it doesn't exist
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	total := fileCopyTotals{}

	for _, coll := range fileCollections {
		if err := validateCollectionStorageKey("collection name", coll.Name); err != nil {
			return err
		}
		collectionPath, err := m.findCollectionStoragePath(coll)
		if err != nil {
			return err
		}
		if collectionPath == "" {
			if m.verbose {
				fmt.Fprintf(m.output, "  %s: no files (skipping)\n", coll.Name)
			}
			continue
		}

		collTotal, err := m.copyCollectionFiles(coll, collectionPath, storagePath)
		if err != nil {
			return err
		}

		total.files += collTotal.files
		total.bytes += collTotal.bytes
		total.warnings += collTotal.warnings
		m.stats.FailedFiles = append(m.stats.FailedFiles, collTotal.failedFiles...)
	}

	if total.warnings > 0 {
		fmt.Fprintf(m.output, "  Warnings: %d files failed to copy\n", total.warnings)
	}

	fmt.Fprintln(m.output, "")
	return nil
}

func (m *Migrator) resolveTargetStoragePath() (string, error) {
	storageBackend := "local"
	storagePath := filepath.Join(".", "ayb_storage") // Default AYB storage path
	if m.opts.StorageBackend != "" {
		storageBackend = m.opts.StorageBackend
	}
	if m.opts.StoragePath != "" {
		storagePath = m.opts.StoragePath
	}
	if storageBackend == "s3" {
		return "", fmt.Errorf("S3 storage backend not yet implemented for file migration")
	}
	return storagePath, nil
}

// TODO: Document Migrator.findCollectionStoragePath.
func (m *Migrator) findCollectionStoragePath(coll PBCollection) (string, error) {
	candidates := []string{
		filepath.Join(m.opts.SourcePath, "storage", coll.Name), // older PB layout
	}
	if coll.ID != "" {
		if err := validateCollectionStorageKey("collection id", coll.ID); err != nil {
			return "", err
		}
		candidates = append(candidates, filepath.Join(m.opts.SourcePath, "storage", coll.ID)) // newer PB layout
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate, nil
		}
	}
	return "", nil
}

func countCollectionFiles(collectionPath string) int {
	fileCount := 0
	filepath.Walk(collectionPath, func(path string, info os.FileInfo, err error) error {
		if err == nil &&
			!info.IsDir() &&
			info.Mode()&os.ModeSymlink == 0 &&
			!strings.HasSuffix(info.Name(), ".attrs") {
			fileCount++
		}
		return nil
	})
	return fileCount
}

// TODO: Document Migrator.copyCollectionFiles.
func (m *Migrator) copyCollectionFiles(coll PBCollection, collectionPath, storagePath string) (fileCopyTotals, error) {
	total := fileCopyTotals{}

	bucketPath := filepath.Join(storagePath, coll.Name)
	if err := os.MkdirAll(bucketPath, 0755); err != nil {
		return total, fmt.Errorf("failed to create bucket %s: %w", coll.Name, err)
	}

	if countCollectionFiles(collectionPath) == 0 {
		if m.verbose {
			fmt.Fprintf(m.output, "  %s: 0 files\n", coll.Name)
		}
		return total, nil
	}

	err := filepath.Walk(collectionPath, func(sourcePath string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			rel, relErr := filepath.Rel(collectionPath, sourcePath)
			if relErr != nil {
				rel = filepath.Base(sourcePath)
			}
			m.recordFileCopyFailure(&total, coll.Name, rel, sourcePath, walkErr)
			return nil
		}
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 || strings.HasSuffix(info.Name(), ".attrs") {
			return nil
		}

		relPath, err := filepath.Rel(collectionPath, sourcePath)
		if err != nil {
			m.recordFileCopyFailure(&total, coll.Name, filepath.Base(sourcePath), sourcePath, err)
			return nil
		}
		if !isSafeMigrationRelativePath(relPath) {
			m.recordFileCopyFailure(&total, coll.Name, relPath, sourcePath, fmt.Errorf("path outside collection root"))
			return nil
		}

		destPath := filepath.Join(bucketPath, relPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			m.recordFileCopyFailure(&total, coll.Name, relPath, sourcePath, err)
			return nil
		}

		bytes, err := copyFile(sourcePath, destPath)
		if err != nil {
			m.recordFileCopyFailure(&total, coll.Name, relPath, sourcePath, err)
			return nil
		}
		total.files++
		total.bytes += bytes
		m.stats.Files++
		return nil
	})
	if err != nil {
		return total, fmt.Errorf("failed to walk collection storage %s: %w", coll.Name, err)
	}

	fmt.Fprintf(m.output, "  %s: %d files copied\n", coll.Name, total.files)
	return total, nil
}

// recordFileCopyFailure logs a warning and records the failed file path.
func (m *Migrator) recordFileCopyFailure(total *fileCopyTotals, collName, relPath, sourcePath string, err error) {
	if m.verbose {
		fmt.Fprintf(m.output, "    Warning: %s: %v\n", sourcePath, err)
	}
	total.warnings++
	normalizedRelPath := strings.ReplaceAll(filepath.ToSlash(relPath), `\`, "/")
	total.failedFiles = append(total.failedFiles, path.Join(collName, normalizedRelPath))
}

// getCollectionsWithFiles returns collections that have file fields
func getCollectionsWithFiles(collections []PBCollection) []PBCollection {
	var result []PBCollection
	for _, coll := range collections {
		if coll.System || coll.Type == "view" {
			continue
		}
		hasFileField := false
		for _, field := range coll.Schema {
			if field.Type == "file" {
				hasFileField = true
				break
			}
		}
		if hasFileField {
			result = append(result, coll)
		}
	}
	return result
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) (int64, error) {
	sourceInfo, err := os.Lstat(src)
	if err != nil {
		return 0, err
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 {
		return 0, fmt.Errorf("refusing to copy symlink source: %s", src)
	}
	if !sourceInfo.Mode().IsRegular() {
		return 0, fmt.Errorf("source is not a regular file: %s", src)
	}

	sourceFile, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destFile.Close()

	bytes, err := io.Copy(destFile, sourceFile)
	if err != nil {
		return 0, err
	}

	// Sync to disk
	if err := destFile.Sync(); err != nil {
		return bytes, err
	}

	return bytes, nil
}

func validateCollectionStorageKey(label, value string) error {
	if value == "" {
		return fmt.Errorf("invalid %s: value is empty", label)
	}
	if value == "." || value == ".." || strings.ContainsAny(value, `/\`) || filepath.Base(value) != value {
		return fmt.Errorf("invalid %s %q: path traversal is not allowed", label, value)
	}
	return nil
}

func isSafeMigrationRelativePath(relPath string) bool {
	cleanRel := filepath.Clean(relPath)
	if cleanRel == "." {
		return true
	}
	if filepath.IsAbs(cleanRel) {
		return false
	}
	return cleanRel != ".." && !strings.HasPrefix(cleanRel, ".."+string(filepath.Separator))
}
