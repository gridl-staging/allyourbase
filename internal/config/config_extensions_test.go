package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddExtensionCreatesSection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "ayb.toml")

	if err := AddExtension(path, "pgvector"); err != nil {
		t.Fatalf("AddExtension: %v", err)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "pgvector") {
		t.Errorf("expected pgvector in config, got %s", data)
	}
}

func TestAddExtensionAppends(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "ayb.toml")
	os.WriteFile(path, []byte("[managed_pg]\nextensions = [\"pg_trgm\"]\n"), 0o600)

	if err := AddExtension(path, "pgvector"); err != nil {
		t.Fatalf("AddExtension: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	if !strings.Contains(content, "pgvector") {
		t.Errorf("expected pgvector added, got %s", content)
	}
	if !strings.Contains(content, "pg_trgm") {
		t.Errorf("expected pg_trgm preserved, got %s", content)
	}
}

func TestAddExtensionIdempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "ayb.toml")
	os.WriteFile(path, []byte("[managed_pg]\nextensions = [\"pgvector\"]\n"), 0o600)

	if err := AddExtension(path, "pgvector"); err != nil {
		t.Fatalf("AddExtension: %v", err)
	}

	data, _ := os.ReadFile(path)
	if strings.Count(string(data), "pgvector") != 1 {
		t.Errorf("expected exactly one pgvector, got %s", data)
	}
}

func TestRemoveExtension(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "ayb.toml")
	os.WriteFile(path, []byte("[managed_pg]\nextensions = [\"pgvector\", \"pg_trgm\"]\n"), 0o600)

	if err := RemoveExtension(path, "pgvector"); err != nil {
		t.Fatalf("RemoveExtension: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	if strings.Contains(content, "pgvector") {
		t.Errorf("expected pgvector removed, got %s", content)
	}
	if !strings.Contains(content, "pg_trgm") {
		t.Errorf("expected pg_trgm preserved, got %s", content)
	}
}

func TestRemoveExtensionIdempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "ayb.toml")
	os.WriteFile(path, []byte("[managed_pg]\nextensions = [\"pg_trgm\"]\n"), 0o600)

	if err := RemoveExtension(path, "pgvector"); err != nil {
		t.Fatalf("RemoveExtension: %v", err)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "pg_trgm") {
		t.Errorf("expected pg_trgm preserved, got %s", data)
	}
}

func TestRemoveExtensionNoFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.toml")

	if err := RemoveExtension(path, "pgvector"); err != nil {
		t.Fatalf("RemoveExtension on missing file: %v", err)
	}
}
