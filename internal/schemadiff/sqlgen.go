// Package schemadiff This file generates idempotent DDL SQL statements to apply schema changes forward and reverse them.
package schemadiff

import (
	"fmt"
	"strings"

	"github.com/allyourbase/ayb/internal/sqlutil"
)

// GenerateUp produces idempotent DDL SQL to apply the given ChangeSet forward.
func GenerateUp(cs ChangeSet) string {
	var stmts []string
	for _, c := range cs {
		if sql := changeToUpSQL(c); sql != "" {
			stmts = append(stmts, sql)
		}
	}
	return strings.Join(stmts, "\n\n")
}

// GenerateDown produces idempotent DDL SQL to reverse the given ChangeSet.
func GenerateDown(cs ChangeSet) string {
	var stmts []string
	// Reverse order for down migrations.
	for i := len(cs) - 1; i >= 0; i-- {
		if sql := changeToDownSQL(cs[i]); sql != "" {
			stmts = append(stmts, sql)
		}
	}
	return strings.Join(stmts, "\n\n")
}

// changeToUpSQL returns the idempotent DDL to apply a schema change forward, dispatching on change type, or empty string if unrecognized.
func changeToUpSQL(c Change) string {
	switch c.Type {
	case ChangeCreateTable:
		return sqlCreateTable(c)
	case ChangeDropTable:
		return fmt.Sprintf("DROP TABLE IF EXISTS %s;", sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName))
	case ChangeAddColumn:
		return sqlAddColumn(c)
	case ChangeDropColumn:
		return fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s;",
			sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName), sqlutil.QuoteIdent(c.ColumnName))
	case ChangeAlterColumnType:
		return fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s USING %s::%s;",
			sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName),
			sqlutil.QuoteIdent(c.ColumnName),
			c.NewTypeName,
			sqlutil.QuoteIdent(c.ColumnName),
			c.NewTypeName,
		)
	case ChangeAlterColumnDefault:
		if c.NewDefault == "" {
			return fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT;",
				sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName), sqlutil.QuoteIdent(c.ColumnName))
		}
		return fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s;",
			sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName), sqlutil.QuoteIdent(c.ColumnName), c.NewDefault)
	case ChangeAlterColumnNullable:
		if c.NewNullable {
			return fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL;",
				sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName), sqlutil.QuoteIdent(c.ColumnName))
		}
		return fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;",
			sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName), sqlutil.QuoteIdent(c.ColumnName))
	case ChangeCreateIndex:
		return c.Index.Definition + ";"
	case ChangeDropIndex:
		return fmt.Sprintf("DROP INDEX IF EXISTS %s;",
			sqlutil.QuoteQualifiedName(c.SchemaName, c.Index.Name))
	case ChangeAddForeignKey:
		return sqlAddForeignKey(c)
	case ChangeDropForeignKey:
		return fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;",
			sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName),
			sqlutil.QuoteIdent(c.ForeignKey.ConstraintName))
	case ChangeAddCheckConstraint:
		return fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s CHECK (%s);",
			sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName),
			sqlutil.QuoteIdent(c.CheckConstraint.Name),
			c.CheckConstraint.Definition)
	case ChangeDropCheckConstraint:
		return fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;",
			sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName),
			sqlutil.QuoteIdent(c.CheckConstraint.Name))
	case ChangeCreateEnum:
		return sqlCreateEnum(c)
	case ChangeAlterEnumAddValue:
		return fmt.Sprintf("ALTER TYPE %s ADD VALUE IF NOT EXISTS %s;",
			sqlutil.QuoteQualifiedName(c.EnumSchema, c.EnumName), sqlStringLiteral(c.NewValue))
	case ChangeAddRLSPolicy:
		return sqlAddRLSPolicy(c)
	case ChangeDropRLSPolicy:
		return fmt.Sprintf("DROP POLICY IF EXISTS %s ON %s;",
			sqlutil.QuoteIdent(c.RLSPolicy.Name),
			sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName))
	case ChangeEnableExtension:
		return fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS %s;", sqlutil.QuoteIdent(c.ExtensionName))
	case ChangeDisableExtension:
		return fmt.Sprintf("DROP EXTENSION IF EXISTS %s;", sqlutil.QuoteIdent(c.ExtensionName))
	}
	return ""
}

// changeToDownSQL returns the idempotent DDL to reverse a schema change, dispatching on change type, or empty string if unrecognized.
func changeToDownSQL(c Change) string {
	switch c.Type {
	case ChangeCreateTable:
		return fmt.Sprintf("DROP TABLE IF EXISTS %s;", sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName))
	case ChangeDropTable:
		return sqlCreateTable(c)
	case ChangeAddColumn:
		return fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s;",
			sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName), sqlutil.QuoteIdent(c.ColumnName))
	case ChangeDropColumn:
		return sqlAddColumnWithType(c.SchemaName, c.TableName, c.ColumnName, c.OldTypeName, true, "")
	case ChangeAlterColumnType:
		return fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s USING %s::%s;",
			sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName),
			sqlutil.QuoteIdent(c.ColumnName),
			c.OldTypeName,
			sqlutil.QuoteIdent(c.ColumnName),
			c.OldTypeName,
		)
	case ChangeAlterColumnDefault:
		if c.OldDefault == "" {
			return fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT;",
				sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName), sqlutil.QuoteIdent(c.ColumnName))
		}
		return fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s;",
			sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName), sqlutil.QuoteIdent(c.ColumnName), c.OldDefault)
	case ChangeAlterColumnNullable:
		// Reverse: if we made it nullable, make it not nullable, and vice versa.
		if c.NewNullable {
			return fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;",
				sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName), sqlutil.QuoteIdent(c.ColumnName))
		}
		return fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL;",
			sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName), sqlutil.QuoteIdent(c.ColumnName))
	case ChangeCreateIndex:
		return fmt.Sprintf("DROP INDEX IF EXISTS %s;",
			sqlutil.QuoteQualifiedName(c.SchemaName, c.Index.Name))
	case ChangeDropIndex:
		return c.Index.Definition + ";"
	case ChangeAddForeignKey:
		return fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;",
			sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName),
			sqlutil.QuoteIdent(c.ForeignKey.ConstraintName))
	case ChangeDropForeignKey:
		return sqlAddForeignKey(c)
	case ChangeAddCheckConstraint:
		return fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;",
			sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName),
			sqlutil.QuoteIdent(c.CheckConstraint.Name))
	case ChangeDropCheckConstraint:
		return fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s CHECK (%s);",
			sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName),
			sqlutil.QuoteIdent(c.CheckConstraint.Name),
			c.CheckConstraint.Definition)
	case ChangeCreateEnum:
		return fmt.Sprintf("DROP TYPE IF EXISTS %s;",
			sqlutil.QuoteQualifiedName(c.EnumSchema, c.EnumName))
	case ChangeAlterEnumAddValue:
		// PostgreSQL cannot remove enum values; down migration is a no-op comment.
		return fmt.Sprintf("-- Cannot remove enum value %q from %s.%s in PostgreSQL",
			c.NewValue, c.EnumSchema, c.EnumName)
	case ChangeAddRLSPolicy:
		return fmt.Sprintf("DROP POLICY IF EXISTS %s ON %s;",
			sqlutil.QuoteIdent(c.RLSPolicy.Name),
			sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName))
	case ChangeDropRLSPolicy:
		return sqlAddRLSPolicy(c)
	case ChangeEnableExtension:
		return fmt.Sprintf("DROP EXTENSION IF EXISTS %s;", sqlutil.QuoteIdent(c.ExtensionName))
	case ChangeDisableExtension:
		return fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS %s;", sqlutil.QuoteIdent(c.ExtensionName))
	}
	return ""
}

// sqlCreateTable generates a CREATE TABLE statement from a ChangeCreateTable change.
func sqlCreateTable(c Change) string {
	var colDefs []string
	var pkCols []string

	for _, col := range c.AllColumns {
		def := fmt.Sprintf("  %s %s", sqlutil.QuoteIdent(col.Name), col.TypeName)
		if !col.IsNullable {
			def += " NOT NULL"
		}
		if col.DefaultExpr != "" {
			def += " DEFAULT " + col.DefaultExpr
		}
		colDefs = append(colDefs, def)
		if col.IsPrimaryKey {
			pkCols = append(pkCols, sqlutil.QuoteIdent(col.Name))
		}
	}

	if len(pkCols) > 0 {
		colDefs = append(colDefs, fmt.Sprintf("  PRIMARY KEY (%s)", strings.Join(pkCols, ", ")))
	}

	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n%s\n);",
		sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName),
		strings.Join(colDefs, ",\n"))
}

// sqlAddColumn generates an ADD COLUMN statement.
func sqlAddColumn(c Change) string {
	return sqlAddColumnWithType(c.SchemaName, c.TableName, c.ColumnName, c.NewTypeName, c.NewNullable, c.NewDefault)
}

func sqlAddColumnWithType(schemaName, tableName, colName, typeName string, nullable bool, defaultExpr string) string {
	def := fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s",
		sqlutil.QuoteQualifiedName(schemaName, tableName), sqlutil.QuoteIdent(colName), typeName)
	if !nullable {
		def += " NOT NULL"
	}
	if defaultExpr != "" {
		def += " DEFAULT " + defaultExpr
	}
	return def + ";"
}

// sqlAddForeignKey generates an ADD CONSTRAINT FOREIGN KEY statement.
func sqlAddForeignKey(c Change) string {
	fk := c.ForeignKey
	cols := sqlutil.QuoteIdentList(fk.Columns)
	refCols := sqlutil.QuoteIdentList(fk.ReferencedColumns)

	sql := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
		sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName),
		sqlutil.QuoteIdent(fk.ConstraintName),
		cols, sqlutil.QuoteQualifiedName(fk.ReferencedSchema, fk.ReferencedTable), refCols)

	if fk.OnDelete != "" && fk.OnDelete != "NO ACTION" {
		sql += " ON DELETE " + fk.OnDelete
	}
	if fk.OnUpdate != "" && fk.OnUpdate != "NO ACTION" {
		sql += " ON UPDATE " + fk.OnUpdate
	}
	return sql + ";"
}

// sqlCreateEnum generates a CREATE TYPE ... AS ENUM statement.
func sqlCreateEnum(c Change) string {
	vals := make([]string, len(c.EnumValues))
	for i, v := range c.EnumValues {
		vals[i] = sqlStringLiteral(v)
	}
	return fmt.Sprintf("CREATE TYPE %s AS ENUM (%s);",
		sqlutil.QuoteQualifiedName(c.EnumSchema, c.EnumName),
		strings.Join(vals, ", "))
}

// sqlAddRLSPolicy generates a CREATE POLICY statement.
func sqlAddRLSPolicy(c Change) string {
	pol := c.RLSPolicy
	perm := "PERMISSIVE"
	if !pol.Permissive {
		perm = "RESTRICTIVE"
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("CREATE POLICY %s ON %s",
		sqlutil.QuoteIdent(pol.Name), sqlutil.QuoteQualifiedName(c.SchemaName, c.TableName)))
	parts = append(parts, fmt.Sprintf("AS %s", perm))
	parts = append(parts, fmt.Sprintf("FOR %s", pol.Command))

	if len(pol.Roles) > 0 {
		parts = append(parts, fmt.Sprintf("TO %s", strings.Join(pol.Roles, ", ")))
	}
	if pol.UsingExpr != "" {
		parts = append(parts, fmt.Sprintf("USING (%s)", pol.UsingExpr))
	}
	if pol.WithCheckExpr != "" {
		parts = append(parts, fmt.Sprintf("WITH CHECK (%s)", pol.WithCheckExpr))
	}
	return strings.Join(parts, "\n  ") + ";"
}

// sqlStringLiteral wraps a string in single quotes, escaping embedded quotes.
func sqlStringLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
