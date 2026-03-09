//go:build integration

package schema_test

import (
	"context"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestWatcherEnsureTriggersAndListen(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	// Create a table so schema introspection has something to find.
	_, err := sharedPG.Pool.Exec(ctx, `CREATE TABLE watcher_test (id SERIAL PRIMARY KEY, name TEXT)`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	watcher := schema.NewWatcher(ch, sharedPG.Pool, sharedPG.ConnString, logger)

	// Start watcher in background — it installs triggers, loads schema, then listens.
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- watcher.Start(watchCtx)
	}()

	// Wait for the cache to be ready (initial load).
	select {
	case <-ch.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for initial schema load")
	}

	// Verify initial schema was loaded correctly.
	sc := ch.Get()
	testutil.NotNil(t, sc)
	found := false
	for _, tbl := range sc.Tables {
		if tbl.Name == "watcher_test" {
			found = true
			break
		}
	}
	testutil.True(t, found, "watcher_test table should be in schema cache after initial load")

	// Perform a DDL change — the watcher should detect it via NOTIFY and reload.
	_, err = sharedPG.Pool.Exec(ctx, `CREATE TABLE watcher_new_table (id SERIAL PRIMARY KEY, val INT)`)
	testutil.NoError(t, err)

	// Wait for the schema cache to include the new table (debounce + reload).
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		sc = ch.Get()
		if sc != nil {
			for _, tbl := range sc.Tables {
				if tbl.Name == "watcher_new_table" {
					// Success — the watcher detected the DDL change.
					cancel()
					return
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("watcher did not detect new table within 5 seconds")
}

func TestWatcherDropTableDetected(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE TABLE keep_me (id SERIAL PRIMARY KEY);
		CREATE TABLE drop_me (id SERIAL PRIMARY KEY);
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	watcher := schema.NewWatcher(ch, sharedPG.Pool, sharedPG.ConnString, logger)

	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go watcher.Start(watchCtx)

	select {
	case <-ch.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for initial schema load")
	}

	// Verify both tables are present.
	sc := ch.Get()
	tableNames := make(map[string]bool)
	for _, tbl := range sc.Tables {
		tableNames[tbl.Name] = true
	}
	testutil.True(t, tableNames["keep_me"], "keep_me should be in initial cache")
	testutil.True(t, tableNames["drop_me"], "drop_me should be in initial cache")

	// Drop the table.
	_, err = sharedPG.Pool.Exec(ctx, `DROP TABLE drop_me`)
	testutil.NoError(t, err)

	// Wait for cache to reflect the drop.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		sc = ch.Get()
		if sc != nil {
			found := false
			for _, tbl := range sc.Tables {
				if tbl.Name == "drop_me" {
					found = true
					break
				}
			}
			if !found {
				// Success — drop was detected.
				cancel()
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("watcher did not detect dropped table within 5 seconds")
}

func TestWatcherAlterTableDetected(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	_, err := sharedPG.Pool.Exec(ctx, `CREATE TABLE alter_test (id SERIAL PRIMARY KEY, name TEXT)`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	watcher := schema.NewWatcher(ch, sharedPG.Pool, sharedPG.ConnString, logger)

	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go watcher.Start(watchCtx)

	select {
	case <-ch.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for initial schema load")
	}

	// Verify initial column count.
	sc := ch.Get()
	var initialCols int
	for _, tbl := range sc.Tables {
		if tbl.Name == "alter_test" {
			initialCols = len(tbl.Columns)
			break
		}
	}
	testutil.Equal(t, 2, initialCols) // id + name

	// Add a column.
	_, err = sharedPG.Pool.Exec(ctx, `ALTER TABLE alter_test ADD COLUMN email TEXT`)
	testutil.NoError(t, err)

	// Wait for the cache to reflect the added column.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		sc = ch.Get()
		if sc != nil {
			for _, tbl := range sc.Tables {
				if tbl.Name == "alter_test" && len(tbl.Columns) == 3 {
					// Success — ALTER TABLE was detected.
					cancel()
					return
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("watcher did not detect ALTER TABLE within 5 seconds")
}
