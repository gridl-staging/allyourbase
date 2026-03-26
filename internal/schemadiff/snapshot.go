package schemadiff

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Snapshot is a normalized, diffable representation of a database schema.
// It is serializable to JSON for baseline persistence.
type Snapshot struct {
	Tables     []SnapTable     `json:"tables"`
	Enums      []SnapEnum      `json:"enums"`
	Extensions []SnapExtension `json:"extensions"`
}

// SnapTable represents a table in the snapshot.
type SnapTable struct {
	Schema           string                `json:"schema"`
	Name             string                `json:"name"`
	Kind             string                `json:"kind"`
	Columns          []SnapColumn          `json:"columns"`
	Indexes          []SnapIndex           `json:"indexes,omitempty"`
	ForeignKeys      []SnapForeignKey      `json:"foreignKeys,omitempty"`
	CheckConstraints []SnapCheckConstraint `json:"checkConstraints,omitempty"`
	RLSEnabled       bool                  `json:"rlsEnabled"`
	RLSPolicies      []SnapRLSPolicy       `json:"rlsPolicies,omitempty"`
}

// FullName returns "schema.name".
func (t SnapTable) FullName() string {
	return t.Schema + "." + t.Name
}

// SnapColumn represents a column in the snapshot.
type SnapColumn struct {
	Name         string   `json:"name"`
	TypeName     string   `json:"type"`
	IsNullable   bool     `json:"nullable"`
	DefaultExpr  string   `json:"default,omitempty"`
	IsPrimaryKey bool     `json:"isPrimaryKey"`
	EnumValues   []string `json:"enumValues,omitempty"`
}

// SnapIndex represents an index in the snapshot.
type SnapIndex struct {
	Name       string `json:"name"`
	IsUnique   bool   `json:"isUnique"`
	IsPrimary  bool   `json:"isPrimary"`
	Method     string `json:"method"`
	Definition string `json:"definition"`
}

// SnapForeignKey represents a foreign key in the snapshot.
type SnapForeignKey struct {
	ConstraintName    string   `json:"constraintName"`
	Columns           []string `json:"columns"`
	ReferencedSchema  string   `json:"referencedSchema"`
	ReferencedTable   string   `json:"referencedTable"`
	ReferencedColumns []string `json:"referencedColumns"`
	OnUpdate          string   `json:"onUpdate,omitempty"`
	OnDelete          string   `json:"onDelete,omitempty"`
}

// SnapCheckConstraint represents a CHECK constraint on a table.
type SnapCheckConstraint struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
}

// SnapRLSPolicy represents an RLS policy in the snapshot.
type SnapRLSPolicy struct {
	Name          string   `json:"name"`
	Command       string   `json:"command"`
	Permissive    bool     `json:"permissive"`
	Roles         []string `json:"roles"`
	UsingExpr     string   `json:"usingExpr,omitempty"`
	WithCheckExpr string   `json:"withCheckExpr,omitempty"`
}

// SnapEnum represents an enum type in the snapshot.
type SnapEnum struct {
	Schema string   `json:"schema"`
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

// SnapExtension represents a database extension in the snapshot.
type SnapExtension struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// TakeSnapshot introspects the live database and returns a normalized Snapshot.
// It calls schema.BuildCache for tables/enums and also queries pg_extension directly.
func TakeSnapshot(ctx context.Context, pool *pgxpool.Pool) (*Snapshot, error) {
	cache, err := schema.BuildCache(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("building schema cache: %w", err)
	}

	extensions, err := loadExtensions(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("loading extensions: %w", err)
	}

	snap := FromSchemaCache(cache)
	snap.Extensions = extensions
	return snap, nil
}

// loadExtensions queries pg_extension for all installed extensions (excluding
// internal pg_catalog entries).
func loadExtensions(ctx context.Context, pool *pgxpool.Pool) ([]SnapExtension, error) {
	rows, err := pool.Query(ctx, `
		SELECT extname, extversion
		FROM pg_extension
		WHERE extname != 'plpgsql'
		ORDER BY extname`)
	if err != nil {
		return nil, fmt.Errorf("querying extensions: %w", err)
	}
	defer rows.Close()

	var exts []SnapExtension
	for rows.Next() {
		var e SnapExtension
		if err := rows.Scan(&e.Name, &e.Version); err != nil {
			return nil, fmt.Errorf("scanning extension: %w", err)
		}
		exts = append(exts, e)
	}
	return exts, rows.Err()
}

// FromSchemaCache converts a SchemaCache into a diffable Snapshot.
// Extensions are NOT populated here — use TakeSnapshot for a full snapshot.
func FromSchemaCache(cache *schema.SchemaCache) *Snapshot {
	snap := &Snapshot{}

	// Convert tables.
	for _, tbl := range cache.TableList() {
		st := SnapTable{
			Schema:     tbl.Schema,
			Name:       tbl.Name,
			Kind:       tbl.Kind,
			RLSEnabled: tbl.RLSEnabled,
		}

		for _, col := range tbl.Columns {
			sc := SnapColumn{
				Name:         col.Name,
				TypeName:     col.TypeName,
				IsNullable:   col.IsNullable,
				DefaultExpr:  col.DefaultExpr,
				IsPrimaryKey: col.IsPrimaryKey,
			}
			if len(col.EnumValues) > 0 {
				sc.EnumValues = make([]string, len(col.EnumValues))
				copy(sc.EnumValues, col.EnumValues)
			}
			st.Columns = append(st.Columns, sc)
		}

		for _, idx := range tbl.Indexes {
			st.Indexes = append(st.Indexes, SnapIndex{
				Name:       idx.Name,
				IsUnique:   idx.IsUnique,
				IsPrimary:  idx.IsPrimary,
				Method:     idx.Method,
				Definition: idx.Definition,
			})
		}
		sort.Slice(st.Indexes, func(i, j int) bool {
			return st.Indexes[i].Name < st.Indexes[j].Name
		})

		for _, fk := range tbl.ForeignKeys {
			st.ForeignKeys = append(st.ForeignKeys, SnapForeignKey{
				ConstraintName:    fk.ConstraintName,
				Columns:           fk.Columns,
				ReferencedSchema:  fk.ReferencedSchema,
				ReferencedTable:   fk.ReferencedTable,
				ReferencedColumns: fk.ReferencedColumns,
				OnUpdate:          fk.OnUpdate,
				OnDelete:          fk.OnDelete,
			})
		}
		sort.Slice(st.ForeignKeys, func(i, j int) bool {
			return st.ForeignKeys[i].ConstraintName < st.ForeignKeys[j].ConstraintName
		})

		for _, cc := range tbl.CheckConstraints {
			st.CheckConstraints = append(st.CheckConstraints, SnapCheckConstraint{
				Name:       cc.Name,
				Definition: cc.Definition,
			})
		}
		sort.Slice(st.CheckConstraints, func(i, j int) bool {
			return st.CheckConstraints[i].Name < st.CheckConstraints[j].Name
		})

		for _, pol := range tbl.RLSPolicies {
			st.RLSPolicies = append(st.RLSPolicies, SnapRLSPolicy{
				Name:          pol.Name,
				Command:       pol.Command,
				Permissive:    pol.Permissive,
				Roles:         pol.Roles,
				UsingExpr:     pol.UsingExpr,
				WithCheckExpr: pol.WithCheckExpr,
			})
		}
		sort.Slice(st.RLSPolicies, func(i, j int) bool {
			return st.RLSPolicies[i].Name < st.RLSPolicies[j].Name
		})

		snap.Tables = append(snap.Tables, st)
	}

	// Convert enums — SchemaCache stores enums by OID; normalize to sorted list.
	enumList := make([]SnapEnum, 0, len(cache.Enums))
	for _, e := range cache.Enums {
		enumList = append(enumList, SnapEnum{
			Schema: e.Schema,
			Name:   e.Name,
			Values: e.Values,
		})
	}
	sort.Slice(enumList, func(i, j int) bool {
		if enumList[i].Schema != enumList[j].Schema {
			return enumList[i].Schema < enumList[j].Schema
		}
		return enumList[i].Name < enumList[j].Name
	})
	snap.Enums = enumList

	return snap
}

// SaveSnapshot writes a Snapshot as JSON to the given path.
func SaveSnapshot(path string, snap *Snapshot) error {
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling snapshot: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing snapshot: %w", err)
	}
	return nil
}

// LoadSnapshot reads a Snapshot from JSON at the given path.
func LoadSnapshot(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading snapshot: %w", err)
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("unmarshaling snapshot: %w", err)
	}
	return &snap, nil
}

// tableByName returns a SnapTable by full name, or nil if not found.
func (s *Snapshot) tableByName(fullName string) *SnapTable {
	for i := range s.Tables {
		if s.Tables[i].FullName() == fullName {
			return &s.Tables[i]
		}
	}
	return nil
}

// enumByName returns a SnapEnum by "schema.name", or nil if not found.
func (s *Snapshot) enumByName(schemaName, name string) *SnapEnum {
	for i := range s.Enums {
		if s.Enums[i].Schema == schemaName && s.Enums[i].Name == name {
			return &s.Enums[i]
		}
	}
	return nil
}

// extensionByName returns a SnapExtension by name, or nil if not found.
func (s *Snapshot) extensionByName(name string) *SnapExtension {
	for i := range s.Extensions {
		if s.Extensions[i].Name == name {
			return &s.Extensions[i]
		}
	}
	return nil
}
