package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

func (b *LocalBackend) Put(_ context.Context, bucket, name string, r io.Reader) (int64, error) {
	dir := filepath.Join(b.root, bucket, filepath.Dir(name))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("creating directory: %w", err)
	}

	path := filepath.Join(b.root, bucket, name)
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
	path := filepath.Join(b.root, bucket, name)
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
	path := filepath.Join(b.root, bucket, name)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing file: %w", err)
	}
	return nil
}

func (b *LocalBackend) Exists(_ context.Context, bucket, name string) (bool, error) {
	path := filepath.Join(b.root, bucket, name)
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat file: %w", err)
	}
	return true, nil
}
