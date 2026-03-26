// Package backup This file defines backup configuration structures for full backups and point-in-time recovery, including Config for general backup settings and PITRConfig for WAL archiving.
package backup

import "fmt"

// Config mirrors config.BackupConfig for the backup package.
// The wiring layer (start.go) converts config.BackupConfig → backup.Config.
type Config struct {
	Enabled        bool
	Bucket         string
	Region         string
	Prefix         string
	Schedule       string
	RetentionCount int
	RetentionDays  int
	Encryption     string // "" | "AES256" | "aws:kms"
	Endpoint       string // custom endpoint for LocalStack / MinIO
	AccessKey      string
	SecretKey      string
	PITR           PITRConfig
}

// PITRConfig configures WAL archiving and point-in-time recovery.
// When Enabled is true, WAL segments are archived and physical base backups are taken.
// ShadowMode (default true) archives WAL but refuses restore cutover requests — set
// to false to allow PITR restores in production.
type PITRConfig struct {
	Enabled                  bool
	ArchiveBucket            string
	ArchivePrefix            string // optional namespace prefix; empty means paths start with projects/
	WALRetentionDays         int    // default 14
	BaseBackupRetentionDays  int    // default 35
	ComplianceSnapshotMonths int    // default 12
	EnvironmentClass         string // e.g. "prod", "staging"
	KMSKeyID                 string
	RetentionSchedule        string // cron expression, default "0 4 * * *" (daily 4 AM)
	RPOMinutes               int    // default 5; must be > 0
	StorageBudgetBytes       int64  // default 0 (unlimited)
	ShadowMode               bool   // default true
	BaseBackupSchedule       string // cron expression, default "0 3 * * *"
	VerifySchedule           string // cron expression, default "0 */6 * * *" (every 6 hours)
}

// DefaultPITR returns a PITRConfig with sensible defaults.
func DefaultPITR() PITRConfig {
	return PITRConfig{
		WALRetentionDays:         14,
		BaseBackupRetentionDays:  35,
		ComplianceSnapshotMonths: 12,
		RetentionSchedule:        "0 4 * * *",
		RPOMinutes:               5,
		StorageBudgetBytes:       0,
		ShadowMode:               true,
		BaseBackupSchedule:       "0 3 * * *",
		VerifySchedule:           "0 */6 * * *",
	}
}

// Validate checks that PITRConfig is usable. Disabled configs always pass.
func (p *PITRConfig) Validate() error {
	if !p.Enabled {
		return nil
	}
	if p.ArchiveBucket == "" {
		return fmt.Errorf("pitr: archive_bucket is required when enabled")
	}
	if p.RPOMinutes <= 0 {
		return fmt.Errorf("pitr: rpo_minutes must be > 0")
	}
	if p.WALRetentionDays < 1 {
		return fmt.Errorf("pitr: wal_retention_days must be >= 1")
	}
	if p.RetentionSchedule == "" {
		return fmt.Errorf("pitr: retention_schedule is required when enabled")
	}
	if p.BaseBackupRetentionDays < 1 {
		return fmt.Errorf("pitr: base_backup_retention_days must be >= 1")
	}
	if p.ComplianceSnapshotMonths < 0 {
		return fmt.Errorf("pitr: compliance_snapshot_months must be >= 0")
	}
	if p.StorageBudgetBytes < 0 {
		return fmt.Errorf("pitr: storage_budget_bytes must be >= 0")
	}
	if p.EnvironmentClass == "" {
		p.EnvironmentClass = "non-prod"
	}
	return nil
}

// Default returns a Config with sensible defaults.
func Default() Config {
	return Config{
		Region:   "us-east-1",
		Prefix:   "backups",
		Schedule: "0 2 * * *", // daily at 2 AM UTC
		PITR:     DefaultPITR(),
	}
}

// Validate checks that a Config is usable. Disabled configs always pass.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.Bucket == "" {
		return fmt.Errorf("backup: bucket is required when enabled")
	}
	if c.Region == "" {
		return fmt.Errorf("backup: region is required when enabled")
	}
	if c.RetentionCount < 0 {
		return fmt.Errorf("backup: retention_count must be >= 0")
	}
	if c.RetentionDays < 0 {
		return fmt.Errorf("backup: retention_days must be >= 0")
	}
	switch c.Encryption {
	case "", "AES256", "aws:kms":
		// valid
	default:
		return fmt.Errorf("backup: invalid encryption %q (must be \"\", \"AES256\", or \"aws:kms\")", c.Encryption)
	}
	if err := c.PITR.Validate(); err != nil {
		return err
	}
	return nil
}
