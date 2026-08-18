package urlquery

import (
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// Decode fills the model's own query fields from the URL query, producing the
// query value WithQuery takes as its first argument.
//
// Decoding runs on a per-type field plan compiled once and cached. A field's
// URL name follows modelschema.QueryColumnName — the query tag wins over the
// json tag, which wins over the field name, snake-cased — the same rule that
// names filter columns, so a field's bare key and its operator-filter key
// always agree. A field carrying query:"-" is not decodable, and a field
// whose type no setter exists for (structs such as time.Time, slices, maps)
// is left out of the plan, so its key reports as unsupported.
//
// The keys Filters owns never reach the plan, so the "field[op]" bracket
// syntax and the bare framework timestamp keys stay with Filters. _format
// belongs to the export action and is dropped for every caller. _size is
// dropped when the model cannot carry it: a model embedding only Cursor
// accepts the parameter as its batch size, but the Size field lives in
// Pagination.
//
// Every remaining key must map to a decodable field, so a mistyped filter
// name is reported instead of silently widening the result set, and every
// offending key is reported at once. Value semantics match the streaming
// decoder this replaced: the last value of a repeated key wins, an empty
// value means "not filtering" and is skipped, and pointer fields are
// allocated on demand.
func Decode(q url.Values, m types.Model) error {
	plan := decodePlanOf(reflect.TypeOf(m))
	dst := reflect.ValueOf(m).Elem()

	var unsupported, invalid []string
	for key, values := range q {
		if isFilterQueryKey(key) || key == consts.QUERY_FORMAT {
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

// decodableField assigns one URL value into its struct field; assign returns
// the client-facing message when the value does not parse, empty on success.
type decodableField struct {
	assign func(dst reflect.Value, raw string) string
}

// decodePlans memoizes the decodable fields per model type.
var decodePlans sync.Map

// decodePlanOf returns the decodable fields of a model type keyed by URL
// parameter name, compiled once per type and cached. The type may be a struct
// or a pointer to one.
func decodePlanOf(typ reflect.Type) map[string]decodableField {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if cached, ok := decodePlans.Load(typ); ok {
		return cached.(map[string]decodableField) //nolint:errcheck
	}

	plan := make(map[string]decodableField)
	if typ != nil && typ.Kind() == reflect.Struct {
		collectDecodableFields(typ, nil, 0, plan, make(map[string]int))
	}
	decodePlans.Store(typ, plan)
	return plan
}

// collectDecodableFields walks a struct type, recursing into exported
// embedded structs, and records one decodable field per URL name. When two
// fields resolve to the same name, the shallower one wins, matching Go's own
// field promotion; at equal depth the first declaration wins.
func collectDecodableFields(typ reflect.Type, prefix []int, depth int, plan map[string]decodableField, depths map[string]int) {
	for i := range typ.NumField() {
		field := typ.Field(i)
		index := append(append([]int(nil), prefix...), i)

		if field.Anonymous && field.IsExported() && field.Type.Kind() == reflect.Struct {
			collectDecodableFields(field.Type, index, depth+1, plan, depths)
			continue
		}
		if !field.IsExported() {
			continue
		}
		// query:"-" opts the field out of bare-key decoding entirely; the name
		// resolution below would fall through to the json tag instead.
		if strings.TrimSpace(field.Tag.Get("query")) == "-" {
			continue
		}

		name := modelschema.QueryColumnName(field)
		setter, ok := fieldSetter(field.Type, name)
		if !ok {
			continue
		}
		if seen, exists := depths[name]; exists && seen <= depth {
			continue
		}
		depths[name] = depth
		plan[name] = decodableField{assign: makeAssign(index, field.Type, setter)}
	}
}

// makeAssign binds a setter to its field position, allocating pointer fields
// on demand so a decoded value always lands in a live element.
func makeAssign(index []int, typ reflect.Type, setter func(dst reflect.Value, raw string) string) func(dst reflect.Value, raw string) string {
	if typ.Kind() != reflect.Pointer {
		return func(dst reflect.Value, raw string) string {
			return setter(dst.FieldByIndex(index), raw)
		}
	}
	elem := typ.Elem()
	return func(dst reflect.Value, raw string) string {
		boxed := reflect.New(elem)
		if message := setter(boxed.Elem(), raw); message != "" {
			return message
		}
		dst.FieldByIndex(index).Set(boxed)
		return ""
	}
}

// fieldSetter returns the parse-and-set function for a field type, reported
// per kind so a named scalar type decodes like its underlying kind. A type
// without a setter is not decodable and stays out of the plan. Error wording
// matches the filter and cursor validators.
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
			parsed, err := strconv.ParseBool(raw)
			if err != nil {
				return fmt.Sprintf("invalid query parameter %q: expect a boolean value, got %q", key, raw)
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
