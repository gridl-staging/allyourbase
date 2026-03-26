package ui

import (
	"errors"
	"io/fs"
	"path"
	"strings"
	"testing"
)

func TestEmbeddedDistIncludesFunctionLogSelectors(t *testing.T) {
	jsBundle, err := readEmbeddedMainJS()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			t.Skip("embedded UI asset bundle not found; build ui/dist assets to validate selector markers")
		}
		t.Fatalf("read embedded UI bundle: %v", err)
	}

	for _, marker := range []string{
		"log-row-",
		"log-method-",
		"log-path-",
	} {
		if !strings.Contains(jsBundle, marker) {
			t.Fatalf("embedded UI bundle missing %q; rebuild ui/dist before go build", marker)
		}
	}
}

func TestEmbeddedDistIncludesOIDCProviderSelectors(t *testing.T) {
	jsBundle, err := readEmbeddedMainJS()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			t.Skip("embedded UI asset bundle not found; build ui/dist assets to validate selector markers")
		}
		t.Fatalf("read embedded UI bundle: %v", err)
	}

	for _, marker := range []string{
		"provider-delete-",
		"provider-form-issuer-url",
		"provider-form-display-name",
		"provider-form-scopes",
	} {
		if !strings.Contains(jsBundle, marker) {
			t.Fatalf("embedded UI bundle missing %q; rebuild ui/dist before go build", marker)
		}
	}
}

func readEmbeddedMainJS() (string, error) {
	entries, err := fs.ReadDir(DistDirFS, "assets")
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "index-") || !strings.HasSuffix(name, ".js") {
			continue
		}
		raw, readErr := fs.ReadFile(DistDirFS, path.Join("assets", name))
		if readErr != nil {
			return "", readErr
		}
		return string(raw), nil
	}

	return "", fs.ErrNotExist
}
