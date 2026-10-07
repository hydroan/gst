package modelschema

import (
	"reflect"
	"sync"
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

// timeColumnSetCache memoizes the time-typed column set per model type.
var timeColumnSetCache sync.Map

// TimeColumnSet reports the time-typed columns of a model type by database
// name, resolved once per type and cached. The type may be a struct or a
// pointer to one.
//
// Its consumer is the filter renderer normalizing time comparisons; on a type
// whose columns cannot be resolved it renders without normalization, which is
// the exact SQL every column renders on the dialects that compare time
// natively — hence nil on failure rather than an error.
func TimeColumnSet(typ reflect.Type) map[string]struct{} {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if cached, ok := timeColumnSetCache.Load(typ); ok {
		return cached.(map[string]struct{}) //nolint:errcheck
	}

	var set map[string]struct{}
	if columns, err := Columns(typ); err == nil {
		set = make(map[string]struct{})
		for _, c := range columns {
			if ClassifyColumn(c.Type) == ColumnClassTime {
				set[c.DBName] = struct{}{}
			}
		}
	}
	timeColumnSetCache.Store(typ, set)
	return set
}

// jsonColumnSetCache memoizes the JSON-typed column set per model type.
var jsonColumnSetCache sync.Map

// JSONColumnSet reports the JSON-typed columns of a model type by database
// name, resolved once per type and cached. The type may be a struct or a
// pointer to one.
//
// Its consumers are the like-family cast in the filter renderer and the
// exact-match fail-closed rule in WithQuery; on a type whose columns cannot
// be resolved the columns render without the cast — hence nil on failure
// rather than an error.
func JSONColumnSet(typ reflect.Type) map[string]struct{} {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if cached, ok := jsonColumnSetCache.Load(typ); ok {
		return cached.(map[string]struct{}) //nolint:errcheck
	}

	var set map[string]struct{}
	if columns, err := Columns(typ); err == nil {
		set = make(map[string]struct{})
		for _, c := range columns {
			if IsJSONType(c.Type) {
				set[c.DBName] = struct{}{}
			}
		}
	}
	jsonColumnSetCache.Store(typ, set)
	return set
}
