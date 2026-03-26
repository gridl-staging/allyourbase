// Package schemadiff Compares two database snapshots and emits an ordered sequence of schema changes (extensions, enums, tables) needed to transform the old schema to the new one.
package schemadiff

import "sort"

// ChangeType identifies the kind of schema change.
type ChangeType string

const (
	ChangeCreateTable         ChangeType = "CreateTable"
	ChangeDropTable           ChangeType = "DropTable"
	ChangeAddColumn           ChangeType = "AddColumn"
	ChangeDropColumn          ChangeType = "DropColumn"
	ChangeAlterColumnType     ChangeType = "AlterColumnType"
	ChangeAlterColumnDefault  ChangeType = "AlterColumnDefault"
	ChangeAlterColumnNullable ChangeType = "AlterColumnNullable"
	ChangeCreateIndex         ChangeType = "CreateIndex"
	ChangeDropIndex           ChangeType = "DropIndex"
	ChangeAddForeignKey       ChangeType = "AddForeignKey"
	ChangeDropForeignKey      ChangeType = "DropForeignKey"
	ChangeAddCheckConstraint  ChangeType = "AddCheckConstraint"
	ChangeDropCheckConstraint ChangeType = "DropCheckConstraint"
	ChangeCreateEnum          ChangeType = "CreateEnum"
	ChangeAlterEnumAddValue   ChangeType = "AlterEnumAddValue"
	ChangeAddRLSPolicy        ChangeType = "AddRLSPolicy"
	ChangeDropRLSPolicy       ChangeType = "DropRLSPolicy"
	ChangeEnableExtension     ChangeType = "EnableExtension"
	ChangeDisableExtension    ChangeType = "DisableExtension"
)

// Change is a single schema change with enough metadata to generate SQL.
// represents a single schema change with fields populated according to its Type, containing metadata needed to generate SQL migration statements.
type Change struct {
	Type ChangeType

	// Table-level fields.
	SchemaName string
	TableName  string
	TableKind  string

	// Column fields.
	ColumnName   string
	OldTypeName  string
	NewTypeName  string
	OldDefault   string
	NewDefault   string
	OldNullable  bool
	NewNullable  bool
	IsPrimaryKey bool
	AllColumns   []SnapColumn // used for CreateTable

	// Index fields.
	Index SnapIndex

	// Foreign key fields.
	ForeignKey SnapForeignKey

	// Check constraint fields.
	CheckConstraint SnapCheckConstraint

	// Enum fields.
	EnumSchema string
	EnumName   string
	EnumValues []string // all values for CreateEnum
	NewValue   string   // single new value for AlterEnumAddValue

	// RLS policy fields.
	RLSPolicy SnapRLSPolicy

	// Extension fields.
	ExtensionName    string
	ExtensionVersion string
}

// ChangeSet is an ordered list of schema changes.
type ChangeSet []Change

// Diff compares two snapshots and returns the changes needed to transform old into new.
// Changes are returned in a stable, topologically sensible order.
func Diff(old, new *Snapshot) ChangeSet {
	if old == nil {
		old = &Snapshot{}
	}
	if new == nil {
		new = &Snapshot{}
	}

	var cs ChangeSet

	// Extensions
	cs = append(cs, diffExtensions(old, new)...)
	// Enums (must come before tables that reference them)
	cs = append(cs, diffEnums(old, new)...)
	// Tables
	cs = append(cs, diffTables(old, new)...)

	return cs
}

// compares extensions between old and new snapshots, returning changes for newly enabled and disabled extensions.
func diffExtensions(old, new *Snapshot) []Change {
	var changes []Change

	newExts := make(map[string]SnapExtension, len(new.Extensions))
	for _, e := range new.Extensions {
		newExts[e.Name] = e
	}
	oldExts := make(map[string]SnapExtension, len(old.Extensions))
	for _, e := range old.Extensions {
		oldExts[e.Name] = e
	}

	// Enabled extensions (in new but not old)
	names := sortedStringKeys(newExts)
	for _, name := range names {
		if _, exists := oldExts[name]; !exists {
			e := newExts[name]
			changes = append(changes, Change{
				Type:             ChangeEnableExtension,
				ExtensionName:    e.Name,
				ExtensionVersion: e.Version,
			})
		}
	}

	// Disabled extensions (in old but not new)
	names = sortedStringKeys(oldExts)
	for _, name := range names {
		if _, exists := newExts[name]; !exists {
			e := oldExts[name]
			changes = append(changes, Change{
				Type:             ChangeDisableExtension,
				ExtensionName:    e.Name,
				ExtensionVersion: e.Version,
			})
		}
	}

	return changes
}

// compares enums between snapshots, emitting ChangeCreateEnum for new enums and ChangeAlterEnumAddValue for new values; does not emit drop changes due to PostgreSQL's CASCADE requirement.
func diffEnums(old, new *Snapshot) []Change {
	var changes []Change

	newEnumMap := make(map[string]SnapEnum, len(new.Enums))
	for _, e := range new.Enums {
		newEnumMap[e.Schema+"."+e.Name] = e
	}
	oldEnumMap := make(map[string]SnapEnum, len(old.Enums))
	for _, e := range old.Enums {
		oldEnumMap[e.Schema+"."+e.Name] = e
	}

	keys := sortedStringKeys(newEnumMap)
	for _, key := range keys {
		ne := newEnumMap[key]
		oe, exists := oldEnumMap[key]
		if !exists {
			// New enum.
			changes = append(changes, Change{
				Type:       ChangeCreateEnum,
				EnumSchema: ne.Schema,
				EnumName:   ne.Name,
				EnumValues: ne.Values,
			})
			continue
		}
		// Check for new values (enum values can only be added, not removed, in PG).
		oldVals := make(map[string]bool, len(oe.Values))
		for _, v := range oe.Values {
			oldVals[v] = true
		}
		for _, v := range ne.Values {
			if !oldVals[v] {
				changes = append(changes, Change{
					Type:       ChangeAlterEnumAddValue,
					EnumSchema: ne.Schema,
					EnumName:   ne.Name,
					NewValue:   v,
				})
			}
		}
	}
	// Note: we do not emit DropEnum — PostgreSQL requires CASCADE which is destructive.
	// Callers can handle that as needed.

	return changes
}

// compares tables between snapshots, handling created and dropped tables and delegating to helper functions for columns, indexes, foreign keys, check constraints, and RLS policies in existing tables.
func diffTables(old, new *Snapshot) []Change {
	var changes []Change

	newTableMap := make(map[string]SnapTable, len(new.Tables))
	for _, t := range new.Tables {
		newTableMap[t.FullName()] = t
	}
	oldTableMap := make(map[string]SnapTable, len(old.Tables))
	for _, t := range old.Tables {
		oldTableMap[t.FullName()] = t
	}

	// Created tables
	keys := sortedStringKeys(newTableMap)
	for _, key := range keys {
		nt := newTableMap[key]
		if _, exists := oldTableMap[key]; !exists {
			changes = append(changes, Change{
				Type:       ChangeCreateTable,
				SchemaName: nt.Schema,
				TableName:  nt.Name,
				TableKind:  nt.Kind,
				AllColumns: nt.Columns,
			})
			// Indexes on new table
			for _, idx := range nt.Indexes {
				if !idx.IsPrimary {
					changes = append(changes, Change{
						Type:       ChangeCreateIndex,
						SchemaName: nt.Schema,
						TableName:  nt.Name,
						Index:      idx,
					})
				}
			}
			// FKs on new table
			for _, fk := range nt.ForeignKeys {
				changes = append(changes, Change{
					Type:       ChangeAddForeignKey,
					SchemaName: nt.Schema,
					TableName:  nt.Name,
					ForeignKey: fk,
				})
			}
			// Check constraints on new table
			for _, cc := range nt.CheckConstraints {
				changes = append(changes, Change{
					Type:            ChangeAddCheckConstraint,
					SchemaName:      nt.Schema,
					TableName:       nt.Name,
					CheckConstraint: cc,
				})
			}
			// RLS policies on new table
			for _, pol := range nt.RLSPolicies {
				changes = append(changes, Change{
					Type:       ChangeAddRLSPolicy,
					SchemaName: nt.Schema,
					TableName:  nt.Name,
					RLSPolicy:  pol,
				})
			}
			continue
		}

		// Modified table — diff columns, indexes, FKs, check constraints, RLS.
		ot := oldTableMap[key]
		changes = append(changes, diffColumns(ot, nt)...)
		changes = append(changes, diffIndexes(ot, nt)...)
		changes = append(changes, diffForeignKeys(ot, nt)...)
		changes = append(changes, diffCheckConstraints(ot, nt)...)
		changes = append(changes, diffRLSPolicies(ot, nt)...)
	}

	// Dropped tables (in old but not new)
	keys = sortedStringKeys(oldTableMap)
	for _, key := range keys {
		if _, exists := newTableMap[key]; !exists {
			ot := oldTableMap[key]
			changes = append(changes, Change{
				Type:       ChangeDropTable,
				SchemaName: ot.Schema,
				TableName:  ot.Name,
			})
		}
	}

	return changes
}

// compares columns between tables, returning changes for added, dropped, and altered columns including type, default expression, and nullability modifications.
func diffColumns(old, new SnapTable) []Change {
	var changes []Change

	newColMap := make(map[string]SnapColumn, len(new.Columns))
	for _, c := range new.Columns {
		newColMap[c.Name] = c
	}
	oldColMap := make(map[string]SnapColumn, len(old.Columns))
	for _, c := range old.Columns {
		oldColMap[c.Name] = c
	}

	// Added columns
	for _, nc := range new.Columns {
		if _, exists := oldColMap[nc.Name]; !exists {
			changes = append(changes, Change{
				Type:         ChangeAddColumn,
				SchemaName:   new.Schema,
				TableName:    new.Name,
				ColumnName:   nc.Name,
				NewTypeName:  nc.TypeName,
				NewDefault:   nc.DefaultExpr,
				NewNullable:  nc.IsNullable,
				IsPrimaryKey: nc.IsPrimaryKey,
			})
		}
	}

	// Dropped columns
	for _, oc := range old.Columns {
		if _, exists := newColMap[oc.Name]; !exists {
			changes = append(changes, Change{
				Type:        ChangeDropColumn,
				SchemaName:  old.Schema,
				TableName:   old.Name,
				ColumnName:  oc.Name,
				OldTypeName: oc.TypeName,
			})
		}
	}

	// Altered columns
	for _, nc := range new.Columns {
		oc, exists := oldColMap[nc.Name]
		if !exists {
			continue
		}

		if oc.TypeName != nc.TypeName {
			changes = append(changes, Change{
				Type:        ChangeAlterColumnType,
				SchemaName:  new.Schema,
				TableName:   new.Name,
				ColumnName:  nc.Name,
				OldTypeName: oc.TypeName,
				NewTypeName: nc.TypeName,
			})
		}
		if oc.DefaultExpr != nc.DefaultExpr {
			changes = append(changes, Change{
				Type:       ChangeAlterColumnDefault,
				SchemaName: new.Schema,
				TableName:  new.Name,
				ColumnName: nc.Name,
				OldDefault: oc.DefaultExpr,
				NewDefault: nc.DefaultExpr,
			})
		}
		if oc.IsNullable != nc.IsNullable {
			changes = append(changes, Change{
				Type:        ChangeAlterColumnNullable,
				SchemaName:  new.Schema,
				TableName:   new.Name,
				ColumnName:  nc.Name,
				OldNullable: oc.IsNullable,
				NewNullable: nc.IsNullable,
			})
		}
	}

	return changes
}

// compares indexes between tables, returning changes for created and dropped indexes, excluding primary key indexes.
func diffIndexes(old, new SnapTable) []Change {
	var changes []Change

	newIdxMap := make(map[string]SnapIndex, len(new.Indexes))
	for _, idx := range new.Indexes {
		newIdxMap[idx.Name] = idx
	}
	oldIdxMap := make(map[string]SnapIndex, len(old.Indexes))
	for _, idx := range old.Indexes {
		oldIdxMap[idx.Name] = idx
	}

	// Created indexes
	names := sortedStringKeys(newIdxMap)
	for _, name := range names {
		if _, exists := oldIdxMap[name]; !exists {
			idx := newIdxMap[name]
			if !idx.IsPrimary {
				changes = append(changes, Change{
					Type:       ChangeCreateIndex,
					SchemaName: new.Schema,
					TableName:  new.Name,
					Index:      idx,
				})
			}
		}
	}

	// Dropped indexes
	names = sortedStringKeys(oldIdxMap)
	for _, name := range names {
		if _, exists := newIdxMap[name]; !exists {
			idx := oldIdxMap[name]
			if !idx.IsPrimary {
				changes = append(changes, Change{
					Type:       ChangeDropIndex,
					SchemaName: old.Schema,
					TableName:  old.Name,
					Index:      idx,
				})
			}
		}
	}

	return changes
}

// compares foreign keys between tables, returning changes for added and dropped constraints.
func diffForeignKeys(old, new SnapTable) []Change {
	var changes []Change

	newFKMap := make(map[string]SnapForeignKey, len(new.ForeignKeys))
	for _, fk := range new.ForeignKeys {
		newFKMap[fk.ConstraintName] = fk
	}
	oldFKMap := make(map[string]SnapForeignKey, len(old.ForeignKeys))
	for _, fk := range old.ForeignKeys {
		oldFKMap[fk.ConstraintName] = fk
	}

	names := sortedStringKeys(newFKMap)
	for _, name := range names {
		if _, exists := oldFKMap[name]; !exists {
			changes = append(changes, Change{
				Type:       ChangeAddForeignKey,
				SchemaName: new.Schema,
				TableName:  new.Name,
				ForeignKey: newFKMap[name],
			})
		}
	}

	names = sortedStringKeys(oldFKMap)
	for _, name := range names {
		if _, exists := newFKMap[name]; !exists {
			changes = append(changes, Change{
				Type:       ChangeDropForeignKey,
				SchemaName: old.Schema,
				TableName:  old.Name,
				ForeignKey: oldFKMap[name],
			})
		}
	}

	return changes
}

// compares check constraints between tables, returning changes for added and dropped constraints.
func diffCheckConstraints(old, new SnapTable) []Change {
	var changes []Change

	newCCMap := make(map[string]SnapCheckConstraint, len(new.CheckConstraints))
	for _, cc := range new.CheckConstraints {
		newCCMap[cc.Name] = cc
	}
	oldCCMap := make(map[string]SnapCheckConstraint, len(old.CheckConstraints))
	for _, cc := range old.CheckConstraints {
		oldCCMap[cc.Name] = cc
	}

	names := sortedStringKeys(newCCMap)
	for _, name := range names {
		if _, exists := oldCCMap[name]; !exists {
			changes = append(changes, Change{
				Type:            ChangeAddCheckConstraint,
				SchemaName:      new.Schema,
				TableName:       new.Name,
				CheckConstraint: newCCMap[name],
			})
		}
	}

	names = sortedStringKeys(oldCCMap)
	for _, name := range names {
		if _, exists := newCCMap[name]; !exists {
			changes = append(changes, Change{
				Type:            ChangeDropCheckConstraint,
				SchemaName:      old.Schema,
				TableName:       old.Name,
				CheckConstraint: oldCCMap[name],
			})
		}
	}

	return changes
}

// compares Row-Level Security policies between tables, returning changes for added and dropped policies.
func diffRLSPolicies(old, new SnapTable) []Change {
	var changes []Change

	newPolMap := make(map[string]SnapRLSPolicy, len(new.RLSPolicies))
	for _, pol := range new.RLSPolicies {
		newPolMap[pol.Name] = pol
	}
	oldPolMap := make(map[string]SnapRLSPolicy, len(old.RLSPolicies))
	for _, pol := range old.RLSPolicies {
		oldPolMap[pol.Name] = pol
	}

	names := sortedStringKeys(newPolMap)
	for _, name := range names {
		if _, exists := oldPolMap[name]; !exists {
			changes = append(changes, Change{
				Type:       ChangeAddRLSPolicy,
				SchemaName: new.Schema,
				TableName:  new.Name,
				RLSPolicy:  newPolMap[name],
			})
		}
	}

	names = sortedStringKeys(oldPolMap)
	for _, name := range names {
		if _, exists := newPolMap[name]; !exists {
			changes = append(changes, Change{
				Type:       ChangeDropRLSPolicy,
				SchemaName: old.Schema,
				TableName:  old.Name,
				RLSPolicy:  oldPolMap[name],
			})
		}
	}

	return changes
}

// sortedStringKeys returns sorted keys from a map[string]T.
func sortedStringKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
