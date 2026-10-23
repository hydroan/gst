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
// because their consumers fail closed on a type they cannot resolve. The
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

// columnSet reports, by database name, the columns of a model type keep holds
// for, resolved once per type into cache. The type may be a struct or a
// pointer to one. On a type whose columns cannot be resolved it reports
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

// TimeColumnSet reports the time columns of a model type (see IsTimeColumn)
// by database name, resolved once per type and cached; see columnSet. Its
// consumers normalize time comparisons — the filter renderer, the WithQuery
// conditions and the cursor boundary — which every column renders without
// on the dialects that compare time natively.
func TimeColumnSet(typ reflect.Type) map[string]struct{} {
	return columnSet(&timeColumnSetCache, typ, IsTimeColumn)
}

// dateColumnSetCache memoizes the date column set per model type.
var dateColumnSetCache sync.Map

// DateColumnSet reports the date columns of a model type (see IsDateColumn)
// by database name, resolved once per type and cached; see columnSet. Its
// consumers bind the UTC day a value falls on where a date column is
// compared: the filter renderer, the WithQuery conditions and the cursor
// boundary.
func DateColumnSet(typ reflect.Type) map[string]struct{} {
	return columnSet(&dateColumnSetCache, typ, IsDateColumn)
}

// jsonColumnSetCache memoizes the JSON-typed column set per model type.
var jsonColumnSetCache sync.Map

// JSONColumnSet reports the JSON columns of a model type (see IsJSONColumn)
// by database name, resolved once per type and cached; see columnSet. Its
// consumers are the JSON handling of the filter renderer — the text pattern
// cast, the scalar check and the JSON contains operator — and the exact-match
// fail-closed rule in WithQuery.
func JSONColumnSet(typ reflect.Type) map[string]struct{} {
	return columnSet(&jsonColumnSetCache, typ, IsJSONColumn)
}

// numericColumnSetCache memoizes the numeric column set per model type.
var numericColumnSetCache sync.Map

// NumericColumnSet reports the numeric columns of a model type (see
// IsNumericColumn) by database name, resolved once per type and cached; see
// columnSet. Its consumer is the text pattern cast in the filter renderer,
// which every column renders without on the dialects that read a number in
// a string context by themselves.
func NumericColumnSet(typ reflect.Type) map[string]struct{} {
	return columnSet(&numericColumnSetCache, typ, IsNumericColumn)
}
