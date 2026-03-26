package graphql

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestLoaderPrimeAndLoadBatchesAllKeys(t *testing.T) {
	t.Parallel()

	batchCalls := 0
	captured := []interface{}{}
	loader := newLoader(
		func(ctx context.Context, keys []interface{}) (map[interface{}][]map[string]any, error) {
			batchCalls++
			captured = append(captured[:0], keys...)
			out := make(map[interface{}][]map[string]any, len(keys))
			for _, key := range keys {
				out[key] = []map[string]any{{"id": key}}
			}
			return out, nil
		},
		func(ctx context.Context, key interface{}) ([]map[string]any, error) {
			return []map[string]any{{"id": key}}, nil
		},
	)

	loader.Prime(1)
	loader.Prime(2)
	loader.Prime(3)

	rows, err := loader.Load(context.Background(), 2)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, batchCalls)
	testutil.Equal(t, 1, len(rows))
	testutil.Equal(t, 2, rows[0]["id"])

	sort.Slice(captured, func(i, j int) bool { return captured[i].(int) < captured[j].(int) })
	testutil.Equal(t, 3, len(captured))
	testutil.Equal(t, 1, captured[0])
	testutil.Equal(t, 2, captured[1])
	testutil.Equal(t, 3, captured[2])
}

func TestLoaderLoadCachesResults(t *testing.T) {
	t.Parallel()

	batchCalls := 0
	loader := newLoader(
		func(ctx context.Context, keys []interface{}) (map[interface{}][]map[string]any, error) {
			batchCalls++
			return map[interface{}][]map[string]any{
				1: {{"id": 1}},
			}, nil
		},
		func(ctx context.Context, key interface{}) ([]map[string]any, error) {
			return nil, nil
		},
	)
	loader.Prime(1)

	rows1, err := loader.Load(context.Background(), 1)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(rows1))

	rows2, err := loader.Load(context.Background(), 1)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(rows2))
	testutil.Equal(t, 1, batchCalls)
}

func TestLoaderLoadNilKeyReturnsNilWithoutBatch(t *testing.T) {
	t.Parallel()

	batchCalls := 0
	singleCalls := 0
	loader := newLoader(
		func(ctx context.Context, keys []interface{}) (map[interface{}][]map[string]any, error) {
			batchCalls++
			return map[interface{}][]map[string]any{}, nil
		},
		func(ctx context.Context, key interface{}) ([]map[string]any, error) {
			singleCalls++
			return nil, nil
		},
	)

	rows, err := loader.Load(context.Background(), nil)
	testutil.NoError(t, err)
	testutil.Nil(t, rows)
	testutil.Equal(t, 0, batchCalls)
	testutil.Equal(t, 0, singleCalls)
}

func TestLoaderCacheMissAfterBatchFallsBackToSingleFetch(t *testing.T) {
	t.Parallel()

	batchCalls := 0
	singleCalls := 0
	loader := newLoader(
		func(ctx context.Context, keys []interface{}) (map[interface{}][]map[string]any, error) {
			batchCalls++
			return map[interface{}][]map[string]any{
				1: {{"id": 1}},
				2: {{"id": 2}},
			}, nil
		},
		func(ctx context.Context, key interface{}) ([]map[string]any, error) {
			singleCalls++
			return []map[string]any{{"id": key}}, nil
		},
	)
	loader.Prime(1)
	loader.Prime(2)

	rows1, err := loader.Load(context.Background(), 1)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(rows1))
	testutil.Equal(t, 1, batchCalls)
	testutil.Equal(t, 0, singleCalls)

	rows3, err := loader.Load(context.Background(), 3)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(rows3))
	testutil.True(t, rows3[0]["id"] == 3 || rows3[0]["id"] == "3", "expected fallback row for key=3")
	testutil.Equal(t, 1, batchCalls)
	testutil.Equal(t, 1, singleCalls)
}

func TestDataloaderContextRoundTrip(t *testing.T) {
	t.Parallel()

	dl := NewDataloader(nil, nil)
	ctx := ctxWithDataloader(context.Background(), dl)
	got := dataloaderFromCtx(ctx)
	testutil.True(t, got == dl, "expected same dataloader instance")
}

func TestDataloaderGetLoaderStableByRelationship(t *testing.T) {
	t.Parallel()

	dl := NewDataloader(nil, &schema.SchemaCache{})
	relA := &schema.Relationship{ToSchema: "public", ToTable: "users", ToColumns: []string{"id"}}
	relB := &schema.Relationship{ToSchema: "public", ToTable: "users", ToColumns: []string{"id"}}
	relC := &schema.Relationship{ToSchema: "public", ToTable: "orgs", ToColumns: []string{"id"}}

	loaderA := dl.GetLoader(relA)
	loaderB := dl.GetLoader(relB)
	loaderC := dl.GetLoader(relC)

	testutil.True(t, loaderA == loaderB, "expected same loader for same relationship key")
	testutil.True(t, loaderA != loaderC, "expected different loader for different relationship key")
}

func TestLoaderLoadDoesNotDeadlockWhenBatchPrimesLoader(t *testing.T) {
	t.Parallel()

	type result struct {
		rows []map[string]any
		err  error
	}
	done := make(chan result, 1)
	var loader *Loader
	loader = newLoader(
		func(ctx context.Context, keys []interface{}) (map[interface{}][]map[string]any, error) {
			// Simulate recursive priming that can hit the same loader key space.
			loader.Prime(2)
			return map[interface{}][]map[string]any{
				1: {{"id": 1}},
			}, nil
		},
		func(ctx context.Context, key interface{}) ([]map[string]any, error) {
			return []map[string]any{{"id": key}}, nil
		},
	)
	loader.Prime(1)

	go func() {
		rows, err := loader.Load(context.Background(), 1)
		done <- result{rows: rows, err: err}
	}()

	select {
	case got := <-done:
		testutil.NoError(t, got.err)
		testutil.Equal(t, 1, len(got.rows))
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Load deadlocked while batch callback primed loader")
	}
}

func TestLoaderUsesSQLComparableKeySemantics(t *testing.T) {
	t.Parallel()

	singleCalls := 0
	loader := newLoader(
		func(ctx context.Context, keys []interface{}) (map[interface{}][]map[string]any, error) {
			return map[interface{}][]map[string]any{
				int32(1): {{"id": int32(1), "kind": "batch"}},
			}, nil
		},
		func(ctx context.Context, key interface{}) ([]map[string]any, error) {
			singleCalls++
			return []map[string]any{{"id": key, "kind": "single"}}, nil
		},
	)
	loader.Prime(1)

	intRows, err := loader.Load(context.Background(), 1)
	testutil.NoError(t, err)
	testutil.Equal(t, "batch", intRows[0]["kind"])

	stringRows, err := loader.Load(context.Background(), "1")
	testutil.NoError(t, err)
	testutil.Equal(t, "batch", stringRows[0]["kind"])
	testutil.Equal(t, 0, singleCalls)
}

func TestLoaderTreatsEquivalentNumericKeyTypesAsSame(t *testing.T) {
	t.Parallel()

	singleCalls := 0
	loader := newLoader(
		func(ctx context.Context, keys []interface{}) (map[interface{}][]map[string]any, error) {
			return map[interface{}][]map[string]any{
				int32(1): {{"id": int32(1), "kind": "numeric"}},
			}, nil
		},
		func(ctx context.Context, key interface{}) ([]map[string]any, error) {
			singleCalls++
			return []map[string]any{{"id": key, "kind": "single"}}, nil
		},
	)
	loader.Prime(int64(1))

	rows, err := loader.Load(context.Background(), int64(1))
	testutil.NoError(t, err)
	testutil.Equal(t, "numeric", rows[0]["kind"])
	testutil.Equal(t, 0, singleCalls)
}
