package schema

import (
	"sort"
	"time"
)

// SchemaCache is an immutable snapshot of the database schema.
// A new one is built on each reload and swapped in atomically.
type SchemaCache struct {
	Tables    map[string]*Table    `json:"tables"`    // key: "schema.table"
	Functions map[string]*Function `json:"functions"` // key: "schema.function"
	Enums     map[uint32]*EnumType `json:"-"`         // lookup by OID (internal)
	Schemas   []string             `json:"schemas"`
	BuiltAt   time.Time            `json:"builtAt"`
}

// TableByName returns a table by unqualified name, defaulting to the public schema.
// Falls back to scanning all schemas if not found in public.
func (sc *SchemaCache) TableByName(name string) *Table {
	if t, ok := sc.Tables["public."+name]; ok {
		return t
	}
	for _, t := range sc.Tables {
		if t.Name == name {
			return t
		}
	}
	return nil
}

// TableList returns all tables as a sorted slice.
func (sc *SchemaCache) TableList() []*Table {
	tables := make([]*Table, 0, len(sc.Tables))
	for _, t := range sc.Tables {
		tables = append(tables, t)
	}
	sort.Slice(tables, func(i, j int) bool {
		if tables[i].Schema != tables[j].Schema {
			return tables[i].Schema < tables[j].Schema
		}
		return tables[i].Name < tables[j].Name
	})
	return tables
}

// Table represents a database table, view, or materialized view.
type Table struct {
	Schema        string          `json:"schema"`
	Name          string          `json:"name"`
	Kind          string          `json:"kind"` // table, view, materialized_view, partitioned_table
	Comment       string          `json:"comment,omitempty"`
	Columns       []*Column       `json:"columns"`
	PrimaryKey    []string        `json:"primaryKey"`
	ForeignKeys   []*ForeignKey   `json:"foreignKeys,omitempty"`
	Indexes       []*Index        `json:"indexes,omitempty"`
	Relationships []*Relationship `json:"relationships,omitempty"`
}

// ColumnByName returns a column by name, or nil if not found.
func (t *Table) ColumnByName(name string) *Column {
	for _, c := range t.Columns {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// Column represents a database column.
type Column struct {
	Name         string   `json:"name"`
	Position     int      `json:"position"`
	TypeName     string   `json:"type"`
	TypeOID      uint32   `json:"-"`
	IsNullable   bool     `json:"nullable"`
	DefaultExpr  string   `json:"default,omitempty"`
	Comment      string   `json:"comment,omitempty"`
	IsPrimaryKey bool     `json:"isPrimaryKey"`
	IsJSON       bool     `json:"-"`
	IsEnum       bool     `json:"-"`
	IsArray      bool     `json:"-"`
	JSONType     string   `json:"jsonType"`
	EnumValues   []string `json:"enumValues,omitempty"`
}

// ForeignKey represents a foreign key constraint.
type ForeignKey struct {
	ConstraintName    string   `json:"constraintName"`
	Columns           []string `json:"columns"`
	ReferencedSchema  string   `json:"referencedSchema"`
	ReferencedTable   string   `json:"referencedTable"`
	ReferencedColumns []string `json:"referencedColumns"`
	OnUpdate          string   `json:"onUpdate,omitempty"`
	OnDelete          string   `json:"onDelete,omitempty"`
}

// Index represents a database index.
type Index struct {
	Name       string `json:"name"`
	IsUnique   bool   `json:"isUnique"`
	IsPrimary  bool   `json:"isPrimary"`
	Method     string `json:"method"`
	Definition string `json:"definition"`
}

// EnumType represents a PostgreSQL enum type.
type EnumType struct {
	Schema string   `json:"schema"`
	Name   string   `json:"name"`
	OID    uint32   `json:"-"`
	Values []string `json:"values"`
}

// Relationship represents a detected relationship between tables.
type Relationship struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"` // many-to-one, one-to-many
	FromSchema  string   `json:"fromSchema"`
	FromTable   string   `json:"fromTable"`
	FromColumns []string `json:"fromColumns"`
	ToSchema    string   `json:"toSchema"`
	ToTable     string   `json:"toTable"`
	ToColumns   []string `json:"toColumns"`
	FieldName   string   `json:"fieldName"`
}

// Function represents a PostgreSQL function discoverable via RPC.
type Function struct {
	Schema       string       `json:"schema"`
	Name         string       `json:"name"`
	Comment      string       `json:"comment,omitempty"`
	Parameters   []*FuncParam `json:"parameters"`
	ReturnType   string       `json:"returnType"` // e.g. "integer", "SETOF record", "void"
	ReturnsSet   bool         `json:"returnsSet"`
	IsVoid       bool         `json:"-"`
	HasOutParams bool         `json:"-"` // function has OUT parameters (use SELECT * FROM to unpack)
}

// FuncParam represents a parameter of a PostgreSQL function.
type FuncParam struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Position   int    `json:"position"`
	IsVariadic bool   `json:"isVariadic,omitempty"`
}

// ParamByName returns a parameter by name, or nil if not found.
func (f *Function) ParamByName(name string) *FuncParam {
	for _, p := range f.Parameters {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// FunctionByName returns a function by unqualified name, defaulting to the public schema.
func (sc *SchemaCache) FunctionByName(name string) *Function {
	if sc.Functions == nil {
		return nil
	}
	if f, ok := sc.Functions["public."+name]; ok {
		return f
	}
	for _, f := range sc.Functions {
		if f.Name == name {
			return f
		}
	}
	return nil
}

// relkindToString converts pg_class.relkind to a human-readable string.
func relkindToString(relkind string) string {
	switch relkind {
	case "r":
		return "table"
	case "v":
		return "view"
	case "m":
		return "materialized_view"
	case "p":
		return "partitioned_table"
	default:
		return "table"
	}
}

// fkActionToString converts pg_constraint FK action character to string.
func fkActionToString(action string) string {
	switch action {
	case "a":
		return "NO ACTION"
	case "r":
		return "RESTRICT"
	case "c":
		return "CASCADE"
	case "n":
		return "SET NULL"
	case "d":
		return "SET DEFAULT"
	default:
		return "NO ACTION"
	}
}
