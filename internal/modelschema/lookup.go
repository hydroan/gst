package modelschema

import (
	"reflect"
	"slices"
	"sync"

	gormschema "gorm.io/gorm/schema"
)

// The cached derivations below exist for the per-request paths: deriving the
// same lookup structure from the same type on every request would rebuild an
// identical map each time. Every one of them is a pure derivation of the
// cached columns, and a model type's columns are fixed at build time, so each
// is resolved once per type. The returned maps are shared across callers and
// are read-only.
//
// GoNameIndex and FilterableIndex report resolution failures as errors,
// because their consumers fail closed on a type they cannot resolve. The two
// column sets yield nil instead, and the nil is cached: their consumers treat
// an absent entry as a plain column by design, and a type cannot heal within
// one process, so retrying the parse on every request would only repeat the
// failure.

// byGoName indexes columns by their Go struct field name.
func byGoName(columns []Column) map[string]Column {
	indexed := make(map[string]Column, len(columns))
	for _, col := range columns {
		indexed[col.GoName] = col
	}
	return indexed
}

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

// columnSet reports the columns of a model type that keep selects, by
// database name, resolved once per type into cache. The type may be a struct
// or a pointer to one. On a type whose columns cannot be resolved it reports
// nil rather than an error: its consumers then render every column without
// the treatment the set selects for, which is the SQL every column renders
// on the dialects that need none.
func columnSet(cache *sync.Map, typ reflect.Type, keep func(Column) bool) map[string]struct{} {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if cached, ok := cache.Load(typ); ok {
		return cached.(map[string]struct{}) //nolint:errcheck
	}

	var set map[string]struct{}
	if columns, err := Columns(typ); err == nil {
		set = make(map[string]struct{})
		for _, c := range columns {
			if keep(c) {
				set[c.DBName] = struct{}{}
			}
		}
	}
	cache.Store(typ, set)
	return set
}

// timeColumnSetCache memoizes the time-typed column set per model type.
var timeColumnSetCache sync.Map

// TimeColumnSet reports the time-typed columns of a model type by database
// name, resolved once per type and cached; see columnSet. Its consumer is the
// filter renderer normalizing time comparisons, which every column renders
// without on the dialects that compare time natively.
func TimeColumnSet(typ reflect.Type) map[string]struct{} {
	return columnSet(&timeColumnSetCache, typ, func(c Column) bool { return ClassifyColumn(c.Type) == ColumnClassTime })
}

// jsonColumnSetCache memoizes the JSON-typed column set per model type.
var jsonColumnSetCache sync.Map

// JSONColumnSet reports the JSON-typed columns of a model type by database
// name, resolved once per type and cached; see columnSet. Its consumers are
// the text pattern cast in the filter renderer and the exact-match
// fail-closed rule in WithQuery.
func JSONColumnSet(typ reflect.Type) map[string]struct{} {
	return columnSet(&jsonColumnSetCache, typ, func(c Column) bool { return IsJSONType(c.Type) })
}

// numericColumnSetCache memoizes the numeric column set per model type.
var numericColumnSetCache sync.Map

// NumericColumnSet reports the numeric columns of a model type by database
// name, resolved once per type and cached; see columnSet. Its consumer is the
// text pattern cast in the filter renderer, which every column renders
// without on the dialects that read a number in a string context by
// themselves.
func NumericColumnSet(typ reflect.Type) map[string]struct{} {
	return columnSet(&numericColumnSetCache, typ, func(c Column) bool { return ClassifyColumn(c.Type) == ColumnClassNumeric })
}

// associationFieldsCache memoizes the association field paths per model type.
var associationFieldsCache sync.Map

// AssociationFields reports the struct field index paths of a model type's
// associations — the relationships its gorm schema holds for the type itself,
// a has-many or a belongs-to for one, declared for queries to preload —
// resolved once per type and cached, in field order; nil for a type whose
// schema cannot be resolved. Its consumer is the write path, which zeroes
// them: a write touches the model's own table alone.
func AssociationFields(typ reflect.Type) [][]int {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if cached, ok := associationFieldsCache.Load(typ); ok {
		return cached.([][]int) //nolint:errcheck
	}

	var paths [][]int
	if typ != nil && typ.Kind() == reflect.Struct {
		if parsed, err := gormschema.Parse(reflect.New(typ).Interface(), schemaCache, namer); err == nil {
			// gorm writes into these relationships under this lock when it
			// parses a type holding this one; the walk takes the lock too, or
			// a parse running beside it could hand it an entry half written.
			parsed.Relationships.Mux.RLock()
			paths = make([][]int, 0, len(parsed.Relationships.Relations))
			for _, rel := range parsed.Relationships.Relations {
				// Once gorm parses a type holding this one through a has-one
				// or a has-many, it registers that relationship here too,
				// under a hidden name, with the holder's field. gorm's own
				// preload of clause.Associations keeps the relationships
				// whose schema is the type's, and so does this list.
				if rel.Schema != parsed {
					continue
				}
				paths = append(paths, rel.Field.StructField.Index)
			}
			parsed.Relationships.Mux.RUnlock()
			slices.SortFunc(paths, slices.Compare)
		}
	}
	associationFieldsCache.Store(typ, paths)
	return paths
}
