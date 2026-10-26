package urlquery

import (
	"net/url"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/types"
	"gorm.io/datatypes"
)

// Filters extracts field-level operator filters from URL query keys of the
// form "field[op]=value", e.g. "age[gt]=20" or "remark[like]=hello", plus the
// bare framework timestamp keys ("created_at", "updated_at"), which act as
// exact-match (eq) filters. The field token must be the URL name of a
// queryable column of the model, spelled as the contract spells it (see
// modelschema.QueryColumnName), and op must be a known types.FilterOp;
// anything else is rejected so a mistyped filter can never silently widen
// the result set. Empty values mean "not filtering" and are skipped. Filters
// require the model to embed model.Query, and the returned conditions are
// sorted by key for deterministic SQL.
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

	columns, err := modelschema.FilterableIndex(reflect.TypeOf(m))
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
		col, ok := columns[field]
		if !ok {
			return nil, errors.Newf("invalid field filter %q: unknown field %q", key, field)
		}
		raw := q[key][0]
		if len(raw) == 0 {
			continue
		}
		value, err := normalizeFilterValue(col, op, raw)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid field filter %q", key)
		}
		conds = append(conds, types.NewFilter("", col.DBName, op, value))
	}
	if len(conds) == 0 {
		return nil, nil
	}
	return conds, nil
}

// bareTimeFilterColumns are the framework-managed Base/AutoBase timestamp
// columns. A time decodes into no model field (see fieldSetter), so their
// bare keys are handled by Filters instead, as the exact-match (eq) filter a
// bare key is.
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

// filterOps returns the operators a filter on col takes, in types.FilterOps
// order, read from the data types gorm resolved for the column (see
// modelschema.Column), and the kind of field the others are refused on.
// isnull takes any column; besides it, a time or a time of day (see
// modelschema.IsTimeColumn and modelschema.IsTimeOfDayColumn) takes the
// comparisons alone, a "time" field, a bool eq and ne, a "bool" field, and a
// JSON document (see modelschema.IsJSONColumn) the like family, a "JSON"
// field: whether two documents compare equal or order is the dialect's own
// affair. A number (see modelschema.IsNumericColumn), a string or any other
// column takes every operator.
func filterOps(col modelschema.Column) (ops []types.FilterOp, kind string) {
	switch {
	case modelschema.IsTimeColumn(col), modelschema.IsTimeOfDayColumn(col):
		return []types.FilterOp{
			types.FilterOpEq, types.FilterOpNe,
			types.FilterOpGt, types.FilterOpGte, types.FilterOpLt, types.FilterOpLte,
			types.FilterOpIsNull,
		}, "time"
	case modelschema.IsBoolColumn(col):
		return []types.FilterOp{types.FilterOpEq, types.FilterOpNe, types.FilterOpIsNull}, "bool"
	case modelschema.IsNumericColumn(col):
		return types.FilterOps(), ""
	case modelschema.IsJSONColumn(col):
		return []types.FilterOp{
			types.FilterOpLike, types.FilterOpNotLike, types.FilterOpStartsWith, types.FilterOpEndsWith,
			types.FilterOpIsNull,
		}, "JSON"
	default:
		return types.FilterOps(), ""
	}
}

// normalizeFilterValue validates a filter value against what the column
// stores, read from the data types gorm resolved for it (see
// modelschema.Column), and rewrites it into the canonical typed value bound
// to the statement, so a malformed value is rejected with an error instead
// of being passed to the database where implicit conversion could silently
// match the wrong rows. A pointer column, or a Valuer such as sql.NullInt64,
// is checked as the value gorm reads through it. An operator the column
// takes no filter with (see filterOps) is refused first.
//
//   - isnull requires a boolean value, carried as a bool, whatever the
//     column.
//   - time columns and time of day columns travel as timeBound renders the
//     value.
//   - bool columns carry the boolean value as a bool.
//   - numeric columns require numeric values; in/notin validate every
//     comma-separated member.
//   - JSON columns match the document's text.
//   - in/notin values split on commas here, so the members travel as a real
//     slice: the URL list encoding never reaches the database layer.
//   - string and other scalar values pass through unchanged.
func normalizeFilterValue(col modelschema.Column, op types.FilterOp, value string) (any, error) {
	if ops, kind := filterOps(col); !slices.Contains(ops, op) {
		return nil, errors.Newf("operator %q is not supported on a %s field", op, kind)
	}
	// isnull is the only operator whose value type is independent of the
	// column type: it always carries a boolean.
	if op == types.FilterOpIsNull {
		b, err := modelschema.ParseBool(value)
		if err != nil {
			return nil, errors.Newf("isnull expects a boolean value, got %q", value)
		}
		return b, nil
	}
	switch {
	case modelschema.IsTimeColumn(col), modelschema.IsTimeOfDayColumn(col):
		return timeBound(col, value)
	case modelschema.IsBoolColumn(col):
		b, err := modelschema.ParseBool(value)
		if err != nil {
			return nil, err
		}
		return b, nil
	case modelschema.IsNumericColumn(col):
		switch op {
		case types.FilterOpIn, types.FilterOpNotIn:
			items := strings.Split(value, ",")
			for _, item := range items {
				if err := modelschema.ValidateNumericValue(col, item); err != nil {
					return nil, err
				}
			}
			return items, nil
		case types.FilterOpLike, types.FilterOpNotLike, types.FilterOpStartsWith, types.FilterOpEndsWith:
			// Substring matching relies on the database's string rendering of
			// the number; the pattern itself is not numeric.
			return value, nil
		default:
			if err := modelschema.ValidateNumericValue(col, value); err != nil {
				return nil, err
			}
			return value, nil
		}
	case modelschema.IsJSONColumn(col):
		return value, nil
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

// timeBound is the value a comparison with a time column binds for the RFC
// 3339 value a client sent (see modelschema.ParseTime): the UTC wall clock in
// FilterTimeLayout, the one wall clock the framework stores on every
// dialect. The canonical string form is kept on purpose: binding time.Time
// would let the driver re-render the value in its own location, while the
// string pins the wall clock the parser resolved. A date column takes the
// instant as well, which the database compares by the UTC day it falls on,
// so a service reading the filter sees the time that was sent.
//
// A time of day column takes its value the way modelschema.ParseTimeOfDay
// reads one instead, and binds it in the spelling datatypes.Time stores, the
// text SQLite compares: 17:30:00.5 binds as 17:30:00.500000000.
func timeBound(col modelschema.Column, value string) (string, error) {
	if modelschema.IsTimeOfDayColumn(col) {
		since, err := modelschema.ParseTimeOfDay(value)
		if err != nil {
			return "", err
		}
		return datatypes.Time(since).String(), nil
	}
	t, err := modelschema.ParseTime(value)
	if err != nil {
		return "", err
	}
	return t.UTC().Format(types.FilterTimeLayout), nil
}
