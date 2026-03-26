package config

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func parseConfigFromTOML(t *testing.T, toml string) *Config {
	t.Helper()
	cfg, err := ParseTOML([]byte(toml))
	testutil.NoError(t, err)
	return cfg
}

func assertStringSlice(t *testing.T, got []string, want ...string) {
	t.Helper()
	testutil.SliceLen(t, got, len(want))
	for i, expected := range want {
		testutil.Equal(t, expected, got[i])
	}
}

func assertManagedPGDefaults(t *testing.T, cfg ManagedPGConfig) {
	t.Helper()
	testutil.Equal(t, 15432, cfg.Port)
	testutil.Equal(t, "16", cfg.PGVersion)
	testutil.Equal(t, "", cfg.DataDir)
	testutil.Equal(t, "", cfg.BinaryURL)
	assertStringSlice(t, cfg.Extensions, "pgvector", "pg_trgm", "pg_cron")
	assertStringSlice(t, cfg.SharedPreloadLibraries, "pg_stat_statements")
}

func assertEffectiveExtensions(t *testing.T, cfg ManagedPGConfig, want ...string) {
	t.Helper()
	assertStringSlice(t, cfg.EffectiveExtensions(), want...)
}

func TestManagedPGDefaults(t *testing.T) {
	t.Parallel()
	cfg := Default()

	assertManagedPGDefaults(t, cfg.ManagedPG)
}

func TestManagedPGTOMLRoundTrip(t *testing.T) {
	t.Parallel()
	toml := `
[server]
port = 8090

[managed_pg]
port = 25432
data_dir = "/tmp/pg_data"
binary_url = "https://example.com/pg/{version}/{platform}.tar.xz"
pg_version = "17"
extensions = ["pgvector", "pg_trgm"]
shared_preload_libraries = ["pg_stat_statements"]
`
	cfg := parseConfigFromTOML(t, toml)

	testutil.Equal(t, 25432, cfg.ManagedPG.Port)
	testutil.Equal(t, "/tmp/pg_data", cfg.ManagedPG.DataDir)
	testutil.Equal(t, "https://example.com/pg/{version}/{platform}.tar.xz", cfg.ManagedPG.BinaryURL)
	testutil.Equal(t, "17", cfg.ManagedPG.PGVersion)
	assertStringSlice(t, cfg.ManagedPG.Extensions, "pgvector", "pg_trgm")
	assertStringSlice(t, cfg.ManagedPG.SharedPreloadLibraries, "pg_stat_statements")
}

func TestManagedPGDefaultsAppliedWhenSectionOmitted(t *testing.T) {
	t.Parallel()
	toml := `
[server]
port = 8090
`
	cfg := parseConfigFromTOML(t, toml)

	assertManagedPGDefaults(t, cfg.ManagedPG)
}

func TestManagedPGBinaryURLOverride(t *testing.T) {
	t.Parallel()
	toml := `
[server]
port = 8090

[managed_pg]
binary_url = "https://my-cdn.example.com/postgres/{version}/{platform}.tar.xz"
`
	cfg := parseConfigFromTOML(t, toml)

	testutil.Equal(t, "https://my-cdn.example.com/postgres/{version}/{platform}.tar.xz", cfg.ManagedPG.BinaryURL)
	// Other defaults should still apply.
	testutil.Equal(t, 15432, cfg.ManagedPG.Port)
	testutil.Equal(t, "16", cfg.ManagedPG.PGVersion)
}

func TestEffectiveExtensionsPostGISTrue(t *testing.T) {
	t.Parallel()
	cfg := ManagedPGConfig{
		PostGIS:    true,
		Extensions: []string{"pgvector", "pg_trgm"},
	}
	assertEffectiveExtensions(t, cfg, "postgis", "pgvector", "pg_trgm")
}

func TestEffectiveExtensionsPostGISFalse(t *testing.T) {
	t.Parallel()
	cfg := ManagedPGConfig{
		PostGIS:    false,
		Extensions: []string{"pgvector", "pg_trgm"},
	}
	assertEffectiveExtensions(t, cfg, "pgvector", "pg_trgm")
}

func TestEffectiveExtensionsPostGISTrueDedup(t *testing.T) {
	t.Parallel()
	cfg := ManagedPGConfig{
		PostGIS:    true,
		Extensions: []string{"postgis", "pgvector"},
	}
	// Should NOT duplicate "postgis".
	assertEffectiveExtensions(t, cfg, "postgis", "pgvector")
}

func TestEffectiveExtensionsPostGISTrueMovesPostGISToFront(t *testing.T) {
	t.Parallel()
	cfg := ManagedPGConfig{
		PostGIS:    true,
		Extensions: []string{"pgvector", "postgis", "postgis_topology"},
	}
	assertEffectiveExtensions(t, cfg, "postgis", "pgvector", "postgis_topology")
}

func TestEffectiveExtensionsPostGISTrueNormalizesManualPostGISVariants(t *testing.T) {
	t.Parallel()
	cfg := ManagedPGConfig{
		PostGIS:    true,
		Extensions: []string{" PostGIS ", "pgvector"},
	}
	assertEffectiveExtensions(t, cfg, "postgis", "pgvector")
}

func TestEffectiveExtensionsPostGISFalseManualPostGIS(t *testing.T) {
	t.Parallel()
	cfg := ManagedPGConfig{
		PostGIS:    false,
		Extensions: []string{"postgis", "pgvector"},
	}
	// Manual "postgis" in Extensions is preserved even when PostGIS toggle is false.
	assertEffectiveExtensions(t, cfg, "postgis", "pgvector")
}

func TestEffectiveExtensionsPostGISTOML(t *testing.T) {
	t.Parallel()
	toml := `
[managed_pg]
postgis = true
extensions = ["pgvector"]
`
	cfg := parseConfigFromTOML(t, toml)
	testutil.True(t, cfg.ManagedPG.PostGIS, "PostGIS toggle should be true from TOML")
	assertEffectiveExtensions(t, cfg.ManagedPG, "postgis", "pgvector")
}
