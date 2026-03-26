package schema

import (
	"context"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

func clearSRIDCacheForTests() {
	sridLookupCache.Range(func(key, _ any) bool {
		sridLookupCache.Delete(key)
		return true
	})
}

func TestLookupSRIDReturnsCachedValueWithoutDatabase(t *testing.T) {
	clearSRIDCacheForTests()
	t.Cleanup(clearSRIDCacheForTests)

	cached := &SRIDInfo{
		AuthName:    "EPSG",
		AuthSRID:    4326,
		Name:        "EPSG:4326",
		Description: "WGS 84",
	}
	sridLookupCache.Store(newSRIDLookupCacheKey(nil, 4326), cached)

	got, err := LookupSRID(context.Background(), nil, 4326)
	testutil.NoError(t, err)
	testutil.NotNil(t, got)
	testutil.Equal(t, cached.AuthName, got.AuthName)
	testutil.Equal(t, cached.AuthSRID, got.AuthSRID)
	testutil.Equal(t, cached.Name, got.Name)
	testutil.Equal(t, cached.Description, got.Description)
}

func TestLookupSRIDCacheIsScopedByPool(t *testing.T) {
	clearSRIDCacheForTests()
	t.Cleanup(clearSRIDCacheForTests)

	poolOne := newSRIDTestPool(t, "postgresql://user:pass@127.0.0.1:1/db_one?sslmode=disable")
	poolTwo := newSRIDTestPool(t, "postgresql://user:pass@127.0.0.1:1/db_two?sslmode=disable")

	first := &SRIDInfo{
		AuthName:    "EPSG",
		AuthSRID:    4326,
		Name:        "EPSG:4326",
		Description: "WGS 84 from db one",
	}
	second := &SRIDInfo{
		AuthName:    "CUSTOM",
		AuthSRID:    4326,
		Name:        "CUSTOM:4326",
		Description: "Custom from db two",
	}

	sridLookupCache.Store(newSRIDLookupCacheKey(poolOne, 4326), first)
	sridLookupCache.Store(newSRIDLookupCacheKey(poolTwo, 4326), second)

	firstResult, err := LookupSRID(context.Background(), poolOne, 4326)
	testutil.NoError(t, err)
	testutil.Equal(t, first.Description, firstResult.Description)
	testutil.Equal(t, first.AuthName, firstResult.AuthName)

	secondResult, err := LookupSRID(context.Background(), poolTwo, 4326)
	testutil.NoError(t, err)
	testutil.Equal(t, second.Description, secondResult.Description)
	testutil.Equal(t, second.AuthName, secondResult.AuthName)
}

func newSRIDTestPool(t *testing.T, connString string) *pgxpool.Pool {
	t.Helper()

	cfg, err := pgxpool.ParseConfig(connString)
	testutil.NoError(t, err)

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	testutil.NoError(t, err)
	t.Cleanup(pool.Close)

	return pool
}
