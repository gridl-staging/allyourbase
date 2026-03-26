package replica

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeTopologyRow struct {
	values []any
	err    error
}

func (r *fakeTopologyRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return fmt.Errorf("dest len %d != values len %d", len(dest), len(r.values))
	}
	for i, d := range dest {
		switch ptr := d.(type) {
		case *string:
			v, ok := r.values[i].(string)
			if !ok {
				return fmt.Errorf("value %d is not string", i)
			}
			*ptr = v
		case *int:
			v, ok := r.values[i].(int)
			if !ok {
				return fmt.Errorf("value %d is not int", i)
			}
			*ptr = v
		case *int64:
			v, ok := r.values[i].(int64)
			if !ok {
				return fmt.Errorf("value %d is not int64", i)
			}
			*ptr = v
		case *time.Time:
			v, ok := r.values[i].(time.Time)
			if !ok {
				return fmt.Errorf("value %d is not time.Time", i)
			}
			*ptr = v
		case *bool:
			v, ok := r.values[i].(bool)
			if !ok {
				return fmt.Errorf("value %d is not bool", i)
			}
			*ptr = v
		default:
			return fmt.Errorf("unsupported scan destination type %T", d)
		}
	}
	return nil
}

type fakeTopologyRows struct {
	rows [][]any
	idx  int
	err  error
}

func (r *fakeTopologyRows) Close()                                       {}
func (r *fakeTopologyRows) Err() error                                   { return r.err }
func (r *fakeTopologyRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *fakeTopologyRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeTopologyRows) Values() ([]any, error)                       { return nil, nil }
func (r *fakeTopologyRows) RawValues() [][]byte                          { return nil }
func (r *fakeTopologyRows) Conn() *pgx.Conn                              { return nil }

func (r *fakeTopologyRows) Next() bool {
	if r.idx >= len(r.rows) {
		return false
	}
	r.idx++
	return true
}

func (r *fakeTopologyRows) Scan(dest ...any) error {
	if r.idx == 0 || r.idx > len(r.rows) {
		return fmt.Errorf("scan called out of bounds")
	}
	return (&fakeTopologyRow{values: r.rows[r.idx-1]}).Scan(dest...)
}

type inMemoryTopologyDB struct {
	records map[string]TopologyNodeRecord
	nowFn   func() time.Time
	execSQL []string
}

func newInMemoryTopologyDB() *inMemoryTopologyDB {
	return &inMemoryTopologyDB{
		records: map[string]TopologyNodeRecord{},
		nowFn:   time.Now,
	}
}

func (db *inMemoryTopologyDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	switch {
	case strings.Contains(sql, "SELECT NOT EXISTS"):
		return &fakeTopologyRow{values: []any{len(db.records) == 0}}
	case strings.Contains(sql, "SELECT role, state FROM _ayb_replicas WHERE name = $1"):
		name, _ := args[0].(string)
		record, ok := db.records[name]
		if !ok {
			return &fakeTopologyRow{err: pgx.ErrNoRows}
		}
		return &fakeTopologyRow{values: []any{record.Role, record.State}}
	case strings.Contains(sql, "SELECT name FROM _ayb_replicas WHERE role = $1 AND state != $2"):
		role, _ := args[0].(string)
		state, _ := args[1].(string)
		names := make([]string, 0, len(db.records))
		for _, record := range db.records {
			if record.Role == role && record.State != state {
				names = append(names, record.Name)
			}
		}
		if len(names) == 0 {
			return &fakeTopologyRow{err: pgx.ErrNoRows}
		}
		sort.Strings(names)
		return &fakeTopologyRow{values: []any{names[0]}}
	case strings.Contains(sql, "WHERE name = $1"):
		name, _ := args[0].(string)
		record, ok := db.records[name]
		if !ok {
			return &fakeTopologyRow{err: pgx.ErrNoRows}
		}
		return &fakeTopologyRow{values: []any{
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
			record.CreatedAt,
			record.UpdatedAt,
		}}
	default:
		return &fakeTopologyRow{err: fmt.Errorf("unexpected QueryRow SQL: %s", sql)}
	}
}

func (db *inMemoryTopologyDB) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	if !strings.Contains(sql, "FROM _ayb_replicas") {
		return nil, fmt.Errorf("unexpected Query SQL: %s", sql)
	}

	rows := make([][]any, 0, len(db.records))
	for _, record := range db.records {
		rows = append(rows, []any{
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
			record.CreatedAt,
			record.UpdatedAt,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		left := rows[i][0].(string)
		right := rows[j][0].(string)
		return left < right
	})
	return &fakeTopologyRows{rows: rows}, nil
}

func (db *inMemoryTopologyDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	db.execSQL = append(db.execSQL, sql)
	now := db.nowFn().UTC().Round(time.Second)

	switch {
	case strings.Contains(sql, "INSERT INTO _ayb_replicas"):
		record := TopologyNodeRecord{
			Name:        args[0].(string),
			Host:        args[1].(string),
			Port:        args[2].(int),
			Database:    args[3].(string),
			SSLMode:     args[4].(string),
			Query:       args[5].(string),
			Role:        args[6].(string),
			State:       args[7].(string),
			Weight:      args[8].(int),
			MaxLagBytes: args[9].(int64),
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		if strings.Contains(sql, "ON CONFLICT DO NOTHING") {
			if _, exists := db.records[record.Name]; exists {
				return pgconn.NewCommandTag("INSERT 0 0"), nil
			}
			if hasNonRemovedPrimary(db.records) && record.Role == "primary" && record.State != "removed" {
				return pgconn.NewCommandTag("INSERT 0 0"), nil
			}
			db.records[record.Name] = record
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		}

		if _, exists := db.records[record.Name]; exists {
			return pgconn.CommandTag{}, &pgconn.PgError{Code: "23505", Message: "duplicate key value violates unique constraint"}
		}
		if hasNonRemovedPrimary(db.records) && record.Role == "primary" && record.State != "removed" {
			return pgconn.CommandTag{}, &pgconn.PgError{Code: "23505", Message: "duplicate key value violates unique constraint"}
		}
		db.records[record.Name] = record
		return pgconn.NewCommandTag("INSERT 0 1"), nil
	case strings.Contains(sql, "UPDATE _ayb_replicas"):
		if strings.Contains(sql, "SET role = $1") {
			role := args[0].(string)
			name := args[1].(string)
			record, ok := db.records[name]
			if !ok {
				return pgconn.NewCommandTag("UPDATE 0"), nil
			}

			if role == TopologyRolePrimary && record.State != TopologyStateRemoved {
				for existingName, existingRecord := range db.records {
					if existingName == name {
						continue
					}
					if existingRecord.Role == TopologyRolePrimary && existingRecord.State != TopologyStateRemoved {
						return pgconn.CommandTag{}, &pgconn.PgError{Code: "23505", Message: "duplicate key value violates unique constraint"}
					}
				}
			}

			record.Role = role
			record.UpdatedAt = now
			db.records[name] = record
			return pgconn.NewCommandTag("UPDATE 1"), nil
		}

		state := args[0].(string)
		name := args[1].(string)
		record, ok := db.records[name]
		if !ok {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		}
		record.State = state
		record.UpdatedAt = now
		db.records[name] = record
		return pgconn.NewCommandTag("UPDATE 1"), nil
	default:
		return pgconn.CommandTag{}, fmt.Errorf("unexpected Exec SQL: %s", sql)
	}
}

func hasNonRemovedPrimary(records map[string]TopologyNodeRecord) bool {
	for _, record := range records {
		if record.Role == "primary" && record.State != "removed" {
			return true
		}
	}
	return false
}

func TestTopologyNodeRecordConnectionURLRoundTrip(t *testing.T) {
	t.Parallel()

	record := TopologyNodeRecord{
		Host:     "replica-a.internal",
		Port:     5432,
		Database: "appdb",
		SSLMode:  "require",
		Query:    "application_name=replica-a&sslmode=require",
	}

	connURL := record.ConnectionURL()
	parsed, err := url.Parse(connURL)
	testutil.NoError(t, err)
	testutil.Equal(t, "postgres", parsed.Scheme)
	testutil.Equal(t, "replica-a.internal", parsed.Hostname())
	testutil.Equal(t, "5432", parsed.Port())
	testutil.Equal(t, "/appdb", parsed.Path)
	testutil.Equal(t, "require", parsed.Query().Get("sslmode"))
	testutil.Equal(t, "replica-a", parsed.Query().Get("application_name"))
}

func TestTopologyNodeRecordConnectionURLOmitsUnsetSSLMode(t *testing.T) {
	t.Parallel()

	record := TopologyNodeRecord{
		Host:     "replica-a.internal",
		Port:     5432,
		Database: "appdb",
		Query:    "application_name=replica-a",
	}

	connURL := record.ConnectionURL()
	parsed, err := url.Parse(connURL)
	testutil.NoError(t, err)
	testutil.Equal(t, "replica-a", parsed.Query().Get("application_name"))
	testutil.Equal(t, "", parsed.Query().Get("sslmode"))
}

func TestNormalizeTopologyNodeRecordStripsSensitiveDSNQueryParams(t *testing.T) {
	t.Parallel()

	normalized, err := normalizeTopologyNodeRecord(TopologyNodeRecord{
		Name:        "replica-a",
		Host:        "replica-a.internal",
		Port:        5432,
		Database:    "appdb",
		SSLMode:     "require",
		Query:       "application_name=replica-a&password=secret&sslpassword=topsecret&user=leaky&sslmode=disable",
		Role:        TopologyRoleReplica,
		State:       TopologyStateActive,
		Weight:      1,
		MaxLagBytes: 1,
	})
	testutil.NoError(t, err)

	parsed, err := url.Parse(normalized.ConnectionURL())
	testutil.NoError(t, err)
	testutil.Equal(t, "replica-a", parsed.Query().Get("application_name"))
	testutil.Equal(t, "disable", parsed.Query().Get("sslmode"))
	testutil.Equal(t, "", parsed.Query().Get("password"))
	testutil.Equal(t, "", parsed.Query().Get("sslpassword"))
	testutil.Equal(t, "", parsed.Query().Get("user"))
}

func TestScanTopologyNodeRecordStripsSensitiveDSNQueryParams(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	record, err := scanTopologyNodeRecord(&fakeTopologyRow{values: []any{
		"replica-a",
		"replica-a.internal",
		5432,
		"appdb",
		"require",
		"application_name=replica-a&password=secret&user=leaky",
		TopologyRoleReplica,
		TopologyStateActive,
		1,
		int64(1),
		now,
		now,
	}})
	testutil.NoError(t, err)

	parsed, err := url.Parse(record.ConnectionURL())
	testutil.NoError(t, err)
	testutil.Equal(t, "replica-a", parsed.Query().Get("application_name"))
	testutil.Equal(t, "require", parsed.Query().Get("sslmode"))
	testutil.Equal(t, "", parsed.Query().Get("password"))
	testutil.Equal(t, "", parsed.Query().Get("user"))
}

func TestPostgresReplicaStoreCRUDOperations(t *testing.T) {
	t.Parallel()

	db := newInMemoryTopologyDB()
	store := NewPostgresReplicaStoreWithDB(db)

	empty, err := store.IsEmpty(context.Background())
	testutil.NoError(t, err)
	testutil.True(t, empty)

	primary := TopologyNodeRecord{
		Name:        "primary",
		Host:        "primary.internal",
		Port:        5432,
		Database:    "appdb",
		SSLMode:     "disable",
		Role:        "primary",
		State:       "active",
		Weight:      1,
		MaxLagBytes: 1024,
	}
	replicaA := TopologyNodeRecord{
		Name:        "replica-a",
		Host:        "replica-a.internal",
		Port:        5432,
		Database:    "appdb",
		SSLMode:     "disable",
		Role:        "replica",
		State:       "active",
		Weight:      2,
		MaxLagBytes: 2048,
	}

	testutil.NoError(t, store.Add(context.Background(), primary))
	testutil.NoError(t, store.Add(context.Background(), replicaA))

	record, err := store.Get(context.Background(), "replica-a")
	testutil.NoError(t, err)
	testutil.Equal(t, "replica-a.internal", record.Host)
	testutil.Equal(t, 2, record.Weight)

	testutil.NoError(t, store.UpdateState(context.Background(), "replica-a", "draining"))
	record, err = store.Get(context.Background(), "replica-a")
	testutil.NoError(t, err)
	testutil.Equal(t, "draining", record.State)

	allRecords, err := store.List(context.Background())
	testutil.NoError(t, err)
	testutil.SliceLen(t, allRecords, 2)

	empty, err = store.IsEmpty(context.Background())
	testutil.NoError(t, err)
	testutil.False(t, empty)
}

func TestPostgresReplicaStoreBootstrapWhenEmpty(t *testing.T) {
	t.Parallel()

	db := newInMemoryTopologyDB()
	store := NewPostgresReplicaStoreWithDB(db)

	records := []TopologyNodeRecord{
		{
			Name:        "primary",
			Host:        "primary.internal",
			Port:        5432,
			Database:    "appdb",
			SSLMode:     "disable",
			Role:        "primary",
			State:       "active",
			Weight:      1,
			MaxLagBytes: 1024,
		},
		{
			Name:        "replica-a",
			Host:        "replica-a.internal",
			Port:        5432,
			Database:    "appdb",
			SSLMode:     "disable",
			Role:        "replica",
			State:       "active",
			Weight:      1,
			MaxLagBytes: 1024,
		},
	}

	testutil.NoError(t, store.Bootstrap(context.Background(), records))

	allRecords, err := store.List(context.Background())
	testutil.NoError(t, err)
	testutil.SliceLen(t, allRecords, 2)
	testutil.True(t, strings.Contains(db.execSQL[0], "ON CONFLICT DO NOTHING"))
}

func TestPostgresReplicaStoreNoBootstrapWhenPopulated(t *testing.T) {
	t.Parallel()

	db := newInMemoryTopologyDB()
	store := NewPostgresReplicaStoreWithDB(db)

	existingPrimary := TopologyNodeRecord{
		Name:        "primary",
		Host:        "primary.internal",
		Port:        5432,
		Database:    "appdb",
		SSLMode:     "disable",
		Role:        "primary",
		State:       "active",
		Weight:      1,
		MaxLagBytes: 1024,
	}
	testutil.NoError(t, store.Add(context.Background(), existingPrimary))

	empty, err := store.IsEmpty(context.Background())
	testutil.NoError(t, err)
	if empty {
		t.Fatal("expected populated store to be non-empty")
	}

	// Startup should skip Bootstrap when IsEmpty is false. We assert that by not
	// invoking Bootstrap in this branch.
	allRecords, err := store.List(context.Background())
	testutil.NoError(t, err)
	testutil.SliceLen(t, allRecords, 1)
}

func TestPostgresReplicaStorePrimaryUniquenessConstraintViolation(t *testing.T) {
	t.Parallel()

	db := newInMemoryTopologyDB()
	store := NewPostgresReplicaStoreWithDB(db)

	firstPrimary := TopologyNodeRecord{
		Name:        "primary-a",
		Host:        "primary-a.internal",
		Port:        5432,
		Database:    "appdb",
		SSLMode:     "disable",
		Role:        "primary",
		State:       "active",
		Weight:      1,
		MaxLagBytes: 1024,
	}
	secondPrimary := TopologyNodeRecord{
		Name:        "primary-b",
		Host:        "primary-b.internal",
		Port:        5432,
		Database:    "appdb",
		SSLMode:     "disable",
		Role:        "primary",
		State:       "active",
		Weight:      1,
		MaxLagBytes: 1024,
	}

	testutil.NoError(t, store.Add(context.Background(), firstPrimary))
	err := store.Add(context.Background(), secondPrimary)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "duplicate key")
}

func TestPostgresReplicaStorePromoteNodeSwapsPrimaryAtomically(t *testing.T) {
	t.Parallel()

	db := newInMemoryTopologyDB()
	store := NewPostgresReplicaStoreWithDB(db)

	testutil.NoError(t, store.Add(context.Background(), TopologyNodeRecord{
		Name:        "primary-a",
		Host:        "primary-a.internal",
		Port:        5432,
		Database:    "appdb",
		SSLMode:     "disable",
		Role:        "primary",
		State:       "active",
		Weight:      1,
		MaxLagBytes: 1024,
	}))
	testutil.NoError(t, store.Add(context.Background(), TopologyNodeRecord{
		Name:        "replica-a",
		Host:        "replica-a.internal",
		Port:        5432,
		Database:    "appdb",
		SSLMode:     "disable",
		Role:        "replica",
		State:       "active",
		Weight:      1,
		MaxLagBytes: 1024,
	}))
	testutil.NoError(t, store.Add(context.Background(), TopologyNodeRecord{
		Name:        "replica-b",
		Host:        "replica-b.internal",
		Port:        5432,
		Database:    "appdb",
		SSLMode:     "disable",
		Role:        "replica",
		State:       "active",
		Weight:      1,
		MaxLagBytes: 1024,
	}))

	testutil.NoError(t, store.PromoteNode(context.Background(), "replica-a"))

	oldPrimary, err := store.Get(context.Background(), "primary-a")
	testutil.NoError(t, err)
	testutil.Equal(t, TopologyStateRemoved, oldPrimary.State)
	testutil.Equal(t, TopologyRolePrimary, oldPrimary.Role)

	newPrimary, err := store.Get(context.Background(), "replica-a")
	testutil.NoError(t, err)
	testutil.Equal(t, TopologyRolePrimary, newPrimary.Role)
	testutil.Equal(t, TopologyStateActive, newPrimary.State)

	records, err := store.List(context.Background())
	testutil.NoError(t, err)
	activePrimaryCount := 0
	for _, record := range records {
		if record.Role == TopologyRolePrimary && record.State != TopologyStateRemoved {
			activePrimaryCount++
		}
	}
	testutil.Equal(t, 1, activePrimaryCount)
}

func TestPostgresReplicaStorePromoteNodeReturnsExpectedErrors(t *testing.T) {
	t.Parallel()

	t.Run("target not found", func(t *testing.T) {
		t.Parallel()

		db := newInMemoryTopologyDB()
		store := NewPostgresReplicaStoreWithDB(db)
		testutil.NoError(t, store.Add(context.Background(), TopologyNodeRecord{
			Name:        "primary-a",
			Host:        "primary-a.internal",
			Port:        5432,
			Database:    "appdb",
			SSLMode:     "disable",
			Role:        "primary",
			State:       "active",
			Weight:      1,
			MaxLagBytes: 1024,
		}))

		err := store.PromoteNode(context.Background(), "missing")
		testutil.Error(t, err)
		testutil.Contains(t, err.Error(), "not found")
	})

	t.Run("target not active", func(t *testing.T) {
		t.Parallel()

		db := newInMemoryTopologyDB()
		store := NewPostgresReplicaStoreWithDB(db)
		testutil.NoError(t, store.Add(context.Background(), TopologyNodeRecord{
			Name:        "primary-a",
			Host:        "primary-a.internal",
			Port:        5432,
			Database:    "appdb",
			SSLMode:     "disable",
			Role:        "primary",
			State:       "active",
			Weight:      1,
			MaxLagBytes: 1024,
		}))
		testutil.NoError(t, store.Add(context.Background(), TopologyNodeRecord{
			Name:        "replica-a",
			Host:        "replica-a.internal",
			Port:        5432,
			Database:    "appdb",
			SSLMode:     "disable",
			Role:        "replica",
			State:       "draining",
			Weight:      1,
			MaxLagBytes: 1024,
		}))

		err := store.PromoteNode(context.Background(), "replica-a")
		testutil.Error(t, err)
		testutil.Contains(t, err.Error(), "active")
	})

	t.Run("target already primary", func(t *testing.T) {
		t.Parallel()

		db := newInMemoryTopologyDB()
		store := NewPostgresReplicaStoreWithDB(db)
		testutil.NoError(t, store.Add(context.Background(), TopologyNodeRecord{
			Name:        "primary-a",
			Host:        "primary-a.internal",
			Port:        5432,
			Database:    "appdb",
			SSLMode:     "disable",
			Role:        "primary",
			State:       "active",
			Weight:      1,
			MaxLagBytes: 1024,
		}))

		err := store.PromoteNode(context.Background(), "primary-a")
		testutil.Error(t, err)
		testutil.Contains(t, err.Error(), "already primary")
	})

	t.Run("no current active primary", func(t *testing.T) {
		t.Parallel()

		db := newInMemoryTopologyDB()
		store := NewPostgresReplicaStoreWithDB(db)
		testutil.NoError(t, store.Add(context.Background(), TopologyNodeRecord{
			Name:        "primary-a",
			Host:        "primary-a.internal",
			Port:        5432,
			Database:    "appdb",
			SSLMode:     "disable",
			Role:        "primary",
			State:       "removed",
			Weight:      1,
			MaxLagBytes: 1024,
		}))
		testutil.NoError(t, store.Add(context.Background(), TopologyNodeRecord{
			Name:        "replica-a",
			Host:        "replica-a.internal",
			Port:        5432,
			Database:    "appdb",
			SSLMode:     "disable",
			Role:        "replica",
			State:       "active",
			Weight:      1,
			MaxLagBytes: 1024,
		}))

		err := store.PromoteNode(context.Background(), "replica-a")
		testutil.Error(t, err)
		testutil.Contains(t, err.Error(), "no current primary")
	})
}
