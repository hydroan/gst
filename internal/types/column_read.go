package types

import (
	"math"
	"reflect"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
)

// Bound is one end of the range a column's comparison filters confine it to:
// the value at that end, whether the range includes it, and whether any filter
// gave that end at all. An absent end leaves the range open on that side.
type Bound[T any] struct {
	Value     T
	Inclusive bool
	Present   bool
}

// Split separates the filters that apply to this column from the rest, keeping
// the order of both. A service that takes one column's conditions over from a
// request, such as a time window it resolves itself, splits them off before
// passing the rest on to the query.
//
// A filter parsed from a request names no table and applies by column name
// alone; a filter built from a column reference names its table, which has to
// be this column's table as well. Groups and the other column-less filters are
// never this column's.
func (c Column[T]) Split(filters []Filter) (own, rest []Filter) {
	for _, f := range filters {
		if namesColumn(f.Table, f.Column, c) {
			own = append(own, f)
			continue
		}
		rest = append(rest, f)
	}
	return own, rest
}

// Values returns the values this column's equality filters hold it to: the
// value of each eq filter and the members of each in filter, in filter order
// and converted to the column's type. Filters on other columns are ignored;
// Split tells which filters are this column's.
//
// Any other operator on this column is an error, and so is a value that does
// not convert: a condition the caller cannot honor is refused rather than
// dropped, which would widen the result.
func (c Column[T]) Values(filters []Filter) ([]T, error) {
	var values []T
	for _, f := range filters {
		if !namesColumn(f.Table, f.Column, c) {
			continue
		}
		switch f.Op {
		case FilterOpEq:
			value, err := filterValueAs[T](f.Value)
			if err != nil {
				return nil, errors.Wrapf(err, "column %q", c.name)
			}
			values = append(values, value)
		case FilterOpIn:
			members, err := filterMembersAs[T](f.Value)
			if err != nil {
				return nil, errors.Wrapf(err, "column %q", c.name)
			}
			values = append(values, members...)
		default:
			return nil, errors.Newf("column %q: operator %q is not an equality filter", c.name, f.Op)
		}
	}
	return values, nil
}

// Bounds returns the ends of the range this column's comparison filters
// confine it to: gt and gte give the lower end, lt and lte the upper one, each
// converted to the column's type. Filters on other columns are ignored; Split
// tells which filters are this column's.
//
// Any other operator on this column is an error. So is a second filter for an
// end already given, which would leave the caller to work out the tighter of
// the two, and a value that does not convert.
func (c Column[T]) Bounds(filters []Filter) (lower, upper Bound[T], err error) {
	for _, f := range filters {
		if !namesColumn(f.Table, f.Column, c) {
			continue
		}
		end, side := &lower, "lower"
		switch f.Op {
		case FilterOpGt, FilterOpGte:
		case FilterOpLt, FilterOpLte:
			end, side = &upper, "upper"
		default:
			return Bound[T]{}, Bound[T]{}, errors.Newf("column %q: operator %q is not a range filter", c.name, f.Op)
		}
		if end.Present {
			return Bound[T]{}, Bound[T]{}, errors.Newf("column %q: more than one filter gives the %s bound", c.name, side)
		}
		value, convertErr := filterValueAs[T](f.Value)
		if convertErr != nil {
			return Bound[T]{}, Bound[T]{}, errors.Wrapf(convertErr, "column %q", c.name)
		}
		*end = Bound[T]{Value: value, Inclusive: f.Op == FilterOpGte || f.Op == FilterOpLte, Present: true}
	}
	return lower, upper, nil
}

// namesColumn reports whether a filter or an order naming table and column
// reads the referenced column. One parsed from a request names no table, so
// the column name alone decides; one built from a column reference names its
// table, which has to match as well.
func namesColumn(table, column string, ref AnyColumnRef) bool {
	return column == ref.Name() && (table == "" || table == ref.Table())
}

// filterTimeType is the Go type of a time column.
var filterTimeType = reflect.TypeFor[time.Time]()

// filterValueAs converts one filter value to the column type T. A value built
// through a column reference already has that type. One parsed from a request
// carries the canonical form the parser normalized it to: the string spelling
// of a scalar, a bool for a bool column, and the UTC wall clock in
// FilterTimeLayout for a time column. A value built from a plain column name
// may carry any numeric type, which converts when it fits.
func filterValueAs[T any](value any) (T, error) {
	if typed, ok := value.(T); ok {
		return typed, nil
	}
	var zero T
	target := reflect.TypeFor[T]()
	converted, ok := convertFilterValue(reflect.ValueOf(value), target)
	if !ok {
		return zero, errors.Newf("value %v of type %T does not convert to %s", value, value, target)
	}
	result, ok := reflect.TypeAssert[T](converted)
	if !ok {
		// Unreachable: the conversion builds a value of exactly the target type.
		return zero, errors.Newf("value %v of type %T does not convert to %s", value, value, target)
	}
	return result, nil
}

// filterMembersAs converts the members of an in filter's value to the column
// type T.
func filterMembersAs[T any](value any) ([]T, error) {
	if typed, ok := value.([]T); ok {
		return typed, nil
	}
	source := reflect.ValueOf(value)
	if source.Kind() != reflect.Slice && source.Kind() != reflect.Array {
		return nil, errors.Newf("in filter value %v of type %T is not a list", value, value)
	}
	members := make([]T, 0, source.Len())
	for i := range source.Len() {
		member, err := filterValueAs[T](source.Index(i).Interface())
		if err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, nil
}

// convertFilterValue converts source to target, the column type, and reports
// whether it could. A numeric conversion refuses a value the target cannot
// hold instead of truncating it, and never turns a fraction into an integer.
func convertFilterValue(source reflect.Value, target reflect.Type) (reflect.Value, bool) {
	if !source.IsValid() {
		return reflect.Value{}, false
	}
	if target == filterTimeType {
		raw, ok := reflect.TypeAssert[string](source)
		if !ok {
			return reflect.Value{}, false
		}
		parsed, err := time.ParseInLocation(FilterTimeLayout, raw, time.UTC)
		if err != nil {
			return reflect.Value{}, false
		}
		return reflect.ValueOf(parsed), true
	}

	result := reflect.New(target).Elem()
	switch target.Kind() {
	case reflect.String:
		if source.Kind() != reflect.String {
			return reflect.Value{}, false
		}
		result.SetString(source.String())
	case reflect.Bool:
		switch source.Kind() {
		case reflect.Bool:
			result.SetBool(source.Bool())
		case reflect.String:
			parsed, err := strconv.ParseBool(source.String())
			if err != nil {
				return reflect.Value{}, false
			}
			result.SetBool(parsed)
		default:
			return reflect.Value{}, false
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var n int64
		switch {
		case source.Kind() == reflect.String:
			parsed, err := strconv.ParseInt(source.String(), 10, target.Bits())
			if err != nil {
				return reflect.Value{}, false
			}
			n = parsed
		case source.CanInt():
			n = source.Int()
		case source.CanUint():
			if source.Uint() > math.MaxInt64 {
				return reflect.Value{}, false
			}
			n = int64(source.Uint()) //nolint:gosec // G115: the check above keeps the value within int64.
		default:
			return reflect.Value{}, false
		}
		if result.OverflowInt(n) {
			return reflect.Value{}, false
		}
		result.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		var n uint64
		switch {
		case source.Kind() == reflect.String:
			parsed, err := strconv.ParseUint(source.String(), 10, target.Bits())
			if err != nil {
				return reflect.Value{}, false
			}
			n = parsed
		case source.CanUint():
			n = source.Uint()
		case source.CanInt():
			if source.Int() < 0 {
				return reflect.Value{}, false
			}
			n = uint64(source.Int()) //nolint:gosec // G115: the check above rules out a negative value.
		default:
			return reflect.Value{}, false
		}
		if result.OverflowUint(n) {
			return reflect.Value{}, false
		}
		result.SetUint(n)
	case reflect.Float32, reflect.Float64:
		var f float64
		switch {
		case source.Kind() == reflect.String:
			parsed, err := strconv.ParseFloat(source.String(), target.Bits())
			if err != nil {
				return reflect.Value{}, false
			}
			f = parsed
		case source.CanFloat():
			f = source.Float()
		case source.CanInt():
			f = float64(source.Int())
		case source.CanUint():
			f = float64(source.Uint())
		default:
			return reflect.Value{}, false
		}
		if result.OverflowFloat(f) {
			return reflect.Value{}, false
		}
		result.SetFloat(f)
	default:
		return reflect.Value{}, false
	}
	return result, true
}
