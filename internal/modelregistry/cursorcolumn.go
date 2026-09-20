package modelregistry

import (
	"reflect"
	"sync"

	"github.com/hydroan/gst/internal/modelschema"
)

// identifyingColumnsCache memoizes the answer per model type; a model's
// columns and index declarations are fixed for the life of the binary.
var identifyingColumnsCache sync.Map

// IdentifyingColumns returns the database columns of m a single row can be
// told apart by: the primary key, and every column carrying a unique index
// of its own, as declared through Indexes.
//
// Cursor pagination reads them because a cursor is one boundary value: the
// page after it is every row whose column compares past it. On a column two
// rows can share, the rows on the boundary's own value are split between
// pages — the ones the first page had no room for are never read, and
// nothing reports it. A column no two rows share leaves no such gap.
//
// A model that declares no index and has no primary key yields an empty set,
// which refuses every cursor rather than letting one silently skip rows.
func IdentifyingColumns(m any) (map[string]struct{}, error) {
	typ := reflect.TypeOf(m)
	if cached, ok := identifyingColumnsCache.Load(typ); ok {
		return cached.(map[string]struct{}), nil //nolint:errcheck
	}

	columns, err := modelschema.Columns(typ)
	if err != nil {
		return nil, err
	}
	identifying := make(map[string]struct{}, 2)
	byGoName := make(map[string]string, len(columns))
	for _, col := range columns {
		byGoName[col.GoName] = col.DBName
		if col.PrimaryKey {
			identifying[col.DBName] = struct{}{}
		}
	}
	// The caller may hold nothing but the type — the database chain asks
	// about *new(M), a nil pointer — and a value method called on one
	// panics. A fresh instance answers the same declaration.
	declared := m
	if typ.Kind() == reflect.Pointer && reflect.ValueOf(m).IsNil() {
		declared = reflect.New(typ.Elem()).Interface()
	}
	if declarer, ok := asIndexer(declared); ok {
		for _, index := range declarer.Indexes() {
			// Only an index over one column identifies a row on its own: a
			// unique index over two says nothing about either column alone.
			if !index.Unique || len(index.Fields) != 1 {
				continue
			}
			if dbName, ok := byGoName[index.Fields[0]]; ok {
				identifying[dbName] = struct{}{}
			}
		}
	}

	identifyingColumnsCache.Store(typ, identifying)
	return identifying, nil
}
