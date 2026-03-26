// Package fdw provides foreign data wrapper operations for PostgreSQL, including server management, table imports, and credential storage via vault integration.
package fdw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/sqlutil"
	"github.com/allyourbase/ayb/internal/vault"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	validIdentifierPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

// ForeignServer is a tracked FDW server.
type ForeignServer struct {
	Name      string            `json:"name"`
	FDWType   string            `json:"fdw_type"`
	Options   map[string]string `json:"options"`
	CreatedAt time.Time         `json:"created_at"`
}

// ForeignColumn is a column in a foreign table.
type ForeignColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ForeignTable describes an imported foreign table.
type ForeignTable struct {
	Schema     string            `json:"schema"`
	Name       string            `json:"name"`
	ServerName string            `json:"server_name"`
	Columns    []ForeignColumn   `json:"columns"`
	Options    map[string]string `json:"options"`
}

// UserMapping contains CURRENT_USER mapping credentials for postgres_fdw.
type UserMapping struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

// CreateServerOpts contains server creation parameters.
type CreateServerOpts struct {
	Name        string            `json:"name"`
	FDWType     string            `json:"fdw_type"`
	Options     map[string]string `json:"options"`
	UserMapping UserMapping       `json:"user_mapping"`
}

// ImportOpts controls IMPORT FOREIGN SCHEMA behavior.
type ImportOpts struct {
	RemoteSchema string   `json:"remote_schema"`
	LocalSchema  string   `json:"local_schema"`
	TableNames   []string `json:"table_names"`
}

// FDWService is the service contract for FDW operations.
type FDWService interface {
	CreateServer(ctx context.Context, opts CreateServerOpts) error
	ListServers(ctx context.Context) ([]ForeignServer, error)
	DropServer(ctx context.Context, name string, cascade bool) error
	ImportTables(ctx context.Context, serverName string, opts ImportOpts) ([]ForeignTable, error)
	ListForeignTables(ctx context.Context) ([]ForeignTable, error)
	DropForeignTable(ctx context.Context, schemaName, tableName string) error
}

// DB is the minimal database interface required by Service.
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
}

// VaultStore is the minimal vault interface required by Service.
type VaultStore interface {
	SetSecret(ctx context.Context, name string, value []byte) error
	GetSecret(ctx context.Context, name string) ([]byte, error)
	DeleteSecret(ctx context.Context, name string) error
}

// Service implements FDWService.
type Service struct {
	db         DB
	vaultStore VaultStore
}

// NewService creates a new FDW service.
func NewService(pool DB, vaultStore VaultStore) *Service {
	return &Service{db: pool, vaultStore: vaultStore}
}

// ValidateIdentifier validates PostgreSQL identifiers used by FDW service.
func ValidateIdentifier(name string) error {
	if name == "" {
		return errors.New("identifier must not be empty")
	}
	if len(name) > 63 {
		return fmt.Errorf("identifier %q exceeds PostgreSQL max length of 63 characters", name)
	}
	if !validIdentifierPattern.MatchString(name) {
		return fmt.Errorf("identifier %q contains invalid characters (must match ^[a-zA-Z_][a-zA-Z0-9_]*$)", name)
	}
	return nil
}

// ValidateFDWType validates supported FDW types.
func ValidateFDWType(fdwType string) error {
	switch fdwType {
	case "postgres_fdw", "file_fdw":
		return nil
	default:
		return fmt.Errorf("unsupported fdw type %q (supported: postgres_fdw, file_fdw)", fdwType)
	}
}

func (s *Service) ensureDB() error {
	if s == nil || s.db == nil {
		return errors.New("fdw service database is not configured")
	}
	return nil
}

func (s *Service) ensureVaultStore() error {
	if s == nil || s.vaultStore == nil {
		return errors.New("fdw service vault store is not configured")
	}
	return nil
}

// CreateServer creates a FDW server with the specified type and options, stores postgres_fdw credentials in vault, creates a user mapping for CURRENT_USER, and tracks the server in the _ayb_fdw_servers table.
func (s *Service) CreateServer(ctx context.Context, opts CreateServerOpts) error {
	if err := s.ensureDB(); err != nil {
		return err
	}
	if err := s.ensureVaultStore(); err != nil {
		return err
	}
	if err := ValidateIdentifier(opts.Name); err != nil {
		return err
	}
	if err := ValidateFDWType(opts.FDWType); err != nil {
		return err
	}

	optionKeys, err := requiredOptionKeys(opts)
	if err != nil {
		return err
	}

	secretName := ""
	secretStored := false

	err = s.withTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS `+sqlutil.QuoteIdent(opts.FDWType)); err != nil {
			return fmt.Errorf("create extension %q: %w", opts.FDWType, err)
		}

		serverSQL := fmt.Sprintf(
			`CREATE SERVER %s FOREIGN DATA WRAPPER %s OPTIONS (%s)`,
			sqlutil.QuoteIdent(opts.Name),
			sqlutil.QuoteIdent(opts.FDWType),
			buildOptionsClause(optionKeys, opts.Options),
		)
		if _, err := tx.Exec(ctx, serverSQL); err != nil {
			return fmt.Errorf("create foreign server %q: %w", opts.Name, err)
		}

		if opts.FDWType == "postgres_fdw" {
			if strings.TrimSpace(opts.UserMapping.User) == "" {
				return errors.New("postgres_fdw user mapping user is required")
			}
			if strings.TrimSpace(opts.UserMapping.Password) == "" {
				return errors.New("postgres_fdw user mapping password is required")
			}

			secretName = fdwPasswordSecretKey(opts.Name)
			if err := s.vaultStore.SetSecret(ctx, secretName, []byte(opts.UserMapping.Password)); err != nil {
				return fmt.Errorf("store fdw password secret: %w", err)
			}
			secretStored = true

			mappingSQL := fmt.Sprintf(
				`CREATE USER MAPPING FOR CURRENT_USER SERVER %s OPTIONS (user '%s', password '%s')`,
				sqlutil.QuoteIdent(opts.Name),
				escapeSQLLiteral(opts.UserMapping.User),
				escapeSQLLiteral(opts.UserMapping.Password),
			)
			if _, err := tx.Exec(ctx, mappingSQL); err != nil {
				return fmt.Errorf("create user mapping for server %q: %w", opts.Name, err)
			}
		}

		optionsJSON, err := json.Marshal(opts.Options)
		if err != nil {
			return fmt.Errorf("marshal server options: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO _ayb_fdw_servers (name, fdw_type, options)
			VALUES ($1, $2, $3::jsonb)
		`, opts.Name, opts.FDWType, optionsJSON); err != nil {
			return fmt.Errorf("insert fdw tracking row for %q: %w", opts.Name, err)
		}
		return nil
	})
	if err != nil {
		if secretStored {
			if deleteErr := s.vaultStore.DeleteSecret(ctx, secretName); deleteErr != nil {
				return errors.Join(err, fmt.Errorf("cleanup fdw password secret %q: %w", secretName, deleteErr))
			}
		}
		return err
	}
	return nil
}

// requiredOptionKeys validates that required options are present for the given FDW type and returns the ordered list of option keys to use in server creation.
func requiredOptionKeys(opts CreateServerOpts) ([]string, error) {
	switch opts.FDWType {
	case "postgres_fdw":
		for _, key := range []string{"host", "port", "dbname"} {
			if strings.TrimSpace(opts.Options[key]) == "" {
				return nil, fmt.Errorf("postgres_fdw option %q is required", key)
			}
		}
		return []string{"dbname", "host", "port"}, nil
	case "file_fdw":
		if strings.TrimSpace(opts.Options["filename"]) == "" {
			return nil, errors.New(`file_fdw option "filename" is required`)
		}
		return []string{"filename"}, nil
	default:
		return nil, fmt.Errorf("unsupported fdw type %q", opts.FDWType)
	}
}

func escapeSQLLiteral(value string) string {
	return strings.ReplaceAll(value, `'`, `''`)
}

func buildOptionsClause(keys []string, options map[string]string) string {
	ordered := append([]string(nil), keys...)
	sort.Strings(ordered)

	parts := make([]string, 0, len(ordered))
	for _, key := range ordered {
		parts = append(parts, fmt.Sprintf(`%s '%s'`, key, escapeSQLLiteral(options[key])))
	}
	return strings.Join(parts, ", ")
}

// ListServers returns all tracked FDW servers with their configuration retrieved from the _ayb_fdw_servers table.
func (s *Service) ListServers(ctx context.Context) ([]ForeignServer, error) {
	if err := s.ensureDB(); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(ctx, `
		SELECT name, fdw_type, options, created_at
		FROM _ayb_fdw_servers
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("query fdw servers: %w", err)
	}
	defer rows.Close()

	servers := make([]ForeignServer, 0)
	for rows.Next() {
		var server ForeignServer
		var optionsJSON []byte
		if err := rows.Scan(&server.Name, &server.FDWType, &optionsJSON, &server.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan fdw server row: %w", err)
		}
		server.Options = map[string]string{}
		if len(optionsJSON) > 0 {
			if err := json.Unmarshal(optionsJSON, &server.Options); err != nil {
				return nil, fmt.Errorf("decode options for server %q: %w", server.Name, err)
			}
		}
		servers = append(servers, server)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate fdw servers: %w", err)
	}
	return servers, nil
}

// ImportTables executes IMPORT FOREIGN SCHEMA to import tables from a remote server into the specified local schema, validating all identifiers and returning the imported foreign tables with their column definitions.
func (s *Service) ImportTables(ctx context.Context, serverName string, opts ImportOpts) ([]ForeignTable, error) {
	if err := s.ensureDB(); err != nil {
		return nil, err
	}
	if err := ValidateIdentifier(serverName); err != nil {
		return nil, err
	}

	remoteSchema := opts.RemoteSchema
	if strings.TrimSpace(remoteSchema) == "" {
		remoteSchema = "public"
	}
	localSchema := opts.LocalSchema
	if strings.TrimSpace(localSchema) == "" {
		localSchema = "public"
	}
	if err := ValidateIdentifier(remoteSchema); err != nil {
		return nil, fmt.Errorf("invalid remote schema: %w", err)
	}
	if err := ValidateIdentifier(localSchema); err != nil {
		return nil, fmt.Errorf("invalid local schema: %w", err)
	}

	for _, table := range opts.TableNames {
		if err := ValidateIdentifier(table); err != nil {
			return nil, fmt.Errorf("invalid table name in filter %q: %w", table, err)
		}
	}

	importSQL := buildImportSchemaSQL(serverName, remoteSchema, localSchema, opts.TableNames)
	if _, err := s.db.Exec(ctx, importSQL); err != nil {
		return nil, fmt.Errorf("import foreign schema from server %q: %w", serverName, err)
	}

	return s.listForeignTablesQuery(ctx, serverName, localSchema, opts.TableNames)
}

// buildImportSchemaSQL constructs an IMPORT FOREIGN SCHEMA SQL statement, optionally limiting the import to specified table names.
func buildImportSchemaSQL(serverName, remoteSchema, localSchema string, tableNames []string) string {
	var limitClause string
	if len(tableNames) > 0 {
		quotedTables := make([]string, 0, len(tableNames))
		for _, table := range tableNames {
			quotedTables = append(quotedTables, sqlutil.QuoteIdent(table))
		}
		limitClause = " LIMIT TO (" + strings.Join(quotedTables, ", ") + ")"
	}
	return fmt.Sprintf(
		`IMPORT FOREIGN SCHEMA %s%s FROM SERVER %s INTO %s`,
		sqlutil.QuoteIdent(remoteSchema),
		limitClause,
		sqlutil.QuoteIdent(serverName),
		sqlutil.QuoteIdent(localSchema),
	)
}

func (s *Service) ListForeignTables(ctx context.Context) ([]ForeignTable, error) {
	if err := s.ensureDB(); err != nil {
		return nil, err
	}
	return s.listForeignTablesQuery(ctx, "", "", nil)
}

// listForeignTablesQuery queries the information schema to retrieve foreign tables, optionally filtered by server name, schema name, or specific table names, and returns their column information.
func (s *Service) listForeignTablesQuery(ctx context.Context, serverName, schemaName string, onlyTables []string) ([]ForeignTable, error) {
	query := `
		SELECT
			ft.foreign_table_schema,
			ft.foreign_table_name,
			ft.foreign_server_name,
			c.column_name,
			c.udt_name
		FROM information_schema.foreign_tables ft
		JOIN information_schema.columns c
			ON c.table_schema = ft.foreign_table_schema
			AND c.table_name = ft.foreign_table_name
		WHERE ($1 = '' OR ft.foreign_server_name = $1)
			AND ($2 = '' OR ft.foreign_table_schema = $2)
		ORDER BY ft.foreign_table_schema, ft.foreign_table_name, c.ordinal_position
	`
	rows, err := s.db.Query(ctx, query, serverName, schemaName)
	if err != nil {
		return nil, fmt.Errorf("query foreign tables: %w", err)
	}
	defer rows.Close()

	filter := make(map[string]struct{}, len(onlyTables))
	for _, name := range onlyTables {
		filter[name] = struct{}{}
	}

	out := make([]ForeignTable, 0)
	indexByKey := make(map[string]int)
	for rows.Next() {
		var schema string
		var table string
		var server string
		var col string
		var typ string
		if err := rows.Scan(&schema, &table, &server, &col, &typ); err != nil {
			return nil, fmt.Errorf("scan foreign table row: %w", err)
		}

		if len(filter) > 0 {
			if _, ok := filter[table]; !ok {
				continue
			}
		}

		key := schema + "." + table
		idx, exists := indexByKey[key]
		if !exists {
			idx = len(out)
			indexByKey[key] = idx
			out = append(out, ForeignTable{
				Schema:     schema,
				Name:       table,
				ServerName: server,
				Columns:    []ForeignColumn{},
				Options:    map[string]string{},
			})
		}
		out[idx].Columns = append(out[idx].Columns, ForeignColumn{
			Name: col,
			Type: typ,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate foreign table rows: %w", err)
	}
	return out, nil
}

// DropForeignTable drops a foreign table by schema and name.
func (s *Service) DropForeignTable(ctx context.Context, schemaName, tableName string) error {
	if err := s.ensureDB(); err != nil {
		return err
	}
	if err := ValidateIdentifier(schemaName); err != nil {
		return fmt.Errorf("invalid schema name: %w", err)
	}
	if err := ValidateIdentifier(tableName); err != nil {
		return fmt.Errorf("invalid table name: %w", err)
	}

	sql := fmt.Sprintf(`DROP FOREIGN TABLE IF EXISTS %s`, sqlutil.QuoteQualifiedName(schemaName, tableName))
	if _, err := s.db.Exec(ctx, sql); err != nil {
		return fmt.Errorf("drop foreign table %s.%s: %w", schemaName, tableName, err)
	}
	return nil
}

// DropServer drops a FDW server and its user mappings, optionally cascading to dependent objects, and removes the associated vault secret containing the password.
func (s *Service) DropServer(ctx context.Context, name string, cascade bool) error {
	if err := s.ensureDB(); err != nil {
		return err
	}
	if err := s.ensureVaultStore(); err != nil {
		return err
	}
	if err := ValidateIdentifier(name); err != nil {
		return err
	}

	if err := s.withTx(ctx, func(tx pgx.Tx) error {
		dropMappingSQL := fmt.Sprintf(`DROP USER MAPPING IF EXISTS FOR CURRENT_USER SERVER %s`, sqlutil.QuoteIdent(name))
		if _, err := tx.Exec(ctx, dropMappingSQL); err != nil {
			return fmt.Errorf("drop user mapping for server %q: %w", name, err)
		}

		dropServerSQL := fmt.Sprintf(`DROP SERVER IF EXISTS %s`, sqlutil.QuoteIdent(name))
		if cascade {
			dropServerSQL += ` CASCADE`
		}
		if _, err := tx.Exec(ctx, dropServerSQL); err != nil {
			return fmt.Errorf("drop server %q: %w", name, err)
		}

		if _, err := tx.Exec(ctx, `DELETE FROM _ayb_fdw_servers WHERE name = $1`, name); err != nil {
			return fmt.Errorf("delete tracking row for server %q: %w", name, err)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := s.vaultStore.DeleteSecret(ctx, fdwPasswordSecretKey(name)); err != nil {
		if !errors.Is(err, vault.ErrSecretNotFound) {
			return fmt.Errorf("delete fdw password secret for %q: %w", name, err)
		}
	}
	return nil
}

// withTx begins a database transaction, executes the provided function, and commits on success or rolls back on error.
func (s *Service) withTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	committed = true
	return nil
}

func fdwPasswordSecretKey(serverName string) string {
	return fmt.Sprintf("fdw.%s.password", serverName)
}
