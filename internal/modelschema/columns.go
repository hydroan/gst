// Package modelschema resolves the database columns of a model struct.
//
// It is the single place where a Go struct field is mapped to a database
// column: the mapping is delegated to gorm's own schema parser, so column
// names always match what gorm actually emits, including the column tag,
// the ignore markers, embedded struct lifting, and gorm's commonInitialisms
// handling (which a plain snake case conversion does not reproduce).
package modelschema

import (
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/stoewer/go-strcase"
	"gorm.io/gorm/schema"
)

// Column is one database column of a model.
//
// QueryName and DBName are deliberately separate: QueryName is the URL
// parameter clients filter by and stays a front-end contract, while DBName is
// whatever gorm writes into SQL. They are usually equal, and diverge when a
// model carries a column tag or a field name gorm renders differently.
type Column struct {
	GoName     string       // Go struct field name.
	QueryName  string       // URL parameter name: query tag, else json tag, else field name, in snake case.
	DBName     string       // Database column name resolved by gorm.
	Type       reflect.Type // Go type of the underlying struct field.
	Index      []int        // Struct field index path, usable with reflect.Value.FieldByIndex.
	Filterable bool         // Whether clients may filter on the column through the URL.
}

// ColumnClass is the aggregate capability a column type carries. It decides
// which reference gg gen writes for the column and which aggregate functions
// the string-name constructors accept for it, so both consumers read the same
// rule rather than each carrying a copy that could drift.
type ColumnClass int

const (
	// ColumnClassOther carries the aggregate functions that cannot be silently
	// wrong on any type: COUNT, COUNT DISTINCT, MIN and MAX.
	ColumnClassOther ColumnClass = iota
	// ColumnClassNumeric additionally carries SUM and AVG.
	ColumnClassNumeric
	// ColumnClassTime additionally carries time bucketing.
	ColumnClassTime
)

// timeType is the one type that gets time bucketing. A named type whose
// underlying type is time.Time is deliberately excluded: it is not a
// time.Time, so a reference typed on it would not compile.
var timeType = reflect.TypeFor[time.Time]()

// ClassifyColumn reports the aggregate capability of a column type. Pointers
// are dereferenced, since an aggregate reads the pointed-to value.
//
// Classification reads reflect.Kind only. Recognizing decimal types through
// driver.Valuer looks tempting, but uuid, JSON and enum types stored as text
// implement it too, and treating those as numeric would reintroduce exactly
// the failure the split exists to prevent: MySQL and SQLite answer SUM over a
// text column with 0 and a warning rather than an error, so the mistake
// reaches a report as a plausible wrong number instead of a failure. A decimal
// stored as a struct is therefore classified as other, and is summed through
// SumOf, whose column is checked against the model schema at build time.
func ClassifyColumn(typ reflect.Type) ColumnClass {
	if typ == nil {
		return ColumnClassOther
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == timeType {
		return ColumnClassTime
	}
	switch typ.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return ColumnClassNumeric
	default:
		return ColumnClassOther
	}
}

// IsJSONType reports whether a column type stores as a JSON document. The
// answer comes from the type itself through gorm's GormDataTypeInterface,
// which is how the gorm.io/datatypes types (JSON, JSONType, JSONSlice,
// JSONMap) and custom JSON wrappers declare their column type. Pointers are
// dereferenced, and the method is looked up on both receivers.
//
// The declared name is matched by its "json" prefix rather than by equality:
// the datatypes family does not spell one name (JSONMap declares "jsonmap",
// the others "json"), and a dialect-flavored wrapper may declare "jsonb".
//
// Consumers use it to keep text operators away from JSON columns where a
// dialect is strict about operand types; see the WithQuery JSON handling in
// the database package.
func IsJSONType(typ reflect.Type) bool {
	if typ == nil {
		return false
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	value := reflect.New(typ)
	decl, ok := value.Elem().Interface().(schema.GormDataTypeInterface)
	if !ok {
		if decl, ok = value.Interface().(schema.GormDataTypeInterface); !ok {
			return false
		}
	}
	return strings.HasPrefix(strings.ToLower(decl.GormDataType()), "json")
}

// schemaCache is the parse cache gorm expects; parsing a model type is only
// done once per process.
var schemaCache = &sync.Map{}

// namer matches the naming strategy the database drivers use. Every driver
// opens gorm with a default gorm.Config, so the zero NamingStrategy is the
// one that runs in production.
var namer = schema.NamingStrategy{}

// columnsCache memoizes the resolved columns per model type.
var columnsCache sync.Map

// TableName resolves the table a model type maps to, using the same gorm
// naming strategy the drivers run with.
//
// It exists because a model's own GetTableName is empty unless the model
// overrides it: the framework base returns "", and gorm then derives the name
// from the struct. Code that has to write the table name into SQL itself, such
// as the correlated-subquery filters, cannot rely on GetTableName alone and
// must not re-derive the name with its own snake case conversion either.
func TableName(typ reflect.Type) (string, error) {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == nil || typ.Kind() != reflect.Struct {
		return "", errors.Newf("modelschema: expect a struct type, got %v", typ)
	}
	parsed, err := schema.Parse(reflect.New(typ).Interface(), schemaCache, namer)
	if err != nil {
		return "", errors.Wrapf(err, "modelschema: parse %s", typ.String())
	}
	return parsed.Table, nil
}

// Columns returns every database column of a model struct type, sorted by
// database column name so repeated calls and generated code stay stable. The
// type may be a struct or a pointer to one.
func Columns(typ reflect.Type) ([]Column, error) {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == nil || typ.Kind() != reflect.Struct {
		return nil, errors.Newf("modelschema: expect a struct type, got %v", typ)
	}
	if cached, ok := columnsCache.Load(typ); ok {
		return cached.([]Column), nil //nolint:errcheck
	}

	parsed, err := schema.Parse(reflect.New(typ).Interface(), schemaCache, namer)
	if err != nil {
		return nil, errors.Wrapf(err, "modelschema: parse %s", typ.String())
	}

	// FieldsByDBName is gorm's deduplicated column set: a field declared on the
	// model shadows the one lifted from an embedded struct under the same column
	// name, and only the winner appears here. Iterating parsed.Fields instead
	// would yield both and produce two columns with one name.
	columns := make([]Column, 0, len(parsed.FieldsByDBName))
	for _, field := range parsed.FieldsByDBName {
		columns = append(columns, Column{
			GoName:    field.StructField.Name,
			QueryName: queryColumnName(field.StructField),
			DBName:    field.DBName,
			Type:      field.FieldType,
			Index:     field.StructField.Index,
			// A field hidden from JSON is framework-managed bookkeeping such
			// as the soft-delete timestamp: it stays a real column, but
			// clients must not filter on it.
			Filterable: strings.TrimSpace(field.StructField.Tag.Get("json")) != "-",
		})
	}
	sort.Slice(columns, func(i, j int) bool { return columns[i].DBName < columns[j].DBName })

	columnsCache.Store(typ, columns)
	return columns, nil
}

// byGoName indexes columns by their Go struct field name.
func byGoName(columns []Column) map[string]Column {
	indexed := make(map[string]Column, len(columns))
	for _, col := range columns {
		indexed[col.GoName] = col
	}
	return indexed
}

// The cached indexes below exist for the per-request paths: deriving the same
// index from the same type on every request would rebuild an identical map
// each time. Both indexes are pure derivations of the cached columns, and a
// model type's columns are fixed at build time, so each is resolved once per
// type. The returned maps are shared across callers and are read-only.

// goNameIndexCache memoizes the by-Go-name index per model type.
var goNameIndexCache sync.Map

// GoNameIndex returns the columns of a model type indexed by Go struct field
// name, resolved once per type and cached. The type may be a struct or a
// pointer to one.
func GoNameIndex(typ reflect.Type) (map[string]Column, error) {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if cached, ok := goNameIndexCache.Load(typ); ok {
		return cached.(map[string]Column), nil //nolint:errcheck
	}

	columns, err := Columns(typ)
	if err != nil {
		return nil, err
	}
	index := byGoName(columns)
	goNameIndexCache.Store(typ, index)
	return index, nil
}

// filterableIndexCache memoizes the filterable-column index per model type.
var filterableIndexCache sync.Map

// FilterableIndex returns the client-filterable columns of a model type
// indexed by the URL parameter name clients filter with, resolved once per
// type and cached. The type may be a struct or a pointer to one.
//
// Both the parameter name and the database column name come from the parsed
// columns, so a filter can never name a column gorm does not emit; fields
// hidden from JSON stay out, because clients must not filter on them.
func FilterableIndex(typ reflect.Type) (map[string]Column, error) {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if cached, ok := filterableIndexCache.Load(typ); ok {
		return cached.(map[string]Column), nil //nolint:errcheck
	}

	columns, err := Columns(typ)
	if err != nil {
		return nil, err
	}
	index := make(map[string]Column, len(columns))
	for _, col := range columns {
		if !col.Filterable {
			continue
		}
		index[col.QueryName] = col
	}
	filterableIndexCache.Store(typ, index)
	return index, nil
}

// queryColumnName resolves the URL parameter name of a struct field: the
// query tag wins over the json tag, which wins over the field name, and the
// result is converted to snake case. A "-" tag is skipped rather than used
// as a name, so the next source decides.
//
// This is the client-facing name only. It never decides the database column
// name, which comes from gorm (see Column).
func queryColumnName(field reflect.StructField) string {
	name := tagName(field, "query")
	if name == "" {
		name = tagName(field, "json")
	}
	if name == "" {
		name = field.Name
	}
	return strcase.SnakeCase(name)
}

// tagName reads a struct tag and strips its options suffix.
func tagName(field reflect.StructField, key string) string {
	value := strings.TrimSpace(field.Tag.Get(key))
	if idx := strings.IndexByte(value, ','); idx >= 0 {
		value = value[:idx]
	}
	if value == "-" {
		return ""
	}
	return strings.TrimSpace(value)
}
