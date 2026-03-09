//go:build integration

package schema_test

import (
	"context"
	"sync"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestCacheHolderLoad(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestSchema(t, ctx)

	ch := schema.NewCacheHolder(sharedPG.Pool, testutil.DiscardLogger())

	// Before load, Get() returns nil.
	testutil.Nil(t, ch.Get())

	// Load should succeed.
	err := ch.Load(ctx)
	testutil.NoError(t, err)

	// After load, Get() returns a populated cache.
	sc := ch.Get()
	testutil.NotNil(t, sc)
	testutil.True(t, len(sc.Tables) >= 4, "expected tables in cache")
}

func TestCacheHolderReload(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestSchema(t, ctx)

	ch := schema.NewCacheHolder(sharedPG.Pool, testutil.DiscardLogger())
	err := ch.Load(ctx)
	testutil.NoError(t, err)

	sc1 := ch.Get()
	builtAt1 := sc1.BuiltAt

	// Reload should produce a new cache with a different builtAt.
	err = ch.Reload(ctx)
	testutil.NoError(t, err)

	sc2 := ch.Get()
	testutil.NotNil(t, sc2)
	testutil.True(t, sc2.BuiltAt.After(builtAt1), "reloaded cache should have a later builtAt")
}

func TestCacheHolderReady(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestSchema(t, ctx)

	ch := schema.NewCacheHolder(sharedPG.Pool, testutil.DiscardLogger())

	// Ready should not be signaled yet.
	select {
	case <-ch.Ready():
		t.Fatal("ready should not be signaled before Load()")
	default:
	}

	// Load.
	err := ch.Load(ctx)
	testutil.NoError(t, err)

	// Ready should be signaled now.
	select {
	case <-ch.Ready():
		// OK
	default:
		t.Fatal("ready should be signaled after Load()")
	}
}

func TestCacheHolderConcurrentGet(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestSchema(t, ctx)

	ch := schema.NewCacheHolder(sharedPG.Pool, testutil.DiscardLogger())
	err := ch.Load(ctx)
	testutil.NoError(t, err)

	// Concurrent Get() calls should not panic or race.
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc := ch.Get()
			if sc == nil {
				t.Error("expected non-nil cache from concurrent Get()")
			}
		}()
	}
	wg.Wait()
}

func TestCacheHolderConcurrentReloadAndGet(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestSchema(t, ctx)

	ch := schema.NewCacheHolder(sharedPG.Pool, testutil.DiscardLogger())
	err := ch.Load(ctx)
	testutil.NoError(t, err)

	// Concurrent Get() + Reload() should not panic or race.
	var wg sync.WaitGroup

	// Readers.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc := ch.Get()
			if sc == nil {
				t.Error("expected non-nil cache from concurrent Get()")
			}
		}()
	}

	// Writers (reloaders).
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := ch.Reload(ctx); err != nil {
				t.Errorf("concurrent Reload() failed: %v", err)
			}
		}()
	}

	wg.Wait()

	// After all concurrent operations, cache should still be valid.
	sc := ch.Get()
	testutil.NotNil(t, sc)
	testutil.True(t, len(sc.Tables) >= 4, "expected tables in cache after concurrent reload")
}
