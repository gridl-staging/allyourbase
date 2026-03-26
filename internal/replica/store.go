// Package replica Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/replica/store.go.
package replica

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	TopologyRolePrimary = "primary"
	TopologyRoleReplica = "replica"

	TopologyStateActive   = "active"
	TopologyStateDraining = "draining"
	TopologyStateRemoved  = "removed"
)

var ErrTopologyNodeNotFound = errors.New("replica topology node not found")

type TopologyNodeRecord struct {
	Name        string
	Host        string
	Port        int
	Database    string
	SSLMode     string
	Query       string
	Role        string
	State       string
	Weight      int
	MaxLagBytes int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (r TopologyNodeRecord) ConnectionURL() string {
	databaseName := strings.TrimPrefix(r.Database, "/")
	hostPort := net.JoinHostPort(r.Host, strconv.Itoa(r.Port))

	return (&url.URL{
		Scheme:   "postgres",
		Host:     hostPort,
		Path:     "/" + databaseName,
		RawQuery: topologyRecordRawQuery(r.Query, r.SSLMode),
	}).String()
}

func topologyRecordRawQuery(rawQuery, sslMode string) string {
	rawQuery = strings.TrimPrefix(strings.TrimSpace(rawQuery), "?")
	if rawQuery != "" {
		return rawQuery
	}

	sslMode = strings.TrimSpace(sslMode)
	if sslMode == "" {
		return ""
	}

	values := url.Values{}
	values.Set("sslmode", sslMode)
	return values.Encode()
}

type ReplicaStore interface {
	List(ctx context.Context) ([]TopologyNodeRecord, error)
	Get(ctx context.Context, name string) (TopologyNodeRecord, error)
	IsEmpty(ctx context.Context) (bool, error)
	Bootstrap(ctx context.Context, records []TopologyNodeRecord) error
	UpdateState(ctx context.Context, name, state string) error
	Add(ctx context.Context, record TopologyNodeRecord) error
	PromoteNode(ctx context.Context, targetName string) error
}

type replicaStoreDB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type PostgresReplicaStore struct {
	db replicaStoreDB
}

type replicaRoleStateQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type replicaExecQuerier interface {
	replicaRoleStateQuerier
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type replicaTxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

var sensitiveTopologyQueryKeys = map[string]struct{}{
	"password":    {},
	"passfile":    {},
	"sslpassword": {},
	"user":        {},
}

const topologyColumns = `
	name,
	host,
	port,
	database,
	ssl_mode,
	dsn_query,
	role,
	state,
	weight,
	max_lag_bytes,
	created_at,
	updated_at
`

func NewPostgresReplicaStore(pool *pgxpool.Pool) *PostgresReplicaStore {
	return &PostgresReplicaStore{db: pool}
}

func NewPostgresReplicaStoreWithDB(db replicaStoreDB) *PostgresReplicaStore {
	return &PostgresReplicaStore{db: db}
}

// TODO: Document PostgresReplicaStore.List.
func (s *PostgresReplicaStore) List(ctx context.Context) ([]TopologyNodeRecord, error) {
	rows, err := s.db.Query(ctx, `SELECT `+topologyColumns+` FROM _ayb_replicas ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list replica topology records: %w", err)
	}
	defer rows.Close()

	records := make([]TopologyNodeRecord, 0)
	for rows.Next() {
		record, scanErr := scanTopologyNodeRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		records = append(records, record)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate replica topology records: %w", rows.Err())
	}
	return records, nil
}

func (s *PostgresReplicaStore) Get(ctx context.Context, name string) (TopologyNodeRecord, error) {
	record, err := scanTopologyNodeRecord(s.db.QueryRow(ctx, `SELECT `+topologyColumns+` FROM _ayb_replicas WHERE name = $1`, name))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TopologyNodeRecord{}, ErrTopologyNodeNotFound
		}
		return TopologyNodeRecord{}, fmt.Errorf("get replica topology record %q: %w", name, err)
	}
	return record, nil
}

func (s *PostgresReplicaStore) IsEmpty(ctx context.Context) (bool, error) {
	var empty bool
	if err := s.db.QueryRow(ctx, `SELECT NOT EXISTS (SELECT 1 FROM _ayb_replicas)`).Scan(&empty); err != nil {
		return false, fmt.Errorf("check replica topology emptiness: %w", err)
	}
	return empty, nil
}

func (s *PostgresReplicaStore) Bootstrap(ctx context.Context, records []TopologyNodeRecord) error {
	for _, record := range records {
		normalized, err := normalizeTopologyNodeRecord(record)
		if err != nil {
			return fmt.Errorf("bootstrap replica topology record %q: %w", record.Name, err)
		}
		if err := s.insertTopologyNodeRecord(ctx, normalized, true); err != nil {
			return fmt.Errorf("bootstrap replica topology record %q: %w", normalized.Name, err)
		}
	}
	return nil
}

func (s *PostgresReplicaStore) UpdateState(ctx context.Context, name, state string) error {
	normalizedState, err := normalizeTopologyState(state)
	if err != nil {
		return fmt.Errorf("update replica topology state for %q: %w", name, err)
	}

	result, err := s.db.Exec(ctx, `UPDATE _ayb_replicas SET state = $1, updated_at = NOW() WHERE name = $2`, normalizedState, name)
	if err != nil {
		return fmt.Errorf("update replica topology state for %q: %w", name, err)
	}
	if result.RowsAffected() == 0 {
		return ErrTopologyNodeNotFound
	}
	return nil
}

func (s *PostgresReplicaStore) Add(ctx context.Context, record TopologyNodeRecord) error {
	normalized, err := normalizeTopologyNodeRecord(record)
	if err != nil {
		return fmt.Errorf("add replica topology record %q: %w", record.Name, err)
	}

	if err := s.insertTopologyNodeRecord(ctx, normalized, false); err != nil {
		return fmt.Errorf("add replica topology record %q: %w", normalized.Name, err)
	}
	return nil
}

// TODO: Document PostgresReplicaStore.insertTopologyNodeRecord.
func (s *PostgresReplicaStore) insertTopologyNodeRecord(ctx context.Context, record TopologyNodeRecord, ignoreConflicts bool) error {
	query := `
		INSERT INTO _ayb_replicas (
			name,
			host,
			port,
			database,
			ssl_mode,
			dsn_query,
			role,
			state,
			weight,
			max_lag_bytes
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	if ignoreConflicts {
		query += "\n\t\tON CONFLICT DO NOTHING"
	}

	_, err := s.db.Exec(
		ctx,
		query,
		record.Name,
		record.Host,
		record.Port,
		record.Database,
		record.SSLMode,
		record.Query,
		record.Role,
		record.State,
		record.Weight,
		record.MaxLagBytes,
	)
	return err
}

// TODO: Document normalizeTopologyNodeRecord.
func normalizeTopologyNodeRecord(record TopologyNodeRecord) (TopologyNodeRecord, error) {
	record.Name = strings.TrimSpace(record.Name)
	record.Host = strings.TrimSpace(record.Host)
	record.Database = strings.TrimPrefix(strings.TrimSpace(record.Database), "/")
	record.SSLMode = strings.TrimSpace(record.SSLMode)
	record.Query = strings.TrimPrefix(strings.TrimSpace(record.Query), "?")
	record.Role = strings.TrimSpace(record.Role)
	record.State = strings.TrimSpace(record.State)

	if record.Name == "" {
		return TopologyNodeRecord{}, errors.New("name is required")
	}
	if record.Host == "" {
		return TopologyNodeRecord{}, errors.New("host is required")
	}
	if record.Port <= 0 {
		record.Port = 5432
	}
	if record.Database == "" {
		return TopologyNodeRecord{}, errors.New("database is required")
	}
	sanitizedQuery, sanitizedSSLMode, err := sanitizeTopologyQuery(record.Query, record.SSLMode)
	if err != nil {
		return TopologyNodeRecord{}, fmt.Errorf("invalid query %q: %w", record.Query, err)
	}
	record.Query = sanitizedQuery
	record.SSLMode = sanitizedSSLMode
	if record.Role != TopologyRolePrimary && record.Role != TopologyRoleReplica {
		return TopologyNodeRecord{}, fmt.Errorf("invalid role %q", record.Role)
	}
	if record.State == "" {
		record.State = TopologyStateActive
	}
	normalizedState, err := normalizeTopologyState(record.State)
	if err != nil {
		return TopologyNodeRecord{}, err
	}
	record.State = normalizedState
	if record.Weight <= 0 {
		record.Weight = config.DefaultReplicaWeight
	}
	if record.MaxLagBytes <= 0 {
		record.MaxLagBytes = config.DefaultReplicaMaxLagBytes
	}
	return record, nil
}

func normalizeTopologyState(state string) (string, error) {
	switch state {
	case TopologyStateActive, TopologyStateDraining, TopologyStateRemoved:
		return state, nil
	default:
		return "", fmt.Errorf("invalid state %q", state)
	}
}

// TODO: Document scanTopologyNodeRecord.
func scanTopologyNodeRecord(scanner interface{ Scan(dest ...any) error }) (TopologyNodeRecord, error) {
	var record TopologyNodeRecord
	if err := scanner.Scan(
		&record.Name,
		&record.Host,
		&record.Port,
		&record.Database,
		&record.SSLMode,
		&record.Query,
		&record.Role,
		&record.State,
		&record.Weight,
		&record.MaxLagBytes,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return TopologyNodeRecord{}, err
	}

	// Normalize legacy records so sensitive DSN query parameters are never surfaced.
	sanitizedQuery, sanitizedSSLMode, err := sanitizeTopologyQuery(record.Query, record.SSLMode)
	if err == nil {
		record.Query = sanitizedQuery
		record.SSLMode = sanitizedSSLMode
	} else {
		record.Query = ""
		record.SSLMode = strings.TrimSpace(record.SSLMode)
	}

	return record, nil
}

// TODO: Document sanitizeTopologyQuery.
func sanitizeTopologyQuery(rawQuery, sslMode string) (string, string, error) {
	rawQuery = strings.TrimPrefix(strings.TrimSpace(rawQuery), "?")
	sslMode = strings.TrimSpace(sslMode)
	if rawQuery == "" {
		return "", sslMode, nil
	}

	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", sslMode, err
	}
	stripSensitiveTopologyQueryKeys(values)

	if querySSLMode := strings.TrimSpace(values.Get("sslmode")); querySSLMode != "" {
		sslMode = querySSLMode
	}
	if sslMode != "" {
		values.Set("sslmode", sslMode)
	} else {
		values.Del("sslmode")
	}
	return values.Encode(), sslMode, nil
}

func stripSensitiveTopologyQueryKeys(values url.Values) {
	for key := range values {
		if _, sensitive := sensitiveTopologyQueryKeys[strings.ToLower(strings.TrimSpace(key))]; sensitive {
			values.Del(key)
		}
	}
}
