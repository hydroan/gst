package database

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// WithQuery sets query conditions based on the provided model struct fields.
// It supports exact matching, field-level operator filters, and raw SQL conditions.
// Non-zero fields in the model will be used as query conditions.
//
// Parameters:
//   - query: A model instance with fields set as query conditions. Can be nil to indicate empty query.
//     When nil or all fields are zero values, it's treated as an empty query.
//     Supported field types: string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool, pointer types.
//   - opts: Optional QueryOptions to control query behavior (empty-query safety, operator filters, raw SQL)
//
// Query Behavior:
//
//	Exact Match (Default):
//	- Every non-zero field is one equality condition (WHERE name = 'John')
//	- Multiple fields combine with AND logic (WHERE name = 'John' AND age = 18)
//	- The value is a literal: a comma is data, never a list separator. A
//	  query for several values uses the in operator filter instead
//	  (types.FilterIn, URL form "field[in]=a,b"), where the list is explicit.
//
//	JSON columns (fields whose type declares a JSON gorm data type, such as
//	the gorm.io/datatypes types): a JSON document is not a scalar, so an
//	exact match fails closed to an empty result on every dialect instead of
//	comparing. Substring matching on a JSON document goes through the
//	like-family operator filters, which cast the column where the dialect
//	requires it.
//
//	RawQuery:
//	- When provided, it will be combined with model fields using AND logic
//	- Works even when query is nil
//	- Supports parameterized queries with RawQueryArgs
//	- Example: WHERE age > ? AND status = ?
//	- When both RawQuery and model fields are provided, they are combined with AND logic
//	- Example: RawQuery "age > ?" + model field Name="John" → WHERE age > ? AND name IN ('John')
//
//	AllowEmpty:
//	- By default (false): Empty queries are blocked for safety (adds WHERE 1 = 0)
//	- When true: Allows empty queries to match all records (full table scan)
//	- Empty query cases: nil, empty struct, all fields are zero values, all field values are empty strings
//	- Critical: Use with caution, especially for Delete operations
//
// Examples:
//
//	// Exact match - single field
//	WithQuery(&model.User{Name: "John"})  // WHERE name = 'John'
//
//	// Exact match - multiple fields (AND logic)
//	WithQuery(&model.User{Name: "John", Age: 18})  // WHERE name = 'John' AND age = 18
//
//	// Several values for one field - the list is explicit, never comma-parsed
//	WithQuery(nil, types.QueryOptions{Filters: []types.Filter{types.FilterIn("id", ids)}})
//
//		// Raw SQL query (can be combined with model fields)
//	WithQuery(&model.User{}, types.QueryOptions{RawQuery: "age > ? AND status = ?", RawQueryArgs: []any{18, "active"}})
//	WithQuery(nil, types.QueryOptions{RawQuery: "created_at BETWEEN ? AND ?", RawQueryArgs: []any{startDate, endDate}})
//	WithQuery(&model.User{Name: "John"}, types.QueryOptions{RawQuery: "age > ?", RawQueryArgs: []any{18}})  // WHERE age > ? AND name = 'John'
//
//	// Empty query (blocked by default for safety)
//	WithQuery(nil)  // WHERE 1 = 0 (returns no records)
//	WithQuery(&model.User{})  // WHERE 1 = 0 (returns no records)
//	WithQuery(&model.User{Name: "", Email: ""})  // WHERE 1 = 0 (all values are empty)
//
//	// Empty query with AllowEmpty=true (returns all records)
//	WithQuery(nil, types.QueryOptions{AllowEmpty: true})  // Returns all records
//	WithQuery(&model.User{}, types.QueryOptions{AllowEmpty: true})  // Returns all records
//
//	// Query with some empty and some non-empty fields (works normally)
//	WithQuery(&model.User{Name: "John", Email: ""})  // WHERE name = 'John' (Email is ignored)
//
// NOTE: The underlying type must be pointer to struct, otherwise panic will occur.
// NOTE: Empty query conditions (nil or zero value) are blocked by default for safety to prevent
//
//	catastrophic data loss (e.g., deleting all records). Use QueryOptions{AllowEmpty: true} to override.
//
// NOTE: When both RawQuery and model fields are provided, they are combined with AND logic.
func (db *database[M]) WithQuery(query M, opts ...types.QueryOptions) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Parse query options
	var opt types.QueryOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	// opt.AllowEmpty: default false (block empty queries for safety)

	queryVal := reflect.ValueOf(query)
	// Handle RawQuery first (works even if query is nil)
	// RawQuery will be combined with model fields using AND logic if both are provided
	hasRawQuery := len(opt.RawQuery) > 0
	if hasRawQuery {
		db.ins = db.ins.Where(opt.RawQuery, opt.RawQueryArgs...)
	}

	// Field-level operator conditions are always AND-combined and, like
	// RawQuery, count as real conditions for the empty-query safety checks.
	hasFilters := len(opt.Filters) > 0
	if hasFilters {
		db.applyFilters(opt.Filters)
	}

	// Check if query is nil or empty
	var empty M
	if queryVal.IsNil() || reflect.DeepEqual(query, empty) {
		// Treat nil/empty as empty query
		// If RawQuery or filters are provided, they are already
		// applied above and alone are sufficient, so the empty query safety
		// check is not needed.
		if hasRawQuery || hasFilters {
			return db
		}
		// No RawQuery and empty query: apply safety check
		if !opt.AllowEmpty {
			logger.Database.WithContext(db.ctx, consts.Phase("WithQuery")).Warn("query is nil or empty, adding safety condition to prevent matching all records")
			db.ins = db.ins.Where("1 = 0")
			return db
		}
		// AllowEmpty=true: allow matching all records
		logger.Database.WithContext(db.ctx, consts.Phase("WithQuery")).Info("query is nil or empty but AllowEmpty=true, allowing full table scan")
		return db
	}

	// Process non-nil, non-empty query
	typ := reflect.TypeOf(query).Elem()
	val := reflect.ValueOf(query).Elem()
	q := make(map[string]string)

	// Column names come from gorm through modelschema. A model gorm cannot
	// parse has no usable columns at all, so the query falls closed instead
	// of matching on a guessed column name.
	parsedColumns, parseErr := modelschema.Columns(typ)
	if parseErr != nil {
		logger.Database.WithContext(db.ctx, consts.Phase("WithQuery")).Warnf("cannot resolve columns of %s: %v, adding safety condition", typ, parseErr)
		db.ins = db.ins.Where("1 = 0")
		return db
	}
	structFieldToMap(db.ctx, typ, val, q, opt.PresentFields, modelschema.ByGoName(parsedColumns))

	// A JSON document is not a scalar, so both matching modes below treat its
	// columns specially: exact matching fails closed and fuzzy matching runs
	// over the document's text form.
	jsonColumns := jsonColumnSet(typ)

	// CRITICAL SAFETY CHECK: Empty query conditions
	//
	// Empty query will match ALL records, which is dangerous when:
	// 1. The result is used for subsequent Delete operations → deletes all data (CATASTROPHIC!)
	// 2. Large datasets returned without pagination → performance/memory issues
	//
	// Empty Query Examples:
	//   - WithQuery(nil)                         → nil query
	//   - WithQuery(&User{})                    → all fields are zero values
	//   - WithQuery(&User{Name: "", Email: ""}) → all field values are empty strings
	//   - WithQuery(&KV{Key: ""})               → happens when removed slice is empty
	//
	// By default, empty queries (nil or zero value) are blocked by adding "WHERE 1 = 0" condition.
	// To allow empty queries, use: WithQuery(nil, QueryOptions{AllowEmpty: true}) or
	//                              WithQuery(&User{}, QueryOptions{AllowEmpty: true})
	if len(q) == 0 {
		// If RawQuery or filters are provided, they are already
		// applied above and alone are sufficient, so the empty query safety
		// check is not needed.
		if hasRawQuery || hasFilters {
			return db
		}
		// No RawQuery and empty query: apply safety check
		if !opt.AllowEmpty {
			logger.Database.WithContext(db.ctx, consts.Phase("WithQuery")).Warn("all query fields are empty, adding safety condition to prevent matching all records")
			db.ins = db.ins.Where("1 = 0")
			return db
		}
		// AllowEmpty=true: allow matching all records
		logger.Database.WithContext(db.ctx, consts.Phase("WithQuery")).Info("all query fields are empty but AllowEmpty=true, allowing full table scan")
		return db
	}

	// Every non-zero field is one equality condition. The value binds as a
	// literal: a comma is data, never a list separator, so a value that
	// happens to contain one stays queryable. An explicit list of values is
	// the in operator filter's job.
	hasValidCondition := false
	for k, v := range q {
		if len(v) == 0 {
			continue
		}
		hasValidCondition = true
		if _, isJSON := jsonColumns[k]; isJSON {
			// An exact match against a JSON document is not a scalar
			// comparison: MySQL and SQLite answer it with no rows and
			// postgres rejects the SQL outright. Failing closed keeps one
			// portable answer and never widens the result.
			logger.Database.WithContext(db.ctx, consts.Phase("WithQuery")).Warnf("exact match on JSON column %q cannot compare, adding safety condition", k)
			db.ins = db.ins.Where("1 = 0")
			continue
		}
		db.ins = db.ins.Where(db.quoteIdent(k)+" = ?", v)
	}
	// CRITICAL: Check if all query values are empty after filtering
	// Even if query map is not empty, all values might be empty strings
	// Example: &User{Name: "", Email: ""} has fields but all values are empty
	// Filters applied earlier are real conditions, so they
	// disable this safety check the same way RawQuery would.
	if !hasValidCondition && !hasFilters {
		if !opt.AllowEmpty {
			logger.Database.WithContext(db.ctx, consts.Phase("WithQuery")).Warn("all query values are empty, adding safety condition to prevent matching all records")
			db.ins = db.ins.Where("1 = 0")
		} else {
			logger.Database.WithContext(db.ctx, consts.Phase("WithQuery")).Info("all query values are empty but AllowEmpty=true, allowing full table scan")
		}
	}
	return db
}

// structFieldToMap extracts the field tags from a struct and writes them into a map.
// This map can then be used to build SQL query conditions.
//
// Zero-value fields are treated as unset and skipped, unless their column name
// is listed in present: presence marks filter values explicitly provided by the
// caller, so explicit zero values such as false and 0 still become conditions.
func structFieldToMap(ctx context.Context, typ reflect.Type, val reflect.Value, q map[string]string, present map[string]struct{}, columns map[string]modelschema.Column) {
	if q == nil {
		q = make(map[string]string)
	}
	for i := range typ.NumField() {
		field := typ.Field(i)
		fieldTyp := field.Type
		fieldVal := val.Field(i)

		// A field gorm does not map to a column is not a query condition.
		// The embedded base structs are the exception: they are handled
		// explicitly below, since gorm lifts their fields to the top level.
		col, isColumn := columns[field.Name]

		if fieldVal.IsZero() {
			if len(present) == 0 {
				continue
			}
			if !isColumn {
				continue
			}
			if _, ok := present[col.QueryName]; !ok {
				continue
			}
		}
		if !fieldVal.CanInterface() {
			continue
		}
		fieldTyp, fieldVal, ok := indirectTypeAndValue(fieldTyp, fieldVal)
		if !ok {
			continue
		}
		// The marker interfaces are sealed in modelregistry, so recognition of
		// framework query fields lives there as the single source of truth.
		if modelregistry.IsQueryMarkerType(fieldTyp) {
			continue
		}

		switch fieldTyp.Kind() {
		case reflect.Chan, reflect.Map, reflect.Func:
			continue
		case reflect.Struct:
			// Base and AutoBase are the framework base models: lift only their
			// query-relevant fields instead of recursing, so framework-managed
			// fields such as DeletedAt never leak into query conditions.
			if field.Name == "Base" || field.Name == "AutoBase" {
				if !fieldVal.FieldByName("CreatedBy").IsZero() {
					// Not overwrite the "CreatedBy" value set in types.Model.
					// The "CreatedBy" value set in types.Model has higher priority than base model.
					if _, loaded := q["created_by"]; !loaded {
						q["created_by"] = fieldVal.FieldByName("CreatedBy").Interface().(string) //nolint:errcheck
					}
				}
				if !fieldVal.FieldByName("UpdatedBy").IsZero() {
					// Not overwrite the "UpdatedBy" value set in types.Model.
					// The "UpdatedBy" value set in types.Model has higher priority than base model.
					if _, loaded := q["updated_by"]; !loaded {
						q["updated_by"] = fieldVal.FieldByName("UpdatedBy").Interface().(string) //nolint:errcheck
					}
				}
				if !fieldVal.FieldByName("ID").IsZero() {
					// Not overwrite the "ID" value set in types.Model.
					// The "ID" value set in types.Model has higher priority than base model.
					if _, loaded := q["id"]; !loaded {
						switch idField := fieldVal.FieldByName("ID"); idField.Kind() {
						case reflect.String: // Base: UUIDv7 string primary key.
							q["id"] = idField.String()
						case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64: // AutoBase: auto-increment integer primary key.
							q["id"] = strconv.FormatUint(idField.Uint(), 10)
						}
					}
				}
			} else {
				structFieldToMap(ctx, fieldTyp, fieldVal, q, present, columns)
			}
			continue
		}
		if !isColumn {
			continue
		}
		columnName := col.DBName

		if !fieldVal.CanInterface() {
			continue
		}
		v := fieldVal.Interface()
		var _v string
		switch fieldVal.Kind() {
		case reflect.Bool:
			// A WHERE IN clause quotes its values automatically, eg WHERE `default` IN ('true'),
			// but what we want is WHERE `default` IN (true),
			// so the only way out is to convert the bool into an int here.
			_v = strconv.Itoa(boolToInt(v.(bool))) //nolint:errcheck
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			_v = fmt.Sprintf("%d", v)
		case reflect.Float32, reflect.Float64:
			_v = fmt.Sprintf("%g", v)
		case reflect.String:
			_v = fmt.Sprintf("%s", v)
		case reflect.Pointer:
			v = fieldVal.Elem().Interface()
			// switch typ.Elem().Kind() {
			switch fieldVal.Elem().Kind() {
			case reflect.Bool:
				_v = strconv.Itoa(boolToInt(v.(bool))) //nolint:errcheck
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				_v = fmt.Sprintf("%d", v)
			case reflect.Float32, reflect.Float64:
				_v = fmt.Sprintf("%g", v)
			case reflect.String:
				_v = fmt.Sprintf("%s", v)
			case reflect.Struct, reflect.Map, reflect.Chan, reflect.Func: // ignore the struct, map, chan, func
			default:
				_v = fmt.Sprintf("%v", v)
			}
		case reflect.Slice:
			_len := fieldVal.Len()
			if _len == 0 {
				logger.Database.WithContext(ctx, consts.Phase("WithQuery")).Warn("reflect.Slice length is 0")
				_len = 1
			}
			slice := reflect.MakeSlice(fieldVal.Type(), _len, _len)
			switch slice.Index(0).Kind() {
			case reflect.String: // handle string slice.
				// WARN: fieldVal.Type() may be a named slice type (e.g.
				// datatypes.JSONSlice[string]) instead of []string, so asserting
				// slice.Interface().([]string) directly would panic. Rebuild the
				// value as a plain []string first.
				slice = reflect.MakeSlice(reflect.TypeFor[[]string](), _len, _len)
				reflect.Copy(slice, fieldVal)
				_v = strings.Join(slice.Interface().([]string), ",") //nolint:errcheck
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			// TODO: handle integer slice.
			case reflect.Float32, reflect.Float64:
			// TODO: handle float slice.
			default:
				_v = fmt.Sprintf("%v", v)
			}
		case reflect.Struct, reflect.Map, reflect.Chan, reflect.Func: // ignore the struct, map, chan, func
		default:
			_v = fmt.Sprintf("%v", v)
		}

		q[columnName] = _v
	}
}

// boolToInt converts a boolean value to an integer.
// Returns 1 for true, 0 for false.
// Useful for database operations that require integer representations of boolean values.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// indirectTypeAndValue recursively dereferences pointer types and values.
// Follows pointer chains until reaching a non-pointer type.
// Used for reflection operations that need to work with the underlying concrete type.
//
// Parameters:
//   - t: The reflect.Type to dereference
//   - v: The reflect.Value to dereference
//
// Returns:
//   - reflect.Type: The final non-pointer type
//   - reflect.Value: The final non-pointer value
//   - bool: true if successful, false if encountered nil pointer
//
// Example:
//   - Input: **int (pointer to pointer to int)
//   - Output: int (the underlying int type)
func indirectTypeAndValue(t reflect.Type, v reflect.Value) (reflect.Type, reflect.Value, bool) {
	for t.Kind() == reflect.Pointer {
		if v.IsNil() {
			return t, v, false
		}
		t = t.Elem()
		v = v.Elem()
	}
	return t, v, true
}
