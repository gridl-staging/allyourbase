package pbmigrate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/allyourbase/ayb/internal/migrate"
)

// Analyze inspects the PocketBase source without modifying the target database.
// It returns a report summarizing what the migration would do.
func Analyze(sourcePath string) (*migrate.AnalysisReport, error) {
	reader, err := NewReader(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("opening source: %w", err)
	}
	defer reader.Close()

	collections, err := reader.ReadCollections()
	if err != nil {
		return nil, fmt.Errorf("reading collections: %w", err)
	}

	report := &migrate.AnalysisReport{
		SourceType: "PocketBase",
	}

	// Source info: SQLite file size
	dataPath := filepath.Join(sourcePath, "data.db")
	if info, err := os.Stat(dataPath); err == nil {
		report.SourceInfo = fmt.Sprintf("SQLite %s", formatSize(info.Size()))
	}

	for _, coll := range collections {
		if coll.System {
			continue
		}

		switch coll.Type {
		case "auth":
			count, err := reader.CountRecords(coll.Name)
			if err != nil {
				report.Warnings = append(report.Warnings,
					fmt.Sprintf("could not count auth users in %s: %v", coll.Name, err))
				continue
			}
			report.AuthUsers += count

		case "view":
			report.Views++

		default:
			report.Tables++
			count, err := reader.CountRecords(coll.Name)
			if err != nil {
				report.Warnings = append(report.Warnings,
					fmt.Sprintf("could not count records in %s: %v", coll.Name, err))
				continue
			}
			report.Records += count
		}

		// Count RLS policies that would be generated
		policies := countPolicies(coll)
		report.RLSPolicies += policies
	}

	// Count storage files
	storagePath := filepath.Join(sourcePath, "storage")
	if _, err := os.Stat(storagePath); err == nil {
		fileCollections := getCollectionsWithFiles(collections)
		for _, coll := range fileCollections {
			collPath := filepath.Join(storagePath, coll.Name)
			if _, err := os.Stat(collPath); os.IsNotExist(err) {
				continue
			}
			filepath.Walk(collPath, func(path string, info os.FileInfo, err error) error {
				if err == nil && !info.IsDir() {
					report.Files++
					report.FileSizeBytes += info.Size()
				}
				return nil
			})
		}
	}

	return report, nil
}

// countPolicies returns how many RLS policies would be generated for a collection.
func countPolicies(coll PBCollection) int {
	if coll.System || coll.Type == "auth" || coll.Type == "view" {
		return 0
	}
	count := 0
	// ViewRule is intentionally excluded: both ListRule and ViewRule map to SELECT
	// in PostgreSQL, and GenerateRLSPolicies() uses only ListRule for the SELECT policy.
	rules := []*string{coll.ListRule, coll.CreateRule, coll.UpdateRule, coll.DeleteRule}
	for _, r := range rules {
		if r != nil {
			count++
		}
	}
	return count
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
