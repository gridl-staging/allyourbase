package pbmigrate

import (
	"context"
	"fmt"
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

	t.Run("reject symlink source", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		targetPath := filepath.Join(tmpDir, "target.txt")
		err := os.WriteFile(targetPath, []byte("target"), 0644)
		testutil.NoError(t, err)

		linkPath := filepath.Join(tmpDir, "source-link.txt")
		if err := os.Symlink(targetPath, linkPath); err != nil {
			t.Skipf("symlink not supported in this environment: %v", err)
		}

		dstPath := filepath.Join(tmpDir, "dest.txt")
		_, err = copyFile(linkPath, dstPath)
		testutil.ErrorContains(t, err, "refusing to copy symlink source")
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

func TestMigrateFiles_SkipsSymlinkSources(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	pbDataPath := filepath.Join(tmpDir, "pb_data")
	collPath := filepath.Join(pbDataPath, "storage", "posts")
	err := os.MkdirAll(collPath, 0755)
	testutil.NoError(t, err)

	// Regular file that should be copied.
	err = os.WriteFile(filepath.Join(collPath, "real.jpg"), []byte("real"), 0644)
	testutil.NoError(t, err)

	// External file plus symlink inside collection; symlink must be skipped.
	outsidePath := filepath.Join(tmpDir, "outside-secret.txt")
	err = os.WriteFile(outsidePath, []byte("secret"), 0644)
	testutil.NoError(t, err)

	linkPath := filepath.Join(collPath, "secret-link.txt")
	if err := os.Symlink(outsidePath, linkPath); err != nil {
		t.Skipf("symlink not supported in this environment: %v", err)
	}

	destPath := filepath.Join(tmpDir, "ayb_storage")
	migrator := &Migrator{
		opts: MigrationOptions{
			SourcePath:  pbDataPath,
			DatabaseURL: "postgres://test",
			StoragePath: destPath,
		},
		output: os.Stdout,
		stats:  MigrationStats{},
	}

	collections := []PBCollection{
		{
			Name:   "posts",
			Type:   "base",
			System: false,
			Schema: []PBField{
				{Name: "image", Type: "file"},
			},
		},
	}

	err = migrator.migrateFiles(context.Background(), collections)
	testutil.NoError(t, err)

	copied, err := os.ReadFile(filepath.Join(destPath, "posts", "real.jpg"))
	testutil.NoError(t, err)
	testutil.Equal(t, "real", string(copied))

	_, err = os.Stat(filepath.Join(destPath, "posts", "secret-link.txt"))
	testutil.ErrorContains(t, err, "no such file")

	testutil.Equal(t, 1, migrator.stats.Files)
}

func TestMigrateFiles_UsesCollectionIDDirectoryWhenNameDirectoryMissing(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	pbDataPath := filepath.Join(tmpDir, "pb_data")
	idPath := filepath.Join(pbDataPath, "storage", "collection-id-1")
	err := os.MkdirAll(idPath, 0755)
	testutil.NoError(t, err)
	err = os.WriteFile(filepath.Join(idPath, "avatar.png"), []byte("id-dir-file"), 0644)
	testutil.NoError(t, err)

	destPath := filepath.Join(tmpDir, "ayb_storage")
	migrator := &Migrator{
		opts: MigrationOptions{
			SourcePath:  pbDataPath,
			DatabaseURL: "postgres://test",
			StoragePath: destPath,
		},
		output: os.Stdout,
		stats:  MigrationStats{},
	}

	collections := []PBCollection{
		{
			ID:     "collection-id-1",
			Name:   "profiles",
			Type:   "base",
			System: false,
			Schema: []PBField{
				{Name: "avatar", Type: "file"},
			},
		},
	}

	err = migrator.migrateFiles(context.Background(), collections)
	testutil.NoError(t, err)

	copied, err := os.ReadFile(filepath.Join(destPath, "profiles", "avatar.png"))
	testutil.NoError(t, err)
	testutil.Equal(t, "id-dir-file", string(copied))
	testutil.Equal(t, 1, migrator.stats.Files)
}

func TestMigrateFiles_RejectsTraversalCollectionName(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	pbDataPath := filepath.Join(tmpDir, "pb_data")
	storagePath := filepath.Join(pbDataPath, "storage")
	err := os.MkdirAll(storagePath, 0o755)
	testutil.NoError(t, err)

	destPath := filepath.Join(tmpDir, "ayb_storage")
	migrator := &Migrator{
		opts: MigrationOptions{
			SourcePath:  pbDataPath,
			DatabaseURL: "postgres://test",
			StoragePath: destPath,
		},
		output: os.Stdout,
	}

	err = migrator.migrateFiles(context.Background(), []PBCollection{
		{
			Name:   "../escape",
			Type:   "base",
			System: false,
			Schema: []PBField{{Name: "file", Type: "file"}},
		},
	})
	testutil.ErrorContains(t, err, `invalid collection name "../escape"`)
}

func TestMigrateFiles_FailedCopiesRecordExactPaths(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	pbDataPath := filepath.Join(tmpDir, "pb_data")
	collPath := filepath.Join(pbDataPath, "storage", "docs")
	err := os.MkdirAll(collPath, 0755)
	testutil.NoError(t, err)

	// Create two good files and one that will fail (unreadable).
	err = os.WriteFile(filepath.Join(collPath, "good1.txt"), []byte("ok1"), 0644)
	testutil.NoError(t, err)
	err = os.WriteFile(filepath.Join(collPath, "good2.txt"), []byte("ok2"), 0644)
	testutil.NoError(t, err)

	badFile := filepath.Join(collPath, "bad.txt")
	err = os.WriteFile(badFile, []byte("secret"), 0000) // unreadable
	testutil.NoError(t, err)

	destPath := filepath.Join(tmpDir, "ayb_storage")
	migrator := &Migrator{
		opts: MigrationOptions{
			SourcePath:  pbDataPath,
			DatabaseURL: "postgres://test",
			StoragePath: destPath,
		},
		output:  os.Stdout,
		verbose: true,
		stats:   MigrationStats{},
	}

	collections := []PBCollection{
		{
			Name:   "docs",
			Type:   "base",
			Schema: []PBField{{Name: "attachment", Type: "file"}},
		},
	}

	err = migrator.migrateFiles(context.Background(), collections)
	testutil.NoError(t, err) // migration continues past failures

	// The two good files must be copied.
	testutil.Equal(t, 2, migrator.stats.Files)

	// The failed file must be recorded with its exact relative path.
	testutil.SliceLen(t, migrator.stats.FailedFiles, 1)
	testutil.Equal(t, "docs/bad.txt", migrator.stats.FailedFiles[0])
}

func TestMigrateFiles_MultipleCollectionFailuresAccumulate(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	pbDataPath := filepath.Join(tmpDir, "pb_data")

	// Collection "alpha" — one good, one unreadable.
	alphaPath := filepath.Join(pbDataPath, "storage", "alpha")
	err := os.MkdirAll(alphaPath, 0755)
	testutil.NoError(t, err)
	err = os.WriteFile(filepath.Join(alphaPath, "ok.txt"), []byte("a"), 0644)
	testutil.NoError(t, err)
	err = os.WriteFile(filepath.Join(alphaPath, "fail.txt"), []byte("x"), 0000)
	testutil.NoError(t, err)

	// Collection "beta" — one unreadable.
	betaPath := filepath.Join(pbDataPath, "storage", "beta")
	err = os.MkdirAll(betaPath, 0755)
	testutil.NoError(t, err)
	err = os.WriteFile(filepath.Join(betaPath, "fail2.txt"), []byte("y"), 0000)
	testutil.NoError(t, err)

	destPath := filepath.Join(tmpDir, "ayb_storage")
	migrator := &Migrator{
		opts: MigrationOptions{
			SourcePath:  pbDataPath,
			DatabaseURL: "postgres://test",
			StoragePath: destPath,
		},
		output:  os.Stdout,
		verbose: true,
		stats:   MigrationStats{},
	}

	collections := []PBCollection{
		{Name: "alpha", Type: "base", Schema: []PBField{{Name: "f", Type: "file"}}},
		{Name: "beta", Type: "base", Schema: []PBField{{Name: "f", Type: "file"}}},
	}

	err = migrator.migrateFiles(context.Background(), collections)
	testutil.NoError(t, err)

	// One good file copied.
	testutil.Equal(t, 1, migrator.stats.Files)

	// Both failures recorded with collection-qualified paths.
	testutil.SliceLen(t, migrator.stats.FailedFiles, 2)

	// Build a set for order-independent check.
	failSet := make(map[string]bool, len(migrator.stats.FailedFiles))
	for _, f := range migrator.stats.FailedFiles {
		failSet[f] = true
	}
	testutil.True(t, failSet["alpha/fail.txt"], "expected alpha/fail.txt in FailedFiles")
	testutil.True(t, failSet["beta/fail2.txt"], "expected beta/fail2.txt in FailedFiles")
}

func TestMigrateFiles_NoFailuresLeavesFailedFilesNil(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	pbDataPath := filepath.Join(tmpDir, "pb_data")
	collPath := filepath.Join(pbDataPath, "storage", "images")
	err := os.MkdirAll(collPath, 0755)
	testutil.NoError(t, err)
	err = os.WriteFile(filepath.Join(collPath, "photo.jpg"), []byte("img"), 0644)
	testutil.NoError(t, err)

	destPath := filepath.Join(tmpDir, "ayb_storage")
	migrator := &Migrator{
		opts: MigrationOptions{
			SourcePath:  pbDataPath,
			DatabaseURL: "postgres://test",
			StoragePath: destPath,
		},
		output: os.Stdout,
		stats:  MigrationStats{},
	}

	collections := []PBCollection{
		{Name: "images", Type: "base", Schema: []PBField{{Name: "photo", Type: "file"}}},
	}

	err = migrator.migrateFiles(context.Background(), collections)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, migrator.stats.Files)
	testutil.Nil(t, migrator.stats.FailedFiles)
}

func TestRecordFileCopyFailure_NormalizesToSlashSeparatedPaths(t *testing.T) {
	t.Parallel()

	migrator := &Migrator{output: os.Stdout}
	total := fileCopyTotals{}

	migrator.recordFileCopyFailure(&total, "docs", `nested\bad.txt`, "/tmp/source/nested/bad.txt", fmt.Errorf("boom"))

	testutil.SliceLen(t, total.failedFiles, 1)
	testutil.Equal(t, "docs/nested/bad.txt", total.failedFiles[0])
}

func TestMigrateFiles_RejectsTraversalCollectionID(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	pbDataPath := filepath.Join(tmpDir, "pb_data")
	storagePath := filepath.Join(pbDataPath, "storage")
	err := os.MkdirAll(storagePath, 0o755)
	testutil.NoError(t, err)

	destPath := filepath.Join(tmpDir, "ayb_storage")
	migrator := &Migrator{
		opts: MigrationOptions{
			SourcePath:  pbDataPath,
			DatabaseURL: "postgres://test",
			StoragePath: destPath,
		},
		output: os.Stdout,
	}

	err = migrator.migrateFiles(context.Background(), []PBCollection{
		{
			ID:     "../escape-id",
			Name:   "profiles",
			Type:   "base",
			System: false,
			Schema: []PBField{{Name: "file", Type: "file"}},
		},
	})
	testutil.ErrorContains(t, err, `invalid collection id "../escape-id"`)
}
