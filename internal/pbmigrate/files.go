package pbmigrate

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// migrateFiles copies files from PocketBase storage to AYB storage
func (m *Migrator) migrateFiles(ctx context.Context, collections []PBCollection) error {
	fmt.Fprintln(m.output, "Migrating files...")

	storagePath := filepath.Join(m.opts.SourcePath, "storage")
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
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

	// Determine storage backend (default to local)
	storageBackend := "local"
	storagePath = filepath.Join(".", "ayb_storage") // Default AYB storage path
	if m.opts.StorageBackend != "" {
		storageBackend = m.opts.StorageBackend
	}
	if m.opts.StoragePath != "" {
		storagePath = m.opts.StoragePath
	}

	if storageBackend == "s3" {
		return fmt.Errorf("S3 storage backend not yet implemented for file migration")
	}

	// Create storage directory if it doesn't exist
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	totalFiles := 0
	totalBytes := int64(0)
	errorCount := 0

	for _, coll := range fileCollections {
		collectionPath := ""
		candidates := []string{
			filepath.Join(m.opts.SourcePath, "storage", coll.Name), // older PB layout
		}
		if coll.ID != "" {
			candidates = append(candidates, filepath.Join(m.opts.SourcePath, "storage", coll.ID)) // newer PB layout
		}
		for _, candidate := range candidates {
			info, err := os.Stat(candidate)
			if err == nil && info.IsDir() {
				collectionPath = candidate
				break
			}
		}
		if collectionPath == "" {
			if m.verbose {
				fmt.Fprintf(m.output, "  %s: no files (skipping)\n", coll.Name)
			}
			continue
		}

		// Create bucket directory
		bucketPath := filepath.Join(storagePath, coll.Name)
		if err := os.MkdirAll(bucketPath, 0755); err != nil {
			return fmt.Errorf("failed to create bucket %s: %w", coll.Name, err)
		}

		// Count files first
		fileCount := 0
		filepath.Walk(collectionPath, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && !strings.HasSuffix(info.Name(), ".attrs") {
				fileCount++
			}
			return nil
		})

		if fileCount == 0 {
			if m.verbose {
				fmt.Fprintf(m.output, "  %s: 0 files\n", coll.Name)
			}
			continue
		}

		// Copy files
		copied := 0
		err := filepath.Walk(collectionPath, func(sourcePath string, info os.FileInfo, err error) error {
			if err != nil {
				if m.verbose {
					fmt.Fprintf(m.output, "    Warning: failed to access %s: %v\n", sourcePath, err)
				}
				errorCount++
				return nil // Continue walking
			}

			if info.IsDir() {
				return nil // Skip directories
			}
			if strings.HasSuffix(info.Name(), ".attrs") {
				return nil // Skip PocketBase metadata files
			}

			// Get relative path from collection directory
			relPath, err := filepath.Rel(collectionPath, sourcePath)
			if err != nil {
				if m.verbose {
					fmt.Fprintf(m.output, "    Warning: failed to get relative path for %s: %v\n", sourcePath, err)
				}
				errorCount++
				return nil
			}

			// Destination path in AYB storage
			destPath := filepath.Join(bucketPath, relPath)

			// Create destination directory
			destDir := filepath.Dir(destPath)
			if err := os.MkdirAll(destDir, 0755); err != nil {
				if m.verbose {
					fmt.Fprintf(m.output, "    Warning: failed to create directory %s: %v\n", destDir, err)
				}
				errorCount++
				return nil
			}

			// Copy file
			bytes, err := copyFile(sourcePath, destPath)
			if err != nil {
				if m.verbose {
					fmt.Fprintf(m.output, "    Warning: failed to copy %s: %v\n", relPath, err)
				}
				errorCount++
				return nil
			}

			copied++
			totalBytes += bytes
			m.stats.Files++

			return nil
		})

		if err != nil {
			return fmt.Errorf("failed to walk collection storage %s: %w", coll.Name, err)
		}

		totalFiles += copied
		fmt.Fprintf(m.output, "  %s: %d files copied\n", coll.Name, copied)
	}

	if errorCount > 0 {
		fmt.Fprintf(m.output, "  Warnings: %d files failed to copy\n", errorCount)
	}

	fmt.Fprintln(m.output, "")
	return nil
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
