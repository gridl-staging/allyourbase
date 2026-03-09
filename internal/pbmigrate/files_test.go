package pbmigrate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestGetCollectionsWithFiles(t *testing.T) {
	t.Parallel()
	t.Run("no file fields", func(t *testing.T) {
		t.Parallel()
		collections := []PBCollection{
			{
				Name:   "posts",
				Type:   "base",
				System: false,
				Schema: []PBField{
					{Name: "title", Type: "text"},
					{Name: "body", Type: "editor"},
				},
			},
		}

		result := getCollectionsWithFiles(collections)
		testutil.Equal(t, 0, len(result))
	})

	t.Run("single file field", func(t *testing.T) {
		t.Parallel()
		collections := []PBCollection{
			{
				Name:   "posts",
				Type:   "base",
				System: false,
				Schema: []PBField{
					{Name: "title", Type: "text"},
					{Name: "image", Type: "file"},
				},
			},
		}

		result := getCollectionsWithFiles(collections)
		testutil.Equal(t, 1, len(result))
		testutil.Equal(t, "posts", result[0].Name)
	})

	t.Run("multiple collections with files", func(t *testing.T) {
		t.Parallel()
		collections := []PBCollection{
			{
				Name:   "posts",
				Type:   "base",
				System: false,
				Schema: []PBField{
					{Name: "title", Type: "text"},
					{Name: "image", Type: "file"},
				},
			},
			{
				Name:   "users",
				Type:   "auth",
				System: false,
				Schema: []PBField{
					{Name: "email", Type: "email"},
					{Name: "avatar", Type: "file"},
				},
			},
			{
				Name:   "comments",
				Type:   "base",
				System: false,
				Schema: []PBField{
					{Name: "text", Type: "text"},
				},
			},
		}

		result := getCollectionsWithFiles(collections)
		testutil.Equal(t, 2, len(result))
		testutil.Equal(t, "posts", result[0].Name)
		testutil.Equal(t, "users", result[1].Name)
	})

	t.Run("skip system collections", func(t *testing.T) {
		t.Parallel()
		collections := []PBCollection{
			{
				Name:   "_internal",
				Type:   "base",
				System: true,
				Schema: []PBField{
					{Name: "data", Type: "file"},
				},
			},
			{
				Name:   "posts",
				Type:   "base",
				System: false,
				Schema: []PBField{
					{Name: "image", Type: "file"},
				},
			},
		}

		result := getCollectionsWithFiles(collections)
		testutil.Equal(t, 1, len(result))
		testutil.Equal(t, "posts", result[0].Name)
	})

	t.Run("skip view collections", func(t *testing.T) {
		t.Parallel()
		collections := []PBCollection{
			{
				Name:   "stats_view",
				Type:   "view",
				System: false,
				Schema: []PBField{
					{Name: "count", Type: "number"},
				},
			},
			{
				Name:   "posts",
				Type:   "base",
				System: false,
				Schema: []PBField{
					{Name: "image", Type: "file"},
				},
			},
		}

		result := getCollectionsWithFiles(collections)
		testutil.Equal(t, 1, len(result))
		testutil.Equal(t, "posts", result[0].Name)
	})

	t.Run("multiple file fields in one collection", func(t *testing.T) {
		t.Parallel()
		collections := []PBCollection{
			{
				Name:   "posts",
				Type:   "base",
				System: false,
				Schema: []PBField{
					{Name: "title", Type: "text"},
					{Name: "image", Type: "file"},
					{Name: "attachments", Type: "file", Options: map[string]interface{}{"maxSelect": 5.0}},
				},
			},
		}

		result := getCollectionsWithFiles(collections)
		testutil.Equal(t, 1, len(result))
		testutil.Equal(t, "posts", result[0].Name)
	})
}

func TestCopyFile(t *testing.T) {
	t.Parallel()
	t.Run("copy simple file", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		// Create source file
		srcPath := filepath.Join(tmpDir, "source.txt")
		content := []byte("Hello, World!")
		err := os.WriteFile(srcPath, content, 0644)
		testutil.NoError(t, err)

		// Copy to destination
		dstPath := filepath.Join(tmpDir, "dest.txt")
		bytes, err := copyFile(srcPath, dstPath)
		testutil.NoError(t, err)
		testutil.Equal(t, int64(len(content)), bytes)

		// Verify content
		copied, err := os.ReadFile(dstPath)
		testutil.NoError(t, err)
		testutil.Equal(t, string(content), string(copied))
	})

	t.Run("copy large file", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		// Create source file with 1MB of data
		srcPath := filepath.Join(tmpDir, "large.bin")
		content := make([]byte, 1024*1024) // 1MB
		for i := range content {
			content[i] = byte(i % 256)
		}
		err := os.WriteFile(srcPath, content, 0644)
		testutil.NoError(t, err)

		// Copy to destination
		dstPath := filepath.Join(tmpDir, "large_copy.bin")
		bytes, err := copyFile(srcPath, dstPath)
		testutil.NoError(t, err)
		testutil.Equal(t, int64(len(content)), bytes)

		// Verify size
		info, err := os.Stat(dstPath)
		testutil.NoError(t, err)
		testutil.Equal(t, int64(len(content)), info.Size())
	})

	t.Run("missing source file", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		srcPath := filepath.Join(tmpDir, "missing.txt")
		dstPath := filepath.Join(tmpDir, "dest.txt")

		_, err := copyFile(srcPath, dstPath)
		testutil.ErrorContains(t, err, "no such file")
	})

	t.Run("create destination directory", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		// Create source file
		srcPath := filepath.Join(tmpDir, "source.txt")
		content := []byte("test")
		err := os.WriteFile(srcPath, content, 0644)
		testutil.NoError(t, err)

		// Copy to nested destination (directory doesn't exist yet)
		// Note: copyFile doesn't create directories, that's done by the caller
		dstPath := filepath.Join(tmpDir, "subdir", "dest.txt")

		// Create directory first
		err = os.MkdirAll(filepath.Dir(dstPath), 0755)
		testutil.NoError(t, err)

		bytes, err := copyFile(srcPath, dstPath)
		testutil.NoError(t, err)
		testutil.Equal(t, int64(len(content)), bytes)
	})

	t.Run("empty file", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		// Create empty source file
		srcPath := filepath.Join(tmpDir, "empty.txt")
		err := os.WriteFile(srcPath, []byte{}, 0644)
		testutil.NoError(t, err)

		// Copy to destination
		dstPath := filepath.Join(tmpDir, "empty_copy.txt")
		bytes, err := copyFile(srcPath, dstPath)
		testutil.NoError(t, err)
		testutil.Equal(t, int64(0), bytes)

		// Verify it exists
		info, err := os.Stat(dstPath)
		testutil.NoError(t, err)
		testutil.Equal(t, int64(0), info.Size())
	})

	t.Run("overwrite existing file", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		// Create source file
		srcPath := filepath.Join(tmpDir, "source.txt")
		newContent := []byte("new content")
		err := os.WriteFile(srcPath, newContent, 0644)
		testutil.NoError(t, err)

		// Create existing destination file
		dstPath := filepath.Join(tmpDir, "dest.txt")
		err = os.WriteFile(dstPath, []byte("old content"), 0644)
		testutil.NoError(t, err)

		// Copy (should overwrite)
		bytes, err := copyFile(srcPath, dstPath)
		testutil.NoError(t, err)
		testutil.Equal(t, int64(len(newContent)), bytes)

		// Verify content
		copied, err := os.ReadFile(dstPath)
		testutil.NoError(t, err)
		testutil.Equal(t, string(newContent), string(copied))
	})

	t.Run("binary file", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		// Create binary source file
		srcPath := filepath.Join(tmpDir, "image.bin")
		content := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46} // Fake JPEG header
		err := os.WriteFile(srcPath, content, 0644)
		testutil.NoError(t, err)

		// Copy to destination
		dstPath := filepath.Join(tmpDir, "image_copy.bin")
		bytes, err := copyFile(srcPath, dstPath)
		testutil.NoError(t, err)
		testutil.Equal(t, int64(len(content)), bytes)

		// Verify binary content
		copied, err := os.ReadFile(dstPath)
		testutil.NoError(t, err)
		testutil.Equal(t, len(content), len(copied))
		for i := range content {
			testutil.Equal(t, content[i], copied[i])
		}
	})
}

func TestMigrateFiles_NoStorageDirectory(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create pb_data without storage directory
	pbDataPath := filepath.Join(tmpDir, "pb_data")
	err := os.MkdirAll(pbDataPath, 0755)
	testutil.NoError(t, err)

	// Create migrator
	opts := MigrationOptions{
		SourcePath:  pbDataPath,
		DatabaseURL: "postgres://test",
		Verbose:     false,
	}

	migrator := &Migrator{
		opts:    opts,
		output:  os.Stdout,
		verbose: false,
	}

	// Run migration (should not error)
	err = migrator.migrateFiles(context.Background(), []PBCollection{})
	testutil.NoError(t, err)
}

func TestMigrateFiles_NoCollectionsWithFiles(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create pb_data with storage directory
	pbDataPath := filepath.Join(tmpDir, "pb_data")
	storagePath := filepath.Join(pbDataPath, "storage")
	err := os.MkdirAll(storagePath, 0755)
	testutil.NoError(t, err)

	collections := []PBCollection{
		{
			Name:   "posts",
			Type:   "base",
			System: false,
			Schema: []PBField{
				{Name: "title", Type: "text"},
			},
		},
	}

	opts := MigrationOptions{
		SourcePath:  pbDataPath,
		DatabaseURL: "postgres://test",
		Verbose:     false,
	}

	migrator := &Migrator{
		opts:    opts,
		output:  os.Stdout,
		verbose: false,
	}

	// Run migration (should not error)
	err = migrator.migrateFiles(context.Background(), collections)
	testutil.NoError(t, err)
}
