package urlquery

import (
	"reflect"
	"slices"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/types"
)

// Parameter is one URL query parameter the list of a model takes, as the
// parsers of this package read it.
type Parameter struct {
	// Name is the key the URL spells: title, first.label for a column of a
	// struct the model embeds by name, _page for a framework parameter.
	Name string
	// Index is the path of the field the parameter fills or filters by, as
	// reflect.Value.FieldByIndex takes it.
	Index []int
	// Bare reports whether the URL takes the bare key, name=value: Decode
	// fills the field from it, or Filters reads it as an exact match, which
	// it does for the framework timestamps.
	Bare bool
	// Ops are the operators the URL takes in name[op]=value, in
	// types.FilterOps order: those the column's type takes a filter with on
	// a model embedding model.Query, none on another model or for a field no
	// column holds.
	Ops []types.FilterOp
}

// Parameters returns the URL query parameters the list of the model type typ
// takes, in the order of the fields declaring them: a bare key for every
// field Decode fills, the framework's own parameters among them, each with
// the operators Filters takes on its column, and the columns Filters alone
// reads, one no bare key decodes into under its operators alone and a
// framework timestamp under its bare key too. Three parameters the parsers
// take are left out: the _size a model paging by cursor alone takes, which
// is no field's (see Decode), and the export's _format and _limit, which
// Decode lets through for the export to read and document itself.
//
// For a model embedding model.Query beside Title string `query:"title"` and
// ExpiresAt time.Time `query:"expires_at"`, it returns title, bare and under
// every operator, and expires_at, under the comparisons and isnull alone,
// then the framework's parameters and the columns of model.Base.
func Parameters(typ reflect.Type) ([]Parameter, error) {
	plan, err := decodePlanOf(typ)
	if err != nil {
		return nil, err
	}
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == nil || typ.Kind() != reflect.Struct {
		return nil, nil
	}
	var columns map[string]modelschema.Column
	if modelregistry.IsQueryable(reflect.New(typ).Interface()) {
		if columns, err = modelschema.FilterableIndex(typ); err != nil {
			return nil, err
		}
	}

	params := make([]Parameter, 0, len(plan)+len(columns))
	for name, field := range plan {
		if isFilterQueryKey(name) {
			// Decode hands the key to Filters, which reads it below or not at all.
			continue
		}
		param := Parameter{Name: name, Index: field.index, Bare: true}
		if col, ok := columns[name]; ok {
			param.Ops, _ = filterOps(col)
		}
		params = append(params, param)
	}
	for name, col := range columns {
		if _, decoded := plan[name]; decoded && !isFilterQueryKey(name) {
			continue
		}
		_, bare := bareTimeFilterColumns[name]
		ops, _ := filterOps(col)
		params = append(params, Parameter{Name: name, Index: col.Index, Bare: bare, Ops: ops})
	}
	slices.SortFunc(params, func(a, b Parameter) int { return slices.Compare(a.Index, b.Index) })
	return params, nil
}
