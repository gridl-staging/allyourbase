// Package nhostmigrate Parses Hasura v3 table metadata to extract foreign keys and permissions for SQL migration generation.
package nhostmigrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/allyourbase/ayb/internal/migrate"
	"gopkg.in/yaml.v3"
)

type hasuraTableFile struct {
	Table               qualifiedTable       `json:"table"`
	ArrayRelationships  []hasuraRelationship `json:"array_relationships"`
	ObjectRelationships []hasuraRelationship `json:"object_relationships"`
	SelectPermissions   []hasuraPermission   `json:"select_permissions"`
	InsertPermissions   []hasuraPermission   `json:"insert_permissions"`
	UpdatePermissions   []hasuraPermission   `json:"update_permissions"`
	DeletePermissions   []hasuraPermission   `json:"delete_permissions"`
}

type qualifiedTable struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
}

type hasuraRelationship struct {
	Name  string          `json:"name"`
	Using json.RawMessage `json:"using"`
}

type hasuraPermission struct {
	Role string `json:"role"`
}

type metadataForeignKey struct {
	FromSchema string
	FromTable  string
	FromColumn string
	ToSchema   string
	ToTable    string
	ToColumn   string
}

func (f metadataForeignKey) Key() string {
	return strings.ToLower(fmt.Sprintf("%s.%s.%s->%s.%s.%s", f.FromSchema, f.FromTable, f.FromColumn, f.ToSchema, f.ToTable, f.ToColumn))
}

func (f metadataForeignKey) SQL() string {
	constraint := migrate.SanitizeIdentifier(fmt.Sprintf("fk_%s_%s_%s", f.FromTable, f.FromColumn, f.ToTable))
	return fmt.Sprintf(
		"ALTER TABLE ONLY %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s(%s)",
		migrate.QuoteQualifiedTable(f.FromSchema, f.FromTable),
		migrate.QuoteIdent(constraint),
		migrate.QuoteIdent(f.FromColumn),
		migrate.QuoteQualifiedTable(f.ToSchema, f.ToTable),
		migrate.QuoteIdent(f.ToColumn),
	)
}

type permissionAction struct {
	Role   string
	Action string
}

type foreignKeyByColumnResolver func(fromSchema, fromTable, fromColumn string) (metadataForeignKey, bool)

// PermissionActions returns all permission actions defined on the table, aggregating select, insert, update, and delete permissions across all roles.
func (t hasuraTableFile) PermissionActions() []permissionAction {
	out := make([]permissionAction, 0)
	for _, p := range t.SelectPermissions {
		if p.Role != "" {
			out = append(out, permissionAction{Role: p.Role, Action: "SELECT"})
		}
	}
	for _, p := range t.InsertPermissions {
		if p.Role != "" {
			out = append(out, permissionAction{Role: p.Role, Action: "INSERT"})
		}
	}
	for _, p := range t.UpdatePermissions {
		if p.Role != "" {
			out = append(out, permissionAction{Role: p.Role, Action: "UPDATE"})
		}
	}
	for _, p := range t.DeletePermissions {
		if p.Role != "" {
			out = append(out, permissionAction{Role: p.Role, Action: "DELETE"})
		}
	}
	return out
}

func (t hasuraTableFile) ForeignKeys(resolveByColumn foreignKeyByColumnResolver) []metadataForeignKey {
	keys := make([]metadataForeignKey, 0)
	for _, rel := range t.ArrayRelationships {
		fk, ok := parseArrayRelationship(t.Table, rel)
		if ok {
			keys = append(keys, fk)
		}
	}
	for _, rel := range t.ObjectRelationships {
		if fk, ok := parseObjectRelationshipWithResolver(t.Table, rel, resolveByColumn); ok {
			keys = append(keys, fk)
		}
	}
	return keys
}

// parseArrayRelationship extracts a foreign key definition from a Hasura array relationship, returning the metadataForeignKey and a success indicator.
func parseArrayRelationship(current qualifiedTable, rel hasuraRelationship) (metadataForeignKey, bool) {
	var payload struct {
		ForeignKeyConstraintOn struct {
			Table  qualifiedTable `json:"table"`
			Column string         `json:"column"`
		} `json:"foreign_key_constraint_on"`
	}
	if err := json.Unmarshal(rel.Using, &payload); err != nil {
		return metadataForeignKey{}, false
	}
	if payload.ForeignKeyConstraintOn.Table.Name == "" || payload.ForeignKeyConstraintOn.Column == "" {
		return metadataForeignKey{}, false
	}

	fromSchema := payload.ForeignKeyConstraintOn.Table.Schema
	if fromSchema == "" {
		fromSchema = defaultSchema(current.Schema)
	}

	return metadataForeignKey{
		FromSchema: fromSchema,
		FromTable:  payload.ForeignKeyConstraintOn.Table.Name,
		FromColumn: payload.ForeignKeyConstraintOn.Column,
		ToSchema:   defaultSchema(current.Schema),
		ToTable:    current.Name,
		ToColumn:   "id",
	}, true
}

// parseObjectRelationship extracts a foreign key definition from a Hasura object relationship, returning the metadataForeignKey and a success indicator.
func parseObjectRelationship(current qualifiedTable, rel hasuraRelationship) (metadataForeignKey, bool) {
	return parseObjectRelationshipWithResolver(current, rel, nil)
}

// parseObjectRelationshipWithResolver extracts a foreign key definition from a Hasura object relationship, attempting to parse manual configuration, column-based foreign key constraints, and table-based foreign key constraints, optionally using the provided resolver function for column-based lookups, and returns the metadataForeignKey and a success indicator.
func parseObjectRelationshipWithResolver(current qualifiedTable, rel hasuraRelationship, resolveByColumn foreignKeyByColumnResolver) (metadataForeignKey, bool) {
	currentSchema := defaultSchema(current.Schema)

	var manualPayload struct {
		ManualConfiguration struct {
			RemoteTable   qualifiedTable    `json:"remote_table"`
			ColumnMapping map[string]string `json:"column_mapping"`
		} `json:"manual_configuration"`
	}
	if err := json.Unmarshal(rel.Using, &manualPayload); err == nil {
		if manualPayload.ManualConfiguration.RemoteTable.Name != "" && len(manualPayload.ManualConfiguration.ColumnMapping) > 0 {
			keys := make([]string, 0, len(manualPayload.ManualConfiguration.ColumnMapping))
			for from := range manualPayload.ManualConfiguration.ColumnMapping {
				keys = append(keys, from)
			}
			sort.Strings(keys)
			fromCol := keys[0]
			toCol := manualPayload.ManualConfiguration.ColumnMapping[fromCol]
			if toCol == "" {
				return metadataForeignKey{}, false
			}

			remoteSchema := manualPayload.ManualConfiguration.RemoteTable.Schema
			if remoteSchema == "" {
				remoteSchema = currentSchema
			}

			return metadataForeignKey{
				FromSchema: currentSchema,
				FromTable:  current.Name,
				FromColumn: fromCol,
				ToSchema:   remoteSchema,
				ToTable:    manualPayload.ManualConfiguration.RemoteTable.Name,
				ToColumn:   toCol,
			}, true
		}
	}

	var fkColumnPayload struct {
		ForeignKeyConstraintOn string `json:"foreign_key_constraint_on"`
	}
	if err := json.Unmarshal(rel.Using, &fkColumnPayload); err == nil {
		fkColumn := strings.TrimSpace(fkColumnPayload.ForeignKeyConstraintOn)
		if fkColumn != "" && resolveByColumn != nil {
			return resolveByColumn(currentSchema, current.Name, fkColumn)
		}
	}

	var fkPayload struct {
		ForeignKeyConstraintOn struct {
			Table  qualifiedTable `json:"table"`
			Column string         `json:"column"`
		} `json:"foreign_key_constraint_on"`
	}
	if err := json.Unmarshal(rel.Using, &fkPayload); err != nil {
		return metadataForeignKey{}, false
	}
	if fkPayload.ForeignKeyConstraintOn.Table.Name == "" || fkPayload.ForeignKeyConstraintOn.Column == "" {
		return metadataForeignKey{}, false
	}

	remoteSchema := fkPayload.ForeignKeyConstraintOn.Table.Schema
	if remoteSchema == "" {
		remoteSchema = currentSchema
	}

	return metadataForeignKey{
		FromSchema: currentSchema,
		FromTable:  current.Name,
		FromColumn: fkPayload.ForeignKeyConstraintOn.Column,
		ToSchema:   remoteSchema,
		ToTable:    fkPayload.ForeignKeyConstraintOn.Table.Name,
		ToColumn:   "id",
	}, true
}

// loadHasuraV3TableFiles recursively discovers and parses all Hasura v3 table metadata files (JSON or YAML) from the databases/*/tables directory structure, defaulting schema to public if unspecified.
func loadHasuraV3TableFiles(metadataRoot string) ([]hasuraTableFile, error) {
	tablesRoot := filepath.Join(metadataRoot, "databases")
	if info, err := os.Stat(tablesRoot); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("hasura metadata v3 directory not found (expected %s); v2 metadata.json is not supported", tablesRoot)
	}

	tablesDirs, err := filepath.Glob(filepath.Join(metadataRoot, "databases", "*", "tables"))
	if err != nil {
		return nil, fmt.Errorf("listing hasura tables directories: %w", err)
	}

	files := make([]string, 0)
	for _, dir := range tablesDirs {
		walkErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".json" || ext == ".yaml" || ext == ".yml" {
				files = append(files, path)
			}
			return nil
		})
		if walkErr != nil {
			return nil, fmt.Errorf("walking hasura table files in %s: %w", dir, walkErr)
		}
	}
	sort.Strings(files)

	out := make([]hasuraTableFile, 0, len(files))
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading hasura table file %s: %w", path, err)
		}
		var tableFile hasuraTableFile
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".yaml", ".yml":
			var generic map[string]any
			if err := yaml.Unmarshal(b, &generic); err != nil {
				return nil, fmt.Errorf("parsing hasura table file %s: %w", path, err)
			}
			normalized, err := json.Marshal(generic)
			if err != nil {
				return nil, fmt.Errorf("normalizing hasura table file %s: %w", path, err)
			}
			if err := json.Unmarshal(normalized, &tableFile); err != nil {
				return nil, fmt.Errorf("decoding hasura table file %s: %w", path, err)
			}
		default:
			if err := json.Unmarshal(b, &tableFile); err != nil {
				return nil, fmt.Errorf("parsing hasura table file %s: %w", path, err)
			}
		}
		tableFile.Table.Schema = defaultSchema(tableFile.Table.Schema)
		if tableFile.Table.Name == "" {
			continue
		}
		out = append(out, tableFile)
	}

	return out, nil
}

func buildPolicySQL(schema, table, role, action string) string {
	return migrate.BuildAllowPolicySQL(schema, table, role, action)
}

func splitQualifiedTable(raw string) (schema, table string) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, ";")
	if idx := strings.Index(raw, "("); idx >= 0 {
		raw = raw[:idx]
	}
	raw = strings.Trim(raw, `"`)
	parts := strings.Split(raw, ".")
	if len(parts) == 2 {
		return strings.Trim(parts[0], `"`), strings.Trim(parts[1], `"`)
	}
	return "public", strings.Trim(raw, `"`)
}

func shouldSkipQualifiedTable(schema, table string) bool {
	schemaLower := strings.ToLower(schema)
	tableLower := strings.ToLower(table)
	if schemaLower == "information_schema" || schemaLower == "pg_catalog" || schemaLower == "hdb_catalog" {
		return true
	}
	return strings.HasPrefix(tableLower, "hdb_")
}

func qualifiedTableKey(schema, table string) string {
	return strings.ToLower(schema + "." + table)
}

func foreignKeySourceKey(schema, table, column string) string {
	return strings.ToLower(defaultSchema(schema) + "." + table + "." + column)
}

func defaultSchema(schema string) string {
	if strings.TrimSpace(schema) == "" {
		return "public"
	}
	return schema
}
