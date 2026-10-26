package urlquery

import (
	"fmt"
	"net/url"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/types"
)

// Decode fills the model's own query fields from the URL query, producing the
// query value WithQuery takes as its first argument.
//
// Decoding runs on a per-type field plan compiled once and cached. A field's
// URL name is its query tag (see modelschema.QueryColumnName), the same rule
// that names filter columns, so a field's bare key and its operator-filter
// key always agree; a field of a struct the model embeds by name decodes
// under its path, first.label, as its column is filtered. A field without a
// query tag, or tagged query:"-", is no URL parameter, and a field whose type
// no setter exists for (structs such as time.Time, slices, maps) is left out
// of the plan, so their keys report as unsupported. A column hidden from
// clients (json:"-") is left out too: it is not the client's to filter, by a
// bare key no more than by the operator keys Filters refuses for it. A field
// that is no column (gorm:"-") decodes like any other: it is the service's
// parameter, which the framework's own conditions leave alone.
//
// The keys Filters owns never reach the plan, so the "field[op]" bracket
// syntax and the bare framework timestamp keys stay with Filters. _format and
// _limit are the export action's own, which it takes off the query before
// decoding (see WithoutExportKeys), so any other action is refused them. _size
// is dropped when the model cannot carry it: a model embedding only Cursor
// accepts the parameter as its batch size, but the Size field lives in
// Pagination.
//
// Every remaining key must map to a decodable field, so a mistyped filter
// name is reported instead of silently widening the result set, and every
// offending key is reported at once. As for values, the last value of a
// repeated key wins, an empty value means "not filtering" and is skipped, and
// pointer fields are allocated on demand.
func Decode(q url.Values, m types.Model) error {
	plan, err := decodePlanOf(reflect.TypeOf(m))
	if err != nil {
		return err
	}
	dst := reflect.ValueOf(m).Elem()

	var unsupported, invalid []string
	for key, values := range q {
		if isFilterQueryKey(key) {
			continue
		}
		field, ok := plan[key]
		if !ok {
			if key == consts.QUERY_SIZE && !modelregistry.IsPaginatable(m) {
				continue
			}
			unsupported = append(unsupported, key)
			continue
		}
		if len(values) == 0 {
			continue
		}
		raw := values[len(values)-1]
		if raw == "" {
			continue
		}
		if message := field.assign(dst, raw); message != "" {
			invalid = append(invalid, message)
		}
	}
	if len(unsupported) == 0 && len(invalid) == 0 {
		return nil
	}
	// The uniform "invalid query parameter" prefix makes sorting the rendered
	// messages equivalent to sorting by key.
	sort.Strings(invalid)
	parts := make([]string, 0, len(invalid)+1)
	if len(unsupported) > 0 {
		parts = append(parts, unsupportedParameterMessage(unsupported))
	}
	parts = append(parts, invalid...)
	return errors.New(strings.Join(parts, "; "))
}

// decodableField is one field the URL fills: index is its path in the model,
// as reflect.Value.FieldByIndex takes it, and assign sets a URL value into it,
// returning the client-facing message when the value does not parse, empty
// on success.
type decodableField struct {
	index  []int
	assign func(dst reflect.Value, raw string) string
}

// decodePlans memoizes the decodable fields per model type.
var decodePlans sync.Map

// decodePlanOf returns the decodable fields of a model type keyed by URL
// parameter name, compiled once per type and cached. The type may be a struct
// or a pointer to one. The columns of the type come from modelschema, the
// one authority on which of them clients may filter; a type gorm cannot
// parse is an error, which no registered model is.
func decodePlanOf(typ reflect.Type) (map[string]decodableField, error) {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if cached, ok := decodePlans.Load(typ); ok {
		return cached.(map[string]decodableField), nil //nolint:errcheck
	}

	plan := make(map[string]decodableField)
	if typ != nil && typ.Kind() == reflect.Struct {
		columns, err := modelschema.Columns(typ)
		if err != nil {
			return nil, errors.Wrap(err, "urlquery: resolve the model's columns")
		}
		collectDecodableFields(typ, columns, plan)
		collectEmbeddedColumns(typ, columns, plan)
	}
	decodePlans.Store(typ, plan)
	return plan, nil
}

// collectDecodableFields records one decodable field per URL name among the
// fields walkQueryFields reaches in typ, skipping the fields of the columns
// clients may not filter on: a column hidden from JSON keeps its query tag
// out of the plan. When two fields resolve to the same name, the shallower
// one wins, matching Go's own field promotion; at equal depth the first
// declaration wins.
func collectDecodableFields(typ reflect.Type, columns []modelschema.Column, plan map[string]decodableField) {
	unfilterable := make(map[string]bool)
	for _, col := range columns {
		if !col.Filterable {
			unfilterable[fmt.Sprint(col.Index)] = true
		}
	}
	depths := make(map[string]int)
	walkQueryFields(typ, func(_ reflect.Type, field reflect.StructField, index []int, depth int) {
		if unfilterable[fmt.Sprint(index)] {
			return
		}
		name := modelschema.QueryColumnName(field)
		if name == "" {
			return
		}
		setter, ok := fieldSetter(field.Type, name)
		if !ok {
			return
		}
		if seen, exists := depths[name]; exists && seen <= depth {
			return
		}
		depths[name] = depth
		plan[name] = decodableField{index: index, assign: makeAssign(index, field.Type, setter)}
	})
}

// walkQueryFields calls visit for every exported field of the struct type
// typ, recursing into the exported structs it embeds, by value or through a
// pointer, whose fields Go promotes, in place of visiting them: owner is the
// struct declaring the field, index its path from typ, as
// reflect.Value.FieldByIndex takes it, and depth the number of structs
// embedded on the way. A field an outer one shadows is visited too, for the
// caller to resolve.
func walkQueryFields(typ reflect.Type, visit func(owner reflect.Type, field reflect.StructField, index []int, depth int)) {
	var walk func(owner reflect.Type, prefix []int, depth int)
	walk = func(owner reflect.Type, prefix []int, depth int) {
		for i := range owner.NumField() {
			field := owner.Field(i)
			index := append(slices.Clone(prefix), i)
			if field.Anonymous && field.IsExported() {
				embedded := field.Type
				if embedded.Kind() == reflect.Pointer {
					embedded = embedded.Elem()
				}
				if embedded.Kind() == reflect.Struct {
					walk(embedded, index, depth+1)
					continue
				}
			}
			if field.IsExported() {
				visit(owner, field, index, depth)
			}
		}
	}
	walk(typ, nil, 0)
}

// collectEmbeddedColumns adds to the plan the columns of the structs the
// model embeds by name, which gorm lifts into columns of the model's table
// and walkQueryFields does not look into: each under its URL name, the path
// of query tags the field takes (first.label, see modelschema.Column), so a
// bare key reaches the column the operator keys reach. A column clients may
// not filter on, without a URL name or hidden from them, and a field no
// setter exists for stay out, as the model's own do, and a name
// collectDecodableFields already took keeps its field.
func collectEmbeddedColumns(typ reflect.Type, columns []modelschema.Column, plan map[string]decodableField) {
	for _, col := range columns {
		if !col.Filterable || !strings.Contains(col.GoName, ".") {
			continue
		}
		if _, taken := plan[col.QueryName]; taken {
			continue
		}
		field := structFieldAlong(typ, col.Index)
		if setter, ok := fieldSetter(field.Type, col.QueryName); ok {
			plan[col.QueryName] = decodableField{index: col.Index, assign: makeAssign(col.Index, field.Type, setter)}
		}
	}
}

// structFieldAlong returns the struct field an index path reaches in typ,
// through the pointers on the way.
func structFieldAlong(typ reflect.Type, index []int) reflect.StructField {
	var field reflect.StructField
	for _, i := range index {
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		field = typ.Field(i)
		typ = field.Type
	}
	return field
}

// makeAssign binds a setter to its field position, allocating pointer fields
// on demand, a struct embedded through a pointer on the way included, so a
// decoded value always lands in a live element.
func makeAssign(index []int, typ reflect.Type, setter func(dst reflect.Value, raw string) string) func(dst reflect.Value, raw string) string {
	if typ.Kind() != reflect.Pointer {
		return func(dst reflect.Value, raw string) string {
			return setter(fieldAlong(dst, index), raw)
		}
	}
	elem := typ.Elem()
	return func(dst reflect.Value, raw string) string {
		boxed := reflect.New(elem)
		if message := setter(boxed.Elem(), raw); message != "" {
			return message
		}
		fieldAlong(dst, index).Set(boxed)
		return ""
	}
}

// fieldAlong is dst.FieldByIndex(index) with the nil pointers on the way
// allocated.
func fieldAlong(dst reflect.Value, index []int) reflect.Value {
	for _, i := range index {
		for dst.Kind() == reflect.Pointer {
			if dst.IsNil() {
				dst.Set(reflect.New(dst.Type().Elem()))
			}
			dst = dst.Elem()
		}
		dst = dst.Field(i)
	}
	return dst
}

// fieldSetter returns the parse-and-set function for a field type, reported
// per kind so a named scalar type decodes like its underlying kind. A type
// without a setter is not decodable and stays out of the plan. Error wording
// matches the filter and cursor validators, and a boolean is read by the
// modelschema.ParseBool they read one by.
func fieldSetter(typ reflect.Type, key string) (func(dst reflect.Value, raw string) string, bool) {
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.String:
		return func(dst reflect.Value, raw string) string {
			dst.SetString(raw)
			return ""
		}, true
	case reflect.Bool:
		return func(dst reflect.Value, raw string) string {
			parsed, err := modelschema.ParseBool(raw)
			if err != nil {
				return fmt.Sprintf("invalid query parameter %q: %v", key, err)
			}
			dst.SetBool(parsed)
			return ""
		}, true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		bits := typ.Bits()
		return func(dst reflect.Value, raw string) string {
			parsed, err := strconv.ParseInt(raw, 10, bits)
			if err != nil {
				return fmt.Sprintf("invalid query parameter %q: expect a numeric value, got %q", key, raw)
			}
			dst.SetInt(parsed)
			return ""
		}, true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		bits := typ.Bits()
		return func(dst reflect.Value, raw string) string {
			parsed, err := strconv.ParseUint(raw, 10, bits)
			if err != nil {
				return fmt.Sprintf("invalid query parameter %q: expect a numeric value, got %q", key, raw)
			}
			dst.SetUint(parsed)
			return ""
		}, true
	case reflect.Float32, reflect.Float64:
		bits := typ.Bits()
		return func(dst reflect.Value, raw string) string {
			parsed, err := strconv.ParseFloat(raw, bits)
			if err != nil {
				return fmt.Sprintf("invalid query parameter %q: expect a numeric value, got %q", key, raw)
			}
			dst.SetFloat(parsed)
			return ""
		}, true
	default:
		return nil, false
	}
}
