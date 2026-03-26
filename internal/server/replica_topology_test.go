package server

import (
	"net/url"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestTopologyRecordsFromConfigParsesPrimaryAndReplicas(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Database.URL = "postgres://ayb:secret@primary.local:6432/main_db?sslmode=require"
	cfg.Database.Replicas = []config.ReplicaConfig{
		{URL: "postgres://replica-a.local/app_db?sslmode=disable", Weight: 0, MaxLagBytes: 0},
		{URL: "postgres://replica-b.local:7432/app_db?sslmode=verify-full", Weight: 4, MaxLagBytes: 4096},
	}

	records, err := topologyRecordsFromConfig(cfg)
	testutil.NoError(t, err)
	testutil.SliceLen(t, records, 3)

	primary := records[0]
	testutil.Equal(t, "primary", primary.Name)
	testutil.Equal(t, "primary", primary.Role)
	testutil.Equal(t, "active", primary.State)
	testutil.Equal(t, "primary.local", primary.Host)
	testutil.Equal(t, 6432, primary.Port)
	testutil.Equal(t, "main_db", primary.Database)
	testutil.Equal(t, "require", primary.SSLMode)

	replicaOne := records[1]
	testutil.Equal(t, "replica", replicaOne.Role)
	testutil.Equal(t, "active", replicaOne.State)
	testutil.Equal(t, "replica-a.local", replicaOne.Host)
	testutil.Equal(t, 5432, replicaOne.Port)
	testutil.Equal(t, "app_db", replicaOne.Database)
	testutil.Equal(t, "disable", replicaOne.SSLMode)
	testutil.Equal(t, "sslmode=disable", replicaOne.Query)
	testutil.Equal(t, config.DefaultReplicaWeight, replicaOne.Weight)
	testutil.Equal(t, config.DefaultReplicaMaxLagBytes, replicaOne.MaxLagBytes)

	replicaTwo := records[2]
	testutil.Equal(t, "replica-b.local", replicaTwo.Host)
	testutil.Equal(t, 7432, replicaTwo.Port)
	testutil.Equal(t, 4, replicaTwo.Weight)
	testutil.Equal(t, int64(4096), replicaTwo.MaxLagBytes)
}

func TestTopologyRecordsFromConfigPreservesReplicaQueryHintsAndOmittedSSLMode(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Database.URL = "postgres://primary.local:5432/app?sslmode=require"
	cfg.Database.Replicas = []config.ReplicaConfig{
		{URL: "postgres://replica-a.local:5432/app?application_name=replica-a&target_session_attrs=read-write", Weight: 1, MaxLagBytes: 1},
	}

	records, err := topologyRecordsFromConfig(cfg)
	testutil.NoError(t, err)
	testutil.SliceLen(t, records, 2)

	replicaOne := records[1]
	testutil.Equal(t, "", replicaOne.SSLMode)
	parsed, err := url.Parse(replicaOne.ConnectionURL())
	testutil.NoError(t, err)
	testutil.Equal(t, "replica-a", parsed.Query().Get("application_name"))
	testutil.Equal(t, "read-write", parsed.Query().Get("target_session_attrs"))
	testutil.Equal(t, "", parsed.Query().Get("sslmode"))
}

func TestTopologyRecordsFromConfigInvalidPrimaryURL(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Database.URL = "://bad-primary-url"

	_, err := topologyRecordsFromConfig(cfg)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "parse primary database URL")
}

func TestTopologyRecordsFromConfigInvalidReplicaURL(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Database.URL = "postgres://primary.local:5432/app?sslmode=disable"
	cfg.Database.Replicas = []config.ReplicaConfig{
		{URL: "postgres://replica-a.local:5432/app?sslmode=disable", Weight: 1, MaxLagBytes: 1},
		{URL: "://bad-replica-url", Weight: 1, MaxLagBytes: 1},
	}

	_, err := topologyRecordsFromConfig(cfg)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "parse replica URL")
}
