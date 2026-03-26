// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/server/replica_topology.go.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/replica"
)

// TODO: Document bootstrapReplicaStoreFromConfig.
func bootstrapReplicaStoreFromConfig(ctx context.Context, cfg *config.Config, store replica.ReplicaStore, logger *slog.Logger) {
	if cfg == nil || store == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	if ctx == nil {
		ctx = context.Background()
	}

	empty, err := safeReplicaStoreIsEmpty(ctx, store)
	if err != nil {
		logger.Warn("replica topology store availability check failed; skipping bootstrap", "error", err)
		return
	}
	if !empty {
		return
	}

	records, err := topologyRecordsFromConfig(cfg)
	if err != nil {
		logger.Warn("replica topology bootstrap skipped due to invalid config", "error", err)
		return
	}
	if err := store.Bootstrap(ctx, records); err != nil {
		logger.Warn("replica topology bootstrap failed", "error", err)
		return
	}
	logger.Info("replica topology bootstrap completed", "records", len(records))
}

// TODO: Document topologyRecordsFromConfig.
func topologyRecordsFromConfig(cfg *config.Config) ([]replica.TopologyNodeRecord, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	primaryRecord, err := topologyRecordFromURL("primary", cfg.Database.URL, replica.TopologyRolePrimary)
	if err != nil {
		return nil, fmt.Errorf("parse primary database URL: %w", err)
	}
	primaryRecord.State = replica.TopologyStateActive
	primaryRecord.Weight = config.DefaultReplicaWeight
	primaryRecord.MaxLagBytes = config.DefaultReplicaMaxLagBytes

	records := make([]replica.TopologyNodeRecord, 0, len(cfg.Database.Replicas)+1)
	records = append(records, primaryRecord)

	for i, replicaConfig := range cfg.Database.Replicas {
		record, parseErr := topologyRecordFromURL(fmt.Sprintf("replica-%d", i+1), replicaConfig.URL, replica.TopologyRoleReplica)
		if parseErr != nil {
			return nil, fmt.Errorf("parse replica URL at index %d: %w", i, parseErr)
		}
		normalizedReplicaConfig := replica.NormalizeReplicaConfig(replicaConfig)
		record.State = replica.TopologyStateActive
		record.Weight = normalizedReplicaConfig.Weight
		record.MaxLagBytes = normalizedReplicaConfig.MaxLagBytes
		records = append(records, record)
	}

	return records, nil
}

// TODO: Document topologyRecordFromURL.
func topologyRecordFromURL(name, rawURL, role string) (replica.TopologyNodeRecord, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return replica.TopologyNodeRecord{}, err
	}
	if parsed.Hostname() == "" {
		return replica.TopologyNodeRecord{}, fmt.Errorf("missing host")
	}

	port, err := topologyPortFromURL(parsed)
	if err != nil {
		return replica.TopologyNodeRecord{}, err
	}

	databaseName := strings.TrimPrefix(parsed.Path, "/")
	if databaseName == "" {
		return replica.TopologyNodeRecord{}, fmt.Errorf("missing database path")
	}

	sslMode := parsed.Query().Get("sslmode")

	return replica.TopologyNodeRecord{
		Name:        name,
		Host:        parsed.Hostname(),
		Port:        port,
		Database:    databaseName,
		SSLMode:     sslMode,
		Query:       parsed.RawQuery,
		Role:        role,
		State:       replica.TopologyStateActive,
		Weight:      config.DefaultReplicaWeight,
		MaxLagBytes: config.DefaultReplicaMaxLagBytes,
	}, nil
}

func topologyPortFromURL(parsed *url.URL) (int, error) {
	if parsed.Port() == "" {
		return 5432, nil
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		return 0, fmt.Errorf("invalid port %q", parsed.Port())
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("port %d is out of range", port)
	}
	return port, nil
}

func safeReplicaStoreIsEmpty(ctx context.Context, store replica.ReplicaStore) (empty bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("replica store IsEmpty panic: %v", recovered)
		}
	}()
	return store.IsEmpty(ctx)
}

func safeReplicaStoreList(ctx context.Context, store replica.ReplicaStore) (records []replica.TopologyNodeRecord, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("replica store List panic: %v", recovered)
		}
	}()
	return store.List(ctx)
}
