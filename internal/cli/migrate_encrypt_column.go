// Package cli Provides CLI functionality and transaction-based helpers to encrypt database column values using vault-managed encryption keys.
package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"

	"github.com/allyourbase/ayb/internal/api"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/sqlutil"
	"github.com/allyourbase/ayb/internal/vault"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

var (
	sqlIdentifierRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// runMigrateEncryptColumn orchestrates the encryption of a specified table column by loading migration configuration, validating the table and column names, initializing the vault with the master key, and delegating to runFieldEncryptionMigration to perform the encryption.
func runMigrateEncryptColumn(cmd *cobra.Command, args []string) error {
	cfg, err := loadMigrateConfig(cmd)
	if err != nil {
		return err
	}

	tableName := strings.TrimSpace(args[0])
	columnName := strings.TrimSpace(args[1])
	if tableName == "" || columnName == "" {
		return fmt.Errorf("table and column are required")
	}

	if err := validateSQLIdentifier(tableName); err != nil {
		return fmt.Errorf("invalid table name: %w", err)
	}
	if err := validateSQLIdentifier(columnName); err != nil {
		return fmt.Errorf("invalid column name: %w", err)
	}

	masterKey, err := vault.ResolveMasterKey(cfg.Vault.MasterKey)
	if err != nil {
		return fmt.Errorf("resolving vault master key: %w", err)
	}
	vaultEngine, err := vault.New(masterKey)
	if err != nil {
		return fmt.Errorf("initializing vault: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	pool, cleanup, err := connectForMigrate(cmd, cfg, logger)
	if err != nil {
		return err
	}
	defer cleanup()

	fieldEncryptor := api.NewFieldEncryptor(vaultEngine, []config.EncryptedColumnConfig{
		{
			Table:   tableName,
			Columns: []string{columnName},
		},
	})
	updated, err := runFieldEncryptionMigration(context.Background(), pool.DB(), tableName, columnName, fieldEncryptor)
	if err != nil {
		return err
	}

	fmt.Printf("Encrypted %d rows in %s.%s\n", updated, tableName, columnName)
	return nil
}

// runFieldEncryptionMigration performs the actual column encryption by creating a temporary encrypted column, reading all non-null rows, encrypting their values with the provided encrypter, dropping the original plaintext column, and renaming the temporary column to the original name. All operations occur within a single transaction.
func runFieldEncryptionMigration(ctx context.Context, db *pgxpool.Pool, tableName, columnName string, encrypter *api.FieldEncryptor) (int, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	pkColumns, err := fetchPrimaryKeyColumns(ctx, tx, tableName)
	if err != nil {
		return 0, fmt.Errorf("querying primary key columns: %w", err)
	}
	if len(pkColumns) == 0 {
		return 0, fmt.Errorf("table %q has no primary key", tableName)
	}

	quotedTable, err := quoteQualifiedIdentifier(tableName)
	if err != nil {
		return 0, err
	}
	encColumnName := columnName + "_enc"
	quotedEncColumn := sqlutil.QuoteIdent(encColumnName)
	quotedColumn := sqlutil.QuoteIdent(columnName)

	addColumnSQL := fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s BYTEA`, quotedTable, quotedEncColumn)
	if _, err := tx.Exec(ctx, addColumnSQL); err != nil {
		return 0, fmt.Errorf("adding temporary encrypted column: %w", err)
	}

	selectColumns := append(make([]string, 0, len(pkColumns)+1), pkColumns...)
	selectColumns = append(selectColumns, columnName)
	selectCols := make([]string, len(selectColumns))
	for i, col := range selectColumns {
		selectCols[i] = sqlutil.QuoteIdent(col)
	}
	selectSQL := fmt.Sprintf(`SELECT %s FROM %s WHERE %s IS NOT NULL`, strings.Join(selectCols, ", "), quotedTable, sqlutil.QuoteIdent(columnName))
	rows, err := tx.Query(ctx, selectSQL)
	if err != nil {
		return 0, fmt.Errorf("reading plaintext rows: %w", err)
	}
	defer rows.Close()

	conditions := make([]string, 0, len(pkColumns))
	for i, pk := range pkColumns {
		conditions = append(conditions, fmt.Sprintf("%s = $%d", sqlutil.QuoteIdent(pk), i+2))
	}
	updateSQL := fmt.Sprintf(`UPDATE %s SET %s = $1 WHERE %s`, quotedTable, quotedEncColumn, strings.Join(conditions, " AND "))

	var updated int
	for rows.Next() {
		values := make([]any, len(selectColumns))
		dest := make([]any, len(selectColumns))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return 0, fmt.Errorf("scanning source row: %w", err)
		}

		record := map[string]any{columnName: values[len(values)-1]}
		if err := encrypter.EncryptRecord(tableName, record); err != nil {
			return 0, fmt.Errorf("encrypting row value: %w", err)
		}
		ciphertext, ok := record[columnName].([]byte)
		if !ok {
			return 0, fmt.Errorf("expected encrypted bytes for %s.%s", tableName, columnName)
		}

		args := make([]any, len(pkColumns)+1)
		args[0] = ciphertext
		copy(args[1:], values[:len(pkColumns)])
		if _, err := tx.Exec(ctx, updateSQL, args...); err != nil {
			return 0, fmt.Errorf("updating encrypted row: %w", err)
		}
		updated++
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterating source rows: %w", err)
	}

	dropSQL := fmt.Sprintf(`ALTER TABLE %s DROP COLUMN %s`, quotedTable, quotedColumn)
	if _, err := tx.Exec(ctx, dropSQL); err != nil {
		return 0, fmt.Errorf("dropping plaintext column: %w", err)
	}
	renameSQL := fmt.Sprintf(`ALTER TABLE %s RENAME COLUMN %s TO %s`, quotedTable, quotedEncColumn, quotedColumn)
	if _, err := tx.Exec(ctx, renameSQL); err != nil {
		return 0, fmt.Errorf("renaming encrypted column: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	return updated, nil
}

// fetchPrimaryKeyColumns queries PostgreSQL system catalogs to retrieve the ordered list of column names comprising the primary key of the specified table.
func fetchPrimaryKeyColumns(ctx context.Context, tx pgx.Tx, tableName string) ([]string, error) {
	rows, err := tx.Query(ctx, `
SELECT a.attnum, a.attname
FROM pg_index i
JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
WHERE i.indrelid = $1::regclass
  AND i.indisprimary
ORDER BY array_position(i.indkey, a.attnum)
`, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var colName string
		if err := rows.Scan(new(int16), &colName); err != nil {
			return nil, err
		}
		columns = append(columns, colName)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func quoteQualifiedIdentifier(name string) (string, error) {
	if err := validateSQLIdentifier(name); err != nil {
		return "", err
	}
	parts := strings.Split(name, ".")
	for i, part := range parts {
		parts[i] = sqlutil.QuoteIdent(part)
	}
	return strings.Join(parts, "."), nil
}

func validateSQLIdentifier(value string) error {
	for _, part := range strings.Split(value, ".") {
		part = strings.TrimSpace(part)
		if part == "" || !sqlIdentifierRE.MatchString(part) {
			return fmt.Errorf("invalid identifier %q", part)
		}
	}
	return nil
}
