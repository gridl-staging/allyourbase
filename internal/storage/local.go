// Package storage LocalBackend provides file storage on the local filesystem with path traversal protection.
package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalBackend stores files on the local filesystem.
type LocalBackend struct {
	root string
}

// NewLocalBackend creates a local filesystem backend rooted at the given path.
func NewLocalBackend(root string) (*LocalBackend, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolving storage path: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("creating storage directory: %w", err)
	}
	return &LocalBackend{root: abs}, nil
}

// Put writes data from r to the named object in the given bucket, creating parent directories as needed. It returns the number of bytes written. Any partial file is removed if the write fails.
func (b *LocalBackend) Put(_ context.Context, bucket, name string, r io.Reader) (int64, error) {
	path, err := b.objectPath(bucket, name)
	if err != nil {
		return 0, err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("creating directory: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return 0, fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()

	n, err := io.Copy(f, r)
	if err != nil {
		os.Remove(path) // clean up partial file
		return 0, fmt.Errorf("writing file: %w", err)
	}

	return n, nil
}

func (b *LocalBackend) Get(_ context.Context, bucket, name string) (io.ReadCloser, error) {
	path, err := b.objectPath(bucket, name)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("opening file: %w", err)
	}
	return f, nil
}

func (b *LocalBackend) Delete(_ context.Context, bucket, name string) error {
	path, err := b.objectPath(bucket, name)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing file: %w", err)
	}
	return nil
}

func (b *LocalBackend) Exists(_ context.Context, bucket, name string) (bool, error) {
	path, err := b.objectPath(bucket, name)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat file: %w", err)
	}
	return true, nil
}

// objectPath returns the absolute filesystem path for the named object within the bucket, after validating the bucket and name and ensuring the path does not escape the storage root.
func (b *LocalBackend) objectPath(bucket, name string) (string, error) {
	if err := validateBucket(bucket); err != nil {
		return "", err
	}
	if err := validateName(name); err != nil {
		return "", err
	}
	target := filepath.Join(b.root, bucket, name)
	rel, err := filepath.Rel(b.root, target)
	if err != nil {
		return "", fmt.Errorf("resolving object path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: object path escapes storage root", ErrInvalidName)
	}
	return target, nil
}
