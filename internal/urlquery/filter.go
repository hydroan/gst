package urlquery

import (
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/types"
	"github.com/stoewer/go-strcase"
)

// Filters extracts field-level operator filters from URL query keys of the
// form "field[op]=value", e.g. "age[gt]=20" or "remark[like]=hello", plus the
// bare framework timestamp keys ("created_at", "updated_at"), which act as
// exact-match (eq) filters. The field token must resolve (after snake case
// normalization) to a queryable column of the model, and op must be a known
// types.FilterOp; anything else is rejected so a mistyped filter can never
// silently widen the result set. Empty values mean "not filtering" and are
// skipped. Filters require the model to embed model.Query, and the returned
// conditions are sorted by key for deterministic SQL.
func Filters(q url.Values, m types.Model) ([]types.Filter, error) {
	keys := make([]string, 0)
	for key := range q {
		if isFilterQueryKey(key) {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil, nil
	}
	if !modelregistry.IsQueryable(m) {
		return nil, UnsupportedParameterError(keys)
	}
	sort.Strings(keys)

	columns, err := filterableColumns(m)
	if err != nil {
		return nil, err
	}
	conds := make([]types.Filter, 0, len(keys))
	for _, key := range keys {
		var field string
		var op types.FilterOp
		if _, bare := bareTimeFilterColumns[key]; bare && !isFilterKey(key) {
			// The bare framework timestamp key is an exact-match filter.
			field, op = key, types.FilterOpEq
		} else {
			var opToken string
			var ok bool
			field, opToken, ok = splitFilterKey(key)
			if !ok {
				return nil, errors.Newf("invalid field filter %q: expect \"field[op]=value\"", key)
			}
			if op, ok = types.ParseFilterOp(opToken); !ok {
				return nil, errors.Newf("invalid field filter %q: unknown operator %q", key, opToken)
			}
		}
		col, ok := columns[strcase.SnakeCase(field)]
		if !ok {
			return nil, errors.Newf("invalid field filter %q: unknown field %q", key, field)
		}
		raw := q[key][0]
		if len(raw) == 0 {
			continue
		}
		value, err := normalizeFilterValue(col.Type, op, raw)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid field filter %q", key)
		}
		conds = append(conds, types.Filter{Column: col.DBName, Op: op, Value: value})
	}
	if len(conds) == 0 {
		return nil, nil
	}
	return conds, nil
}

// bareTimeFilterColumns are the framework-managed Base/AutoBase timestamp
// columns. They carry query:"-" on the model, so their bare keys are not
// schema-decodable and are handled by Filters instead: the bare key is an
// exact-match (eq) filter, keeping the "bare name filters exactly" contract
// uniform across every documented parameter.
var bareTimeFilterColumns = map[string]struct{}{
	"created_at": {},
	"updated_at": {},
}

// isFilterKey reports whether a query key carries a field-level
// operator filter ("field[op]"). Keys in the framework "_" namespace never
// count: an underscore key with brackets stays a framework parameter and is
// rejected by the regular query decoding path.
func isFilterKey(key string) bool {
	return !strings.HasPrefix(key, "_") && strings.ContainsRune(key, '[')
}

// isFilterQueryKey reports whether Filters owns the key: either an operator
// filter key or a bare framework timestamp key.
func isFilterQueryKey(key string) bool {
	if isFilterKey(key) {
		return true
	}
	_, ok := bareTimeFilterColumns[key]
	return ok
}

// splitFilterKey splits "field[op]" into its field and operator
// tokens, reporting whether the key has exactly that shape.
func splitFilterKey(key string) (field, op string, ok bool) {
	open := strings.IndexByte(key, '[')
	if open <= 0 || !strings.HasSuffix(key, "]") {
		return "", "", false
	}
	field, op = key[:open], key[open+1:len(key)-1]
	if len(op) == 0 || strings.ContainsAny(field, "[]") || strings.ContainsAny(op, "[]") {
		return "", "", false
	}
	return field, op, true
}

// filterableColumns returns the columns a client may filter on, keyed by the
// URL parameter name. Both the parameter name and the database column name
// come from modelschema, the single place that maps struct fields to columns,
// so a filter can never name a column gorm does not emit.
func filterableColumns(m types.Model) (map[string]modelschema.Column, error) {
	parsed, err := modelschema.Columns(reflect.TypeOf(m))
	if err != nil {
		return nil, err
	}
	columns := make(map[string]modelschema.Column, len(parsed))
	for _, col := range parsed {
		if !col.Filterable {
			continue
		}
		columns[col.QueryName] = col
	}
	return columns, nil
}

// timeType is the reflect type time-typed columns are recognized by.
var timeType = reflect.TypeFor[time.Time]()

// normalizeFilterValue validates a filter value against the
// column's Go type and rewrites it into the canonical typed value bound to
// the statement, so a malformed value is rejected with an error instead of
// being passed to the database where implicit conversion could silently
// match the wrong rows.
//
//   - isnull applies to any column and requires a boolean value, carried as
//     a bool; it is handled before the type dispatch below.
//   - time columns accept the comparison operators only; the value must be
//     RFC 3339 (see parseQueryTime) and travels as the UTC wall clock in
//     FilterTimeLayout. The canonical string form is kept on purpose: binding
//     time.Time would let the driver re-render the value in its own location,
//     while the string pins the wall-clock time the parser resolved.
//   - bool columns accept eq/ne with a boolean value, carried as a bool.
//   - numeric columns require numeric values; in/notin validate every
//     comma-separated member.
//   - in/notin values split on commas here, so the members travel as a real
//     slice: the URL list encoding never reaches the database layer.
//   - string and other scalar values pass through unchanged.
func normalizeFilterValue(columnTyp reflect.Type, op types.FilterOp, value string) (any, error) {
	// isnull is the only operator whose value type is independent of the
	// column type: it always carries a boolean and applies to any nullable
	// column, including time columns the comparison gating below would block.
	if op == types.FilterOpIsNull {
		b, err := strconv.ParseBool(value)
		if err != nil {
			return nil, errors.Newf("isnull expects a boolean value, got %q", value)
		}
		return b, nil
	}
	switch {
	case columnTyp == timeType:
		switch op {
		case types.FilterOpEq, types.FilterOpNe, types.FilterOpGt, types.FilterOpGte, types.FilterOpLt, types.FilterOpLte:
			t, err := parseQueryTime(value)
			if err != nil {
				return nil, err
			}
			// The bound travels as the UTC wall clock, which is the one wall
			// clock the framework stores on every dialect; see FilterTimeLayout.
			return t.In(time.UTC).Format(types.FilterTimeLayout), nil
		default:
			return nil, errors.Newf("operator %q is not supported on a time field", op)
		}
	case columnTyp.Kind() == reflect.Bool:
		switch op {
		case types.FilterOpEq, types.FilterOpNe:
			b, err := strconv.ParseBool(value)
			if err != nil {
				return nil, errors.Newf("expect a boolean value, got %q", value)
			}
			return b, nil
		default:
			return nil, errors.Newf("operator %q is not supported on a bool field", op)
		}
	case isNumericKind(columnTyp.Kind()):
		switch op {
		case types.FilterOpIn, types.FilterOpNotIn:
			items := strings.Split(value, ",")
			for _, item := range items {
				if err := validateNumericValue(columnTyp.Kind(), item); err != nil {
					return nil, err
				}
			}
			return items, nil
		case types.FilterOpLike, types.FilterOpNotLike, types.FilterOpStartsWith, types.FilterOpEndsWith:
			// Substring matching relies on the database's string rendering of
			// the number; the pattern itself is not numeric.
			return value, nil
		default:
			if err := validateNumericValue(columnTyp.Kind(), value); err != nil {
				return nil, err
			}
			return value, nil
		}
	default:
		switch op {
		case types.FilterOpIn, types.FilterOpNotIn:
			// The comma split moves the URL list encoding out of the database
			// layer: from here on the members travel as a real slice.
			return strings.Split(value, ","), nil
		default:
			return value, nil
		}
	}
}

func isNumericKind(kind reflect.Kind) bool {
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

func validateNumericValue(kind reflect.Kind, value string) error {
	var err error
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		_, err = strconv.ParseInt(value, 10, 64)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		_, err = strconv.ParseUint(value, 10, 64)
	case reflect.Float32, reflect.Float64:
		_, err = strconv.ParseFloat(value, 64)
	}
	if err != nil {
		return errors.Newf("expect a numeric value, got %q", value)
	}
	return nil
}

// parseQueryTime parses a time-typed query value. RFC 3339 (fractional
// seconds up to nanoseconds accepted) is the one accepted format: its
// mandatory UTC offset makes the value the same instant no matter what zone
// the server runs in, and it is the format time.Time speaks in JSON, so
// query values and body values spell time identically. Zone-less spellings
// and unix timestamps are rejected on purpose — a zone-less value names a
// different instant per server zone, and guessing is worse than failing.
func parseQueryTime(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, errors.Newf("unsupported time format %q, expect RFC 3339", value)
	}
	return t, nil
}
