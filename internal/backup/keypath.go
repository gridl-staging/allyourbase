package backup

import (
	"fmt"
	"strings"
	"time"
)

// ObjectKey builds the canonical S3 object key for a logical backup.
// Format: {prefix}/{db}/{yyyy}/{mm}/{dd}/{timestamp}_{backup_id}.sql.gz
func ObjectKey(prefix, dbName, backupID string, t time.Time) string {
	return fmt.Sprintf("%s/%s/%s/%s_%s.sql.gz",
		strings.TrimRight(prefix, "/"),
		dbName,
		t.UTC().Format("2006/01/02"),
		t.UTC().Format("20060102T150405Z"),
		backupID,
	)
}

// joinPrefix returns prefix + "/" or "" when prefix is empty, ensuring
// paths never start with a leading slash.
func joinPrefix(prefix string) string {
	p := strings.TrimRight(prefix, "/")
	if p == "" {
		return ""
	}
	return p + "/"
}

// BaseBackupKey builds the S3 object key for a physical base backup.
// Format: [{prefix}/]projects/{projectID}/db/{databaseID}/base/{yyyy}/{mm}/{dd}/{timestamp}_{lsn}.tar.zst
func BaseBackupKey(prefix, projectID, databaseID, lsn string, t time.Time) string {
	return fmt.Sprintf("%sprojects/%s/db/%s/base/%s/%s_%s.tar.zst",
		joinPrefix(prefix),
		projectID,
		databaseID,
		t.UTC().Format("2006/01/02"),
		t.UTC().Format("20060102T150405Z"),
		lsn,
	)
}

// WALSegmentKey builds the S3 object key for an archived WAL segment.
// Format: [{prefix}/]projects/{projectID}/db/{databaseID}/wal/{timeline:08d}/{segmentName}
func WALSegmentKey(prefix, projectID, databaseID string, timeline int, segmentName string) string {
	return fmt.Sprintf("%sprojects/%s/db/%s/wal/%08d/%s",
		joinPrefix(prefix),
		projectID,
		databaseID,
		timeline,
		segmentName,
	)
}

// ManifestKey builds the S3 object key for a PITR backup manifest.
// Format: [{prefix}/]projects/{projectID}/db/{databaseID}/manifests/{timestamp}.json
func ManifestKey(prefix, projectID, databaseID string, t time.Time) string {
	return fmt.Sprintf("%sprojects/%s/db/%s/manifests/%s.json",
		joinPrefix(prefix),
		projectID,
		databaseID,
		t.UTC().Format("20060102T150405Z"),
	)
}
