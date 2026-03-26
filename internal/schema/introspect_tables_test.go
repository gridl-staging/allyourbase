package schema

import (
	"context"
	"errors"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type stubExtensionStatusQuerier struct {
	lastQuery string
	lastArgs  []any
	row       pgx.Row
}

func (s *stubExtensionStatusQuerier) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	s.lastQuery = sql
	s.lastArgs = args
	return s.row
}

type stubExtensionStatusRow struct {
	version string
	err     error
}

func (r stubExtensionStatusRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != 1 {
		return errors.New("expected exactly one scan destination")
	}
	versionDest, ok := dest[0].(*string)
	if !ok {
		return errors.New("expected *string scan destination")
	}
	*versionDest = r.version
	return nil
}

func TestLoadPostGISExtensionStatus(t *testing.T) {
	t.Parallel()

	t.Run("returns extension version when installed", func(t *testing.T) {
		t.Parallel()

		querier := &stubExtensionStatusQuerier{
			row: stubExtensionStatusRow{version: "3.5.2"},
		}

		hasExtension, version, err := loadPostGISExtensionStatus(context.Background(), querier, "postgis")
		testutil.NoError(t, err)
		testutil.True(t, hasExtension)
		testutil.Equal(t, "3.5.2", version)
		testutil.Equal(t, "SELECT extversion FROM pg_extension WHERE extname = $1", querier.lastQuery)
		testutil.SliceLen(t, querier.lastArgs, 1)
		testutil.Equal(t, "postgis", querier.lastArgs[0].(string))
	})

	t.Run("returns absent when extension is not installed", func(t *testing.T) {
		t.Parallel()

		querier := &stubExtensionStatusQuerier{
			row: stubExtensionStatusRow{err: pgx.ErrNoRows},
		}

		hasExtension, version, err := loadPostGISExtensionStatus(context.Background(), querier, "postgis_raster")
		testutil.NoError(t, err)
		testutil.False(t, hasExtension)
		testutil.Equal(t, "", version)
	})

	t.Run("wraps query errors with extension name", func(t *testing.T) {
		t.Parallel()

		querier := &stubExtensionStatusQuerier{
			row: stubExtensionStatusRow{err: errors.New("permission denied")},
		}

		hasExtension, version, err := loadPostGISExtensionStatus(context.Background(), querier, "postgis_raster")
		testutil.ErrorContains(t, err, "querying postgis_raster extension")
		testutil.False(t, hasExtension)
		testutil.Equal(t, "", version)
	})
}

func TestEnrichSpatialColumnsAfterBaseScan(t *testing.T) {
	t.Parallel()

	t.Run("skips enrichment when postgis is disabled", func(t *testing.T) {
		t.Parallel()

		tables := map[string]*Table{
			"public.places": {Schema: "public", Name: "places"},
		}
		called := false

		err := enrichSpatialColumnsAfterBaseScan(
			context.Background(),
			nil,
			tables,
			false,
			func(context.Context, *pgxpool.Pool, map[string]*Table) error {
				called = true
				return nil
			},
		)
		testutil.NoError(t, err)
		testutil.False(t, called)
	})

	t.Run("runs enrichment and returns errors when postgis is enabled", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("spatial view unavailable")
		called := false

		err := enrichSpatialColumnsAfterBaseScan(
			context.Background(),
			nil,
			map[string]*Table{},
			true,
			func(context.Context, *pgxpool.Pool, map[string]*Table) error {
				called = true
				return expectedErr
			},
		)

		testutil.True(t, called)
		testutil.ErrorContains(t, err, "spatial view unavailable")
	})
}
