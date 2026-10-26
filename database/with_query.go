package database

import (
	"maps"
	"reflect"
	"slices"
	"time"

	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// WithQuery sets query conditions based on the provided model struct fields.
// It supports exact matching and field-level operator filters.
// Non-zero fields in the model will be used as query conditions.
//
// Parameters:
//   - query: A model instance with fields set as query conditions. Can be nil
//     to indicate empty query. When nil or all fields are zero values, it's
//     treated as an empty query. The conditions are the ones gorm builds for
//     Where(&query): one per field gorm reads as a column, named by the field's
//     own column — a field of an embedded struct under the struct's prefix —
//     and holding the value gorm binds for it. A type implementing
//     driver.Valuer, such as sql.NullString, and a field declaring a
//     serializer, a slice among them, bind the form their column stores; a byte
//     slice matches byte for byte; time.Time compares as the instant it names
//     whatever its offset, and a date column, a datatypes.Date or a time.Time
//     its type tag declares a date, by the UTC day it names. Of the columns
//     every model carries, only id, created_by and updated_by are conditions.
//   - opts: Optional QueryOptions to control query behavior (empty-query
//     safety, operator filters)
//
// Query Behavior:
//
//	Exact Match (Default):
//	- Every non-zero field is one equality condition (WHERE name = 'alpha')
//	- Multiple fields combine with AND logic (WHERE name = 'alpha' AND age = 18)
//	- The value is a literal: a comma is data, never a list separator. A
//	  query for several values uses the in operator filter instead
//	  (types.Column.In, URL form "field[in]=a,b"), where the list is explicit.
//
//	JSON columns (fields whose type declares a JSON gorm data type, such as
//	the gorm.io/datatypes types JSON, JSONSlice, JSONMap and JSONType): a
//	JSON document is not a scalar, so an exact match fails closed to an
//	empty result on every dialect instead of comparing, a JSON array of
//	numbers as much as one of strings or a map. Substring matching on a JSON
//	document goes through the like-family operator filters, which cast the
//	column where the dialect requires it.
//
//	AllowEmpty:
//	- By default (false): Empty queries are blocked for safety (WHERE 1 = 0)
//	- When true: Allows empty queries to match all records (full table scan)
//	- Empty query cases: nil, empty struct, all fields are zero values, all
//	  field values are empty strings
//	- On a Delete passed no record, it means every row of the model's table
//	  (see Delete)
//
// Examples:
//
//	// Exact match - single field
//	WithQuery(&Sample{Name: "alpha"})  // WHERE name = 'alpha'
//
//	// Exact match - multiple fields (AND logic)
//	WithQuery(&Sample{Name: "alpha", Age: 18})  // WHERE name = 'alpha' AND age = 18
//
//	// Several values for one field - the list is explicit, never comma-parsed
//	WithQuery(nil, types.QueryOptions{Filters: []types.Filter{SampleCols.ID.In(ids...)}})
//
//	// Empty query (blocked by default for safety)
//	WithQuery(nil)  // WHERE 1 = 0 (returns no records)
//	WithQuery(&Sample{})  // WHERE 1 = 0 (returns no records)
//	WithQuery(&Sample{Name: "", Code: ""})  // WHERE 1 = 0 (all values are empty)
//
//	// Empty query with AllowEmpty=true (returns all records)
//	WithQuery(nil, types.QueryOptions{AllowEmpty: true})  // Returns all records
//	WithQuery(&Sample{}, types.QueryOptions{AllowEmpty: true})  // Returns all records
//
//	// Query with some empty and some non-empty fields (works normally)
//	WithQuery(&Sample{Name: "alpha", Code: ""})  // WHERE name = 'alpha' (Code is ignored)
//
// NOTE: The underlying type must be pointer to struct, otherwise panic will
// occur.
//
// NOTE: Empty query conditions (nil or zero value) are blocked by default for
// safety, to prevent catastrophic data loss (e.g., deleting all records). Use
// QueryOptions{AllowEmpty: true} to override.
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
	// Field-level operator conditions are always AND-combined and count as
	// real conditions for the empty-query safety checks.
	hasFilters := len(opt.Filters) > 0
	if hasFilters {
		db.applyFilters(opt.Filters)
	}

	// A nil model carries no field conditions; a model with every field at
	// its zero value is caught below, where its fields turn out empty.
	if queryVal.IsNil() {
		// Filters are already applied above and alone are sufficient, so the
		// empty query safety check is not needed.
		if hasFilters {
			return db
		}
		// No filters and empty query: apply safety check
		if !opt.AllowEmpty {
			logger.Database.WithContext(db.ctx, phaseWithQuery).Warnz(
				"query is nil or empty, adding safety condition to prevent matching all records",
				zap.String("model", reflect.TypeOf(*new(M)).Elem().Name()),
			)
			db.ins = db.ins.Where("1 = 0")
			return db
		}
		// AllowEmpty=true: allow matching all records. A Delete passed no
		// record then removes every row of the table (see Delete).
		db.matchAll = true
		return db
	}

	// Process non-nil, non-empty query
	typ := reflect.TypeOf(query).Elem()

	// Column names come from gorm. A model gorm cannot parse has no usable
	// columns at all, so the query falls closed instead of matching on a
	// guessed column name.
	columns, parseErr := modelschema.Columns(typ)
	if parseErr != nil {
		logger.Database.WithContext(db.ctx, phaseWithQuery).Warnz(
			"cannot resolve model columns, adding safety condition",
			zap.String("model", typ.Name()),
			zap.Error(parseErr),
		)
		db.ins = db.ins.Where("1 = 0")
		return db
	}
	q := structConditions(db.ins.Statement, query, columns, opt.PresentFields)

	// A JSON document is not a scalar, so both matching modes below treat its
	// columns specially: exact matching fails closed and fuzzy matching runs
	// over the document's text form. A time column compares through
	// timeComparableExpr on both sides, as the operator filters do, so the
	// comparison agrees across the storage spellings of an instant.
	jsonColumns := modelschema.JSONColumnSet(typ)
	timeColumns := modelschema.TimeColumnSet(typ)

	// CRITICAL SAFETY CHECK: Empty query conditions
	//
	// Empty query will match ALL records, which is dangerous when:
	// 1. The result is used for subsequent Delete operations → deletes all data
	//    (CATASTROPHIC!)
	// 2. Large datasets returned without pagination → performance/memory issues
	//
	// Empty Query Examples:
	//   - WithQuery(nil)                         → nil query
	//   - WithQuery(&Sample{})                   → all fields are zero values
	//   - WithQuery(&Sample{Name: "", Code: ""}) → every field an empty string
	//   - WithQuery(&Sample{Code: code})         → code from an empty slice
	//
	// By default, empty queries (nil or zero value) are blocked by adding a
	// "WHERE 1 = 0" condition. To allow empty queries, use
	// WithQuery(nil, QueryOptions{AllowEmpty: true}) or
	// WithQuery(&Sample{}, QueryOptions{AllowEmpty: true}).
	if len(q) == 0 {
		// Filters are already applied above and alone are sufficient, so the
		// empty query safety check is not needed.
		if hasFilters {
			return db
		}
		// No filters and empty query: apply safety check
		if !opt.AllowEmpty {
			logger.Database.WithContext(db.ctx, phaseWithQuery).Warnz(
				"all query fields are empty, adding safety condition to prevent matching all records",
				zap.String("model", typ.Name()),
			)
			db.ins = db.ins.Where("1 = 0")
			return db
		}
		// AllowEmpty=true: allow matching all records. A Delete passed no
		// record then removes every row of the table (see Delete).
		db.matchAll = true
		return db
	}

	// Every non-zero field is one equality condition. The value binds as the
	// field holds it: a comma in a string is data, never a list separator, so
	// a value that happens to contain one stays queryable. An explicit list
	// of values is the in operator filter's job. The conditions go out in
	// column name order: ranging over the map would order them differently on
	// every run, so one query would render as a different statement each time.
	hasValidCondition := false
	for _, k := range slices.Sorted(maps.Keys(q)) {
		v := q[k]
		// An empty string, byte slice or slice names no value to match.
		if rv := reflect.ValueOf(v); (rv.Kind() == reflect.String || rv.Kind() == reflect.Slice) && rv.Len() == 0 {
			continue
		}
		hasValidCondition = true
		if _, isJSON := jsonColumns[k]; isJSON {
			// An exact match against a JSON document is not a scalar
			// comparison: MySQL and SQLite answer it with no rows and
			// postgres rejects the SQL outright. Failing closed keeps one
			// portable answer and never widens the result.
			logger.Database.WithContext(db.ctx, phaseWithQuery).Warnz("exact match on JSON column cannot compare, adding safety condition", zap.String("column", k))
			db.ins = db.ins.Where("1 = 0")
			continue
		}
		if _, isTime := timeColumns[k]; isTime {
			db.ins = db.ins.Where(db.timeComparableExpr(db.quoteIdent(k))+" = "+db.timeComparableExpr("?"), v)
			continue
		}
		db.ins = db.ins.Where(db.quoteIdent(k)+" = ?", v)
	}
	// CRITICAL: Check if all query values are empty after filtering
	// Even if query map is not empty, all values might be empty strings: a
	// zero-valued field stays out of the map unless it is marked present, but
	// a non-nil pointer to an empty string, an empty non-nil slice, or an
	// empty string marked present each add a key whose value is empty.
	// Filters applied earlier are real conditions, so they disable this
	// safety check.
	if !hasValidCondition && !hasFilters && !opt.AllowEmpty {
		logger.Database.WithContext(db.ctx, phaseWithQuery).Warnz(
			"all query values are empty, adding safety condition to prevent matching all records",
			zap.String("model", typ.Name()),
		)
		db.ins = db.ins.Where("1 = 0")
	}
	return db
}

// timestampColumns are the framework columns a value on the query model never
// narrows a query by: the timestamps a fetched record carries along.
var timestampColumns = []string{"created_at", "updated_at", "deleted_at"}

// structConditions returns the conditions the fields of query stand for, by
// column, the way gorm builds Where(&query): one equality per readable field
// that is not zero, holding the value gorm binds for it, through the type's
// Valuer or the field's serializer. Each column reads the one field gorm
// resolves it to (see modelschema.Columns), the model's own where it
// declares again a field it embeds. gorm's BuildCondition cannot be asked for
// that: it walks every field it parsed, the shadowed one included, and the
// later of the two values it gives the column wins. The framework keeps its
// own rules on top:
//
//   - A zero value is a condition when present lists the column's URL
//     parameter name and clients may filter on the column (see
//     modelschema.Column.Filterable): a column without a query tag or hidden
//     from JSON is never named so, and neither are the columns every model
//     carries (see frameworkColumns).
//   - Of those columns, the timestamps are no condition at all (see
//     timestampColumns).
//   - A nil pointer names no value, and an instant or a day binds in UTC
//     (see conditionValue).
//
// A query whose Label is "a" and whose embedded First, prefixed first_,
// carries Label "b" stands for {"label": "a", "first_label": "b"}.
func structConditions(stmt *gorm.Statement, query any, columns []modelschema.Column, present map[string]struct{}) map[string]any {
	parsed := &gorm.Statement{DB: stmt.DB}
	if err := parsed.Parse(query); err != nil {
		return nil
	}
	value := reflect.Indirect(reflect.ValueOf(query))

	q := make(map[string]any, len(columns))
	for _, column := range columns {
		field := parsed.Schema.FieldsByDBName[column.DBName]
		_, named := present[column.QueryName]
		named = named && column.Filterable && !slices.Contains(frameworkColumns, column.DBName)
		if field == nil || (!named && !field.Readable) || slices.Contains(timestampColumns, column.DBName) {
			continue
		}
		v, zero := field.ValueOf(stmt.Context, value)
		if (zero && !named) || v == nil {
			continue
		}
		_, val, ok := indirectTypeAndValue(reflect.TypeOf(v), reflect.ValueOf(v))
		if !ok {
			continue
		}
		q[column.DBName] = conditionValue(val, modelschema.IsDateColumn(column))
	}
	return q
}

// conditionValue is the value a condition binds for a field value: the UTC
// day it falls on for a date column, day (see dayValue), an instant in UTC,
// and anything else — a scalar, a JSON document, a Valuer such as
// sql.NullString — as it is.
func conditionValue(val reflect.Value, day bool) any {
	if day {
		return dayValue(val.Interface())
	}
	if t, ok := reflect.TypeAssert[time.Time](val); ok {
		return t.UTC()
	}
	return val.Interface()
}

// indirectTypeAndValue dereferences pointer types and values. Follows pointer
// chains until reaching a non-pointer type. Used for reflection operations that
// need to work with the underlying concrete type.
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
