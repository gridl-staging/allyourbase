package codehealth

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const checkFileSizesScript = "scripts/check-file-sizes.sh"

func TestCheckFileSizesScriptFailsForUnallowlistedOversizedFile(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	tempRoot := t.TempDir()
	allowlistPath := filepath.Join(tempRoot, "allowlist-oversized.txt")

	writeLinesFile(t, filepath.Join(tempRoot, "internal", "oversized.go"), 501)
	writeTextFile(t, allowlistPath, "")

	cmd := exec.Command("bash", checkFileSizesScript)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"CHECK_FILE_SIZES_ROOT="+tempRoot,
		"CHECK_FILE_SIZES_ALLOWLIST="+allowlistPath,
	)

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected script failure, got success: %s", output)
	}
	if !strings.Contains(string(output), "internal/oversized.go:501") {
		t.Fatalf("expected output to include oversized file, got: %s", output)
	}
}

func TestCheckFileSizesScriptPassesForAllowlistedOversizedFile(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	tempRoot := t.TempDir()
	allowlistPath := filepath.Join(tempRoot, "allowlist-oversized.txt")

	writeLinesFile(t, filepath.Join(tempRoot, "internal", "oversized.go"), 501)
	writeTextFile(t, allowlistPath, "internal/oversized.go:501\n")

	cmd := exec.Command("bash", checkFileSizesScript)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"CHECK_FILE_SIZES_ROOT="+tempRoot,
		"CHECK_FILE_SIZES_ALLOWLIST="+allowlistPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected script success, got error: %v output=%s", err, output)
	}
}

func writeLinesFile(t *testing.T, path string, lineCount int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	lines := make([]string, lineCount)
	for i := range lines {
		lines[i] = "x"
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
}

func writeTextFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(workingDirectory, "..", ".."))
	if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err != nil {
		t.Fatalf("repo root not found from %s: %v", workingDirectory, err)
	}
	return repoRoot
}
