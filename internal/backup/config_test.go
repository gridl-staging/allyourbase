package backup

import "testing"

func TestConfigValidateDisabled(t *testing.T) {
	c := Config{}
	if err := c.Validate(); err != nil {
		t.Errorf("disabled config should be valid, got: %v", err)
	}
}

func TestConfigValidateEnabledMissingBucket(t *testing.T) {
	c := Config{Enabled: true, Region: "us-east-1"}
	if err := c.Validate(); err == nil {
		t.Error("expected error for missing bucket")
	}
}

func TestConfigValidateEnabledMissingRegion(t *testing.T) {
	c := Config{Enabled: true, Bucket: "my-bucket"}
	if err := c.Validate(); err == nil {
		t.Error("expected error for missing region")
	}
}

func TestConfigValidateNegativeRetentionCount(t *testing.T) {
	c := Config{Enabled: true, Bucket: "b", Region: "r", RetentionCount: -1}
	if err := c.Validate(); err == nil {
		t.Error("expected error for negative retention_count")
	}
}

func TestConfigValidateNegativeRetentionDays(t *testing.T) {
	c := Config{Enabled: true, Bucket: "b", Region: "r", RetentionDays: -1}
	if err := c.Validate(); err == nil {
		t.Error("expected error for negative retention_days")
	}
}

func TestConfigValidateInvalidEncryption(t *testing.T) {
	c := Config{Enabled: true, Bucket: "b", Region: "r", RetentionCount: 7, Encryption: "bad"}
	if err := c.Validate(); err == nil {
		t.Error("expected error for invalid encryption value")
	}
}

func TestConfigValidateValidEncryptionValues(t *testing.T) {
	for _, enc := range []string{"", "AES256", "aws:kms"} {
		c := Config{Enabled: true, Bucket: "b", Region: "r", RetentionCount: 7, Encryption: enc}
		if err := c.Validate(); err != nil {
			t.Errorf("encryption=%q: unexpected error: %v", enc, err)
		}
	}
}

func TestConfigDefault(t *testing.T) {
	d := Default()
	if d.Enabled {
		t.Error("default config should not be enabled")
	}
	if d.Region == "" {
		t.Error("default region should be set")
	}
	if d.Prefix == "" {
		t.Error("default prefix should be set")
	}
	if d.Schedule == "" {
		t.Error("default schedule should be set")
	}
}

// --- PITRConfig validation ---

func TestPITRConfigValidateDisabled(t *testing.T) {
	p := PITRConfig{}
	if err := p.Validate(); err != nil {
		t.Errorf("disabled PITR config should be valid, got: %v", err)
	}
}

func TestPITRConfigValidateEnabledRequiresArchiveBucket(t *testing.T) {
	p := PITRConfig{Enabled: true, RPOMinutes: 5, WALRetentionDays: 14}
	if err := p.Validate(); err == nil {
		t.Error("expected error for missing archive_bucket")
	}
}

func TestPITRConfigValidateRPOMustBePositive(t *testing.T) {
	p := PITRConfig{Enabled: true, ArchiveBucket: "pitr-bucket", WALRetentionDays: 14, RPOMinutes: 0}
	if err := p.Validate(); err == nil {
		t.Error("expected error for rpo_minutes = 0")
	}

	p.RPOMinutes = -1
	if err := p.Validate(); err == nil {
		t.Error("expected error for rpo_minutes < 0")
	}
}

func TestPITRConfigValidateWALRetentionMustBeAtLeastOneDay(t *testing.T) {
	p := PITRConfig{Enabled: true, ArchiveBucket: "pitr-bucket", RPOMinutes: 5, WALRetentionDays: 0}
	if err := p.Validate(); err == nil {
		t.Error("expected error for wal_retention_days = 0")
	}
}

func TestPITRConfigValidateValid(t *testing.T) {
	p := PITRConfig{
		Enabled:                 true,
		ArchiveBucket:           "pitr-bucket",
		RPOMinutes:              5,
		WALRetentionDays:        14,
		BaseBackupRetentionDays: 1,
		BaseBackupSchedule:      "0 */6 * * *",
		RetentionSchedule:       "0 4 * * *",
	}
	if err := p.Validate(); err != nil {
		t.Errorf("valid PITR config should pass validation, got: %v", err)
	}
}

func TestPITRConfigDefaultValues(t *testing.T) {
	d := DefaultPITR()
	if d.RPOMinutes != 5 {
		t.Errorf("default RPOMinutes = %d; want 5", d.RPOMinutes)
	}
	if d.WALRetentionDays != 14 {
		t.Errorf("default WALRetentionDays = %d; want 14", d.WALRetentionDays)
	}
	if !d.ShadowMode {
		t.Error("default ShadowMode should be true")
	}
	if d.BaseBackupSchedule == "" {
		t.Error("default BaseBackupSchedule should be set")
	}
}

func TestConfigValidateCallsPITRValidate(t *testing.T) {
	// A valid backup config with an invalid PITR config should fail.
	c := Config{
		Enabled: true,
		Bucket:  "b",
		Region:  "r",
		PITR: PITRConfig{
			Enabled:          true,
			ArchiveBucket:    "", // missing — invalid
			RPOMinutes:       5,
			WALRetentionDays: 14,
		},
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error when PITR config is invalid")
	}
}
