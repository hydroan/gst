package database

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/hints"
)

// WithIndex specifies database index hints for query optimization.
// The first parameter is the index name, and the second optional parameter specifies the hint type.
// If no hint type is provided, defaults to USE INDEX.
//
// Parameters:
//   - indexName: The name of the index to hint. Empty or whitespace-only names are ignored.
//   - hint: Optional hint mode. If not provided, defaults to consts.IndexHintUse.
//     Supported modes:
//   - consts.IndexHintUse: Suggests the database to use the specified index
//   - consts.IndexHintForce: Forces the database to use the specified index
//   - consts.IndexHintIgnore: Tells the database to ignore the specified index
//
// IMPORTANT: Index hints are ONLY supported in SELECT queries (List, Get, Count, First, Last, Take).
// They are NOT supported in INSERT, UPDATE, DELETE operations. Using WithIndex with Create, Update,
// or Delete methods will result in SQL syntax errors.
//
// Database Compatibility:
//   - MySQL: Fully supported. All hint modes work as expected.
//     If the index doesn't exist, MySQL may return an error.
//   - SQLite/PostgreSQL/Other databases: Not supported.
//     This method will log a warning and skip the hint silently.
//     The query will execute normally without the index hint.
//
// Empty Index Name Handling:
//   - Empty string ("") or whitespace-only strings are automatically trimmed and ignored.
//   - The query will execute normally without any index hint.
//
// Examples:
//
//	// Default USE INDEX hint
//	database.Database[*model.User](context.Background()).WithIndex("idx_name").List(&users)
//
//	// Explicit hint modes
//	database.Database[*model.User](context.Background()).WithIndex("idx_name", consts.IndexHintForce).List(&users)
//	database.Database[*model.User](context.Background()).WithIndex("idx_name", consts.IndexHintIgnore).List(&users)
//
//	// Combined with other methods
//	database.Database[*model.User](context.Background()).
//	    WithIndex("idx_name").
//	    WithQuery(&model.User{Name: "John"}).
//	    List(&users)
//
// NOTE: Index hints are MySQL-specific. On other databases, the hint is silently ignored.
// NOTE: Empty or whitespace-only index names are automatically ignored for safe chaining.
// NOTE: Unknown hint modes will default to USE INDEX with a warning logged.
func (db *database[M]) WithIndex(indexName string, hint ...consts.IndexHintMode) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Trim whitespace from the index name
	indexName = strings.TrimSpace(indexName)
	if len(indexName) == 0 {
		return db
	}

	// Check if database supports index hints (only MySQL supports them)
	// SQLite, PostgreSQL, and other databases don't support index hints
	if db.ins == nil {
		return db
	}

	// Get database driver name to check if it's MySQL
	driverName := db.ins.Name()
	if driverName != "mysql" {
		// Index hints are only supported by MySQL
		// For other databases (SQLite, PostgreSQL, etc.), log a warning and skip
		logger.Database.WithContext(db.ctx, consts.Phase("WithIndex")).Warnf(
			"index hints are not supported by %s database, skipping index hint for: %s",
			driverName, indexName,
		)
		return db
	}

	// Determine the hint type, default to USE if not provided
	var hintMode consts.IndexHintMode
	if len(hint) > 0 {
		hintMode = hint[0]
	} else {
		hintMode = consts.IndexHintUse
	}

	// Apply the appropriate hint
	switch hintMode {
	case consts.IndexHintUse:
		db.ins = db.ins.Clauses(hints.UseIndex(indexName))
	case consts.IndexHintForce:
		db.ins = db.ins.Clauses(hints.ForceIndex(indexName))
	case consts.IndexHintIgnore:
		db.ins = db.ins.Clauses(hints.IgnoreIndex(indexName))
	default:
		logger.Database.Warnf(`unknown index hint mode: %s, using "USE INDEX"`, hintMode)
		// Default to USE INDEX for unknown modes
		db.ins = db.ins.Clauses(hints.UseIndex(indexName))
	}

	return db
}

// WithQuery sets query conditions based on the provided model struct fields.
// It supports exact matching, fuzzy matching (LIKE/REGEXP), OR/AND logic, and raw SQL queries.
// Non-zero fields in the model will be used as query conditions.
//
// Parameters:
//   - query: A model instance with fields set as query conditions. Can be nil to indicate empty query.
//     When nil or all fields are zero values, it's treated as an empty query.
//     Supported field types: string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool, pointer types.
//   - opts: Optional QueryOptions to control query behavior (fuzzy matching, empty queries, OR logic, raw SQL)
//
// Query Behavior:
//
//	Exact Match (Default):
//	- Single value: Uses IN clause with one value (WHERE name IN ('John'))
//	- Multiple values (comma-separated): Uses IN clause with multiple values (WHERE name IN ('John', 'Jack'))
//	- Multiple fields: Uses AND logic to combine conditions (WHERE name IN ('John') AND age IN (18))
//	- Empty strings in comma-separated values are automatically skipped
//
//	FuzzyMatch:
//	- Single value: Uses LIKE pattern (WHERE name LIKE '%John%')
//	- Multiple values (comma-separated): Uses REGEXP pattern (WHERE name REGEXP '.*John.*|.*Jack.*')
//	- REGEXP special characters are automatically escaped using regexp.QuoteMeta
//	- Empty strings in comma-separated values are automatically skipped to prevent matching all records
//	- Note: REGEXP may not be available in all databases (e.g., SQLite requires extension)
//
//	Or:
//	- When true: Combines multiple filters with OR instead of AND
//	- First condition always uses WHERE, subsequent conditions use OR
//	- Example: WHERE name IN ('John') OR email IN ('john@example.com')
//	- Works with both exact match and fuzzy match
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
//	// Exact match - single field, single value
//	WithQuery(&model.User{Name: "John"})  // WHERE name IN ('John')
//
//	// Exact match - single field, multiple values (comma-separated)
//	WithQuery(&model.User{Name: "John,Jack"})  // WHERE name IN ('John', 'Jack')
//	WithQuery(&model.User{ID: "id1,id2,id3"})  // WHERE id IN ('id1', 'id2', 'id3')
//
//	// Exact match - multiple fields (AND logic)
//	WithQuery(&model.User{Name: "John", Age: 18})  // WHERE name IN ('John') AND age IN (18)
//	WithQuery(&model.User{Name: "John", Age: 18, Email: "john@example.com"})  // WHERE name IN ('John') AND age IN (18) AND email IN ('john@example.com')
//
//	// Fuzzy match - single value (LIKE)
//	WithQuery(&model.User{Name: "John"}, types.QueryOptions{FuzzyMatch: true})  // WHERE name LIKE '%John%'
//
//	// Fuzzy match - multiple values (REGEXP)
//	WithQuery(&model.User{Name: "John,Jack"}, types.QueryOptions{FuzzyMatch: true})  // WHERE name REGEXP '.*John.*|.*Jack.*'
//
//	// Fuzzy match - empty strings in comma-separated values are skipped
//	WithQuery(&model.User{Name: "John,,Jack"}, types.QueryOptions{FuzzyMatch: true})  // WHERE name REGEXP '.*John.*|.*Jack.*'
//
//		// Raw SQL query (can be combined with model fields)
//	WithQuery(&model.User{}, types.QueryOptions{RawQuery: "age > ? AND status = ?", RawQueryArgs: []any{18, "active"}})
//	WithQuery(nil, types.QueryOptions{RawQuery: "created_at BETWEEN ? AND ?", RawQueryArgs: []any{startDate, endDate}})
//	WithQuery(&model.User{Name: "John"}, types.QueryOptions{RawQuery: "age > ?", RawQueryArgs: []any{18}})  // WHERE age > ? AND name IN ('John')
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
//	WithQuery(&model.User{Name: "John", Email: ""})  // WHERE name IN ('John') (Email is ignored)
//
//	// Combined options
//	WithQuery(&model.User{Name: "John"}, types.QueryOptions{
//	    FuzzyMatch: true,
//	    AllowEmpty: false,
//	})
//
// NOTE: The underlying type must be pointer to struct, otherwise panic will occur.
// NOTE: Empty query conditions (nil or zero value) are blocked by default for safety to prevent
//
//	catastrophic data loss (e.g., deleting all records). Use QueryOptions{AllowEmpty: true} to override.
//
// NOTE: When both RawQuery and model fields are provided, they are combined with AND logic.
// NOTE: REGEXP function may not be available in all databases (e.g., SQLite requires extension).
//
//	For SQLite compatibility, consider using FuzzyMatch with single values (LIKE) or RawQuery.
func (db *database[M]) WithQuery(query M, opts ...types.QueryOptions) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Parse query options
	var opt types.QueryOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	// opt.FuzzyMatch: default false (exact match)
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

	if opt.FuzzyMatch {
		// // Deprecated!
		// for k, v := range q {
		// 	// WARN: THE SQL STATEMENT MUST CONTAINS backticks ``.
		// 	db.db = db.db.Where(fmt.Sprintf("`%s` LIKE ?", k), fmt.Sprintf("%%%v%%", v))
		// }

		// If the query strings has multiple value(separated by ',')
		// construct the 'WHERE' 'REGEXP' SQL statement
		// eg: SELECT * FROM `assets` WHERE `category_level2_id` REGEXP '.*XS.*|.*NU.*'
		//     SELECT count(*) FROM `assets` WHERE `category_level2_id` REGEXP '.*XS.*|.*NU.*'
		hasValidCondition := false
		for k, v := range q {
			items := strings.Split(v, ",")
			// skip the string slice which all element is empty.
			if len(strings.Join(items, "")) == 0 {
				continue
			}
			hasValidCondition = true
			if len(items) > 1 { // If the query string has multiple value(separated by ','), using regexp
				var regexpVal string
				for _, item := range items {
					// Skip empty items to avoid matching all records (.*.* pattern)
					if len(item) == 0 {
						continue
					}
					// WARN: not forget to escape the regexp value using regexp.QuoteMeta.
					// eg: localhost\hello.world -> localhost\\hello\.world
					regexpVal = regexpVal + "|.*" + regexp.QuoteMeta(item) + ".*"
				}
				// If all items were empty after filtering, skip this condition
				if len(regexpVal) == 0 {
					continue
				}
				regexpVal = strings.TrimPrefix(regexpVal, "|")
				db.ins = db.ins.Where(fmt.Sprintf("%s %s ?", db.quoteIdent(k), db.regexpOperator()), regexpVal)
			} else { // If the query string has only one value, using LIKE
				db.ins = db.ins.Where(db.quoteIdent(k)+" LIKE ?", fmt.Sprintf("%%%v%%", v))
			}
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
	} else {
		// // Deprecated!
		// // SELECT * FROM `assets` WHERE `assets`.`category_level2_id` = 'NU
		// // SELECT count(*) FROM `assets` WHERE `assets`.`category_level2_id` = 'NU'
		// db.db = db.db.Where(query)

		// If the query string has multiple value(separated by ','),
		// construct the 'WHERE' 'IN' SQL statement.
		// eg: SELECT id FROM users WHERE name IN ('user01', 'user02', 'user03', 'user04')
		hasValidCondition := false
		for k, v := range q {
			items := strings.Split(v, ",")
			if len(strings.Join(items, "")) == 0 {
				continue
			}
			hasValidCondition = true
			db.ins = db.ins.Where(db.quoteIdent(k)+" IN ?", items)
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
	}
	return db
}

// applyFilters appends field-level operator filters, each as an AND
// condition with its value bound as a statement parameter. Every mismatch
// fails closed with "1 = 0" instead of being dropped: silently dropping a
// filter would widen the result set, which is dangerous when the result
// feeds deletes or exports. That covers an empty column, an operator the
// switch does not recognize, and a value whose type does not match the
// operator (see the Filter type for the per-operator value contract).
//
// The caller must hold db.mu.
func (db *database[M]) applyFilters(filters []types.Filter) {
	db.ins = db.filterConditions(db.ins, filters, false)
}

// filterConditions applies every filter onto tx and returns the result. The
// filters are OR-combined with each other when or is set and AND-combined
// otherwise; the top-level call always combines with AND.
//
// The caller must hold db.mu.
func (db *database[M]) filterConditions(tx *gorm.DB, filters []types.Filter, or bool) *gorm.DB {
	for _, f := range filters {
		// Groups carry their children in Value and have no column of their
		// own, so they are dispatched before the empty-column check below.
		switch f.Op {
		case types.FilterOpOr:
			tx = db.combine(tx, or, db.groupCondition(f, true))
			continue
		case types.FilterOpAnd:
			tx = db.combine(tx, or, db.groupCondition(f, false))
			continue
		}
		if len(f.Column) == 0 {
			logger.Database.WithContext(db.ctx, consts.Phase("WithQuery")).Warn("filter has empty column, adding safety condition")
			tx = db.combine(tx, or, "1 = 0")
			continue
		}
		column := db.quoteIdent(f.Column)
		switch f.Op {
		case types.FilterOpEq:
			tx = db.applyScalarFilter(tx, or, f, column+" = ?")
		case types.FilterOpNe:
			tx = db.applyScalarFilter(tx, or, f, column+" <> ?")
		case types.FilterOpGt:
			tx = db.applyScalarFilter(tx, or, f, column+" > ?")
		case types.FilterOpGte:
			tx = db.applyScalarFilter(tx, or, f, column+" >= ?")
		case types.FilterOpLt:
			tx = db.applyScalarFilter(tx, or, f, column+" < ?")
		case types.FilterOpLte:
			tx = db.applyScalarFilter(tx, or, f, column+" <= ?")
		case types.FilterOpIn:
			tx = db.applyListFilter(tx, or, f, column+" IN ?")
		case types.FilterOpNotIn:
			tx = db.applyListFilter(tx, or, f, column+" NOT IN ?")
		case types.FilterOpLike:
			tx = db.applyPatternFilter(tx, or, f, column+" LIKE ?"+likeEscapeClause, "%", "%")
		case types.FilterOpNotLike:
			tx = db.applyPatternFilter(tx, or, f, column+" NOT LIKE ?"+likeEscapeClause, "%", "%")
		case types.FilterOpStartsWith:
			tx = db.applyPatternFilter(tx, or, f, column+" LIKE ?"+likeEscapeClause, "", "%")
		case types.FilterOpEndsWith:
			tx = db.applyPatternFilter(tx, or, f, column+" LIKE ?"+likeEscapeClause, "%", "")
		case types.FilterOpIsNull:
			b, ok := f.Value.(bool)
			if !ok {
				tx = db.failClosedFilter(tx, or, f, "expects a bool value")
				continue
			}
			if b {
				tx = db.combine(tx, or, column+" IS NULL")
			} else {
				tx = db.combine(tx, or, column+" IS NOT NULL")
			}
		case types.FilterOpRegex:
			tx = db.applyStringFilter(tx, or, f, column+" "+db.regexpOperator()+" ?")
		case types.FilterOpNotRegex:
			tx = db.applyStringFilter(tx, or, f, "NOT ("+column+" "+db.regexpOperator()+" ?)")
		case types.FilterOpJSONContains:
			// datatypes handles the dialect split: JSON_CONTAINS on MySQL and
			// a json_each EXISTS subquery on SQLite. The column is passed
			// unquoted because the expression quotes it itself.
			s, ok := f.Value.(string)
			if !ok {
				tx = db.failClosedFilter(tx, or, f, "expects a string value")
				continue
			}
			tx = db.combine(tx, or, datatypes.JSONArrayQuery(f.Column).Contains(s))
		default:
			tx = db.failClosedFilter(tx, or, f, "is unknown")
		}
	}
	return tx
}

// combine adds one condition to tx, joining it with OR when or is set and with
// AND otherwise. gorm treats a leading Or the same as a Where, so a group's
// first condition needs no special case.
func (db *database[M]) combine(tx *gorm.DB, or bool, query any, args ...any) *gorm.DB {
	if or {
		return tx.Or(query, args...)
	}
	return tx.Where(query, args...)
}

// groupCondition builds one filter group as a single nested condition, so the
// conditions outside the group can never be absorbed into it. Its children are
// OR-combined when or is set and AND-combined otherwise; a child may itself be
// a group, which is how arbitrary nesting works.
//
// A group whose value is not a filter list, or that carries no children at
// all, fails closed: an empty group is a caller bug, and answering it with the
// logical identity (TRUE for AND) would widen the result set.
func (db *database[M]) groupCondition(f types.Filter, or bool) any {
	children, ok := f.Value.([]types.Filter)
	if !ok {
		logger.Database.WithContext(db.ctx, consts.Phase("WithQuery")).Warnf("filter group %q expects a filter list value, adding safety condition", f.Op)
		return "1 = 0"
	}
	if len(children) == 0 {
		logger.Database.WithContext(db.ctx, consts.Phase("WithQuery")).Warnf("filter group %q has no children, adding safety condition", f.Op)
		return "1 = 0"
	}
	return db.filterConditions(db.ins.Session(&gorm.Session{NewDB: true}), children, or)
}

// failClosedFilter records why a filter cannot be applied and narrows the
// query to an empty result instead of widening it.
func (db *database[M]) failClosedFilter(tx *gorm.DB, or bool, f types.Filter, msg string) *gorm.DB {
	logger.Database.WithContext(db.ctx, consts.Phase("WithQuery")).Warnf("filter operator %q on column %q %s, adding safety condition", f.Op, f.Column, msg)
	return db.combine(tx, or, "1 = 0")
}

// applyScalarFilter binds a comparison filter whose value must be a scalar;
// nil, slice, and array values fail closed.
func (db *database[M]) applyScalarFilter(tx *gorm.DB, or bool, f types.Filter, clause string) *gorm.DB {
	if f.Value == nil {
		return db.failClosedFilter(tx, or, f, "expects a scalar value")
	}
	if k := reflect.ValueOf(f.Value).Kind(); k == reflect.Slice || k == reflect.Array {
		return db.failClosedFilter(tx, or, f, "expects a scalar value")
	}
	return db.combine(tx, or, clause, f.Value)
}

// applyListFilter binds a set-membership filter whose value must be a slice
// or an array; anything else, including a comma-separated string, fails
// closed. An empty slice keeps the SQL list semantics: IN matches nothing,
// and the result never widens.
func (db *database[M]) applyListFilter(tx *gorm.DB, or bool, f types.Filter, clause string) *gorm.DB {
	if f.Value == nil {
		return db.failClosedFilter(tx, or, f, "expects a slice value")
	}
	if k := reflect.ValueOf(f.Value).Kind(); k != reflect.Slice && k != reflect.Array {
		return db.failClosedFilter(tx, or, f, "expects a slice value")
	}
	return db.combine(tx, or, clause, f.Value)
}

// applyPatternFilter binds a LIKE-family filter; the value must be a string
// and is escaped so the stored value matches literally.
func (db *database[M]) applyPatternFilter(tx *gorm.DB, or bool, f types.Filter, clause, prefix, suffix string) *gorm.DB {
	s, ok := f.Value.(string)
	if !ok {
		return db.failClosedFilter(tx, or, f, "expects a string value")
	}
	return db.combine(tx, or, clause, prefix+escapeLikePattern(s)+suffix)
}

// applyStringFilter binds a filter whose value must be a plain string bound
// as-is (the regex operators).
func (db *database[M]) applyStringFilter(tx *gorm.DB, or bool, f types.Filter, clause string) *gorm.DB {
	s, ok := f.Value.(string)
	if !ok {
		return db.failClosedFilter(tx, or, f, "expects a string value")
	}
	return db.combine(tx, or, clause, s)
}

// likeEscapeClause declares the LIKE escape character used by filters.
// The pipe is chosen over the conventional backslash because
// backslash inside a SQL string literal is itself an escape character in
// MySQL but a plain character in SQLite/PostgreSQL, so no single spelling of
// ESCAPE '\' parses the same way across the supported dialects.
const likeEscapeClause = " ESCAPE '|'"

// likePatternEscaper rewrites a filter value into a literal LIKE
// pattern fragment: client values are literals, not pattern language, so the
// wildcards and the escape character itself are escaped.
var likePatternEscaper = strings.NewReplacer("|", "||", "%", `|%`, "_", `|_`)

func escapeLikePattern(value string) string {
	return likePatternEscaper.Replace(value)
}

// defaultCursorColumn is the column cursor pagination falls back to when the
// caller did not name one. It is the primary key of every framework base
// model, which is the only column guaranteed to be unique and monotonic.
const defaultCursorColumn = "id"

// WithCursor enables cursor-based pagination for efficient large dataset
// traversal. Unlike offset pagination, a cursor read stays at constant cost
// however deep the client has paged, because the boundary is a WHERE
// condition rather than a row count to skip.
//
// The cursor carries the feed's ordering, so it also decides the ORDER BY of
// the query: combining WithCursor with WithOrder produces two competing sort
// sources and breaks the boundary condition, which the list controller
// rejects as a client error.
//
// A cursor without a boundary value is a no-op, so an unpaginated first page
// needs no special case at the call site. A time-typed cursor value is
// formatted as "YYYY-MM-DD HH:MM:SS.ffffff".
//
// Examples:
//
//	WithCursor(types.CursorForward(SampleCols.ID.Asc(), lastID)).WithLimit(10).List(&next)
//	WithCursor(types.CursorBackward(SampleCols.ID.Asc(), firstID)).WithLimit(10).List(&prev)
//	WithCursor(types.CursorForward(SampleCols.CreatedAt.Desc(), lastCreatedAt)).WithLimit(10).List(&older)
func (db *database[M]) WithCursor(cursor types.Cursor) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()

	if !cursor.Enabled() {
		return db
	}
	if len(cursor.Order.Column) == 0 {
		cursor.Order.Column = defaultCursorColumn
	}
	db.cursor = cursor

	return db
}

// applyCursorPagination applies cursor-based pagination to the query if a
// cursor is set. Traveling backward reads the feed in reverse, so both the
// boundary comparison and the ORDER BY flip; List reverses the rows afterwards
// to hand them back in the feed's own order.
func (db *database[M]) applyCursorPagination() {
	if !db.cursor.Enabled() {
		return
	}
	direction := db.cursor.Order.Direction
	if db.cursor.Backward {
		direction = direction.Flip()
	}
	comparison := " > ?"
	if direction == types.OrderDesc {
		comparison = " < ?"
	}
	db.ins = db.ins.Where(db.quoteOrderField(db.cursor.Order.Column)+comparison, db.cursor.Value)
	db.ins = db.ins.Order(db.orderClause(types.Order{Column: db.cursor.Order.Column, Direction: direction}))
}

// WithSelect specifies fields to select when querying or updating records.
// The method automatically includes defaultsColumns (id, created_by, updated_by, created_at, updated_at, deleted_at)
// in addition to the specified columns to ensure essential fields are always available.
// Empty or whitespace-only column names are filtered out, and duplicate defaultsColumns are avoided.
//
// Parameters:
//   - columns: Field names to select (defaultsColumns will be automatically added)
//     If no columns are provided, this is a no-op operation and no columns will be selected (returns all columns).
//     If all provided columns are defaultsColumns or empty/whitespace, this is also a no-op (returns all columns).
//     Only when valid non-default columns are provided will Select be applied (valid columns + defaultsColumns).
//
// Returns the same database instance for method chaining.
//
// WARNING: Using WithSelect may result in the removal of certain fields from table records
// if there are multiple hooks in the service and model layers. Use with caution.
//
// Affected operations: Update, List, Get, First, Last, Take.
func (db *database[M]) WithSelect(columns ...string) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()
	if len(columns) == 0 {
		// No-op: return without selecting any columns
		return db
	}
	_columns := make([]string, 0)
	for i := range columns {
		col := strings.TrimSpace(columns[i])
		if len(col) > 0 && !contains(defaultsColumns, col) {
			_columns = append(_columns, col)
		}
	}
	// db.ins = db.ins.Select(append(_columns, defaultsColumns...))
	if len(_columns) == 0 {
		return db
	}
	db.selectColumns = append(db.selectColumns, _columns...)
	db.selectColumns = append(db.selectColumns, defaultsColumns...)
	return db
}

// WithLock adds row-level locking to the query for concurrent access control.
// Uses SELECT ... FOR UPDATE to prevent other transactions from modifying selected rows.
// Must be used within database.Transaction to be effective; outside a
// transaction it logs a warning because row locks are released as soon as the
// statement finishes.
//
// Important: WithLock only applies to SELECT queries (Get, First, List, etc.).
// It does not work with Create, Update, or Delete operations.
//
// Lock modes:
//   - consts.LockUpdate (default): SELECT ... FOR UPDATE
//   - consts.LockShare: SELECT ... FOR SHARE
//   - consts.LockUpdateNoWait: SELECT ... FOR UPDATE NOWAIT
//   - consts.LockShareNoWait: SELECT ... FOR SHARE NOWAIT
//   - consts.LockUpdateSkipLocked: SELECT ... FOR UPDATE SKIP LOCKED
//   - consts.LockShareSkipLocked: SELECT ... FOR SHARE SKIP LOCKED
//
// Example:
//
//	err := database.Transaction(ctx, func(ctx context.Context) error {
//	    // Get and lock the record with FOR UPDATE
//	    record := new(model.Sample)
//	    if err := database.Database[*model.Sample](ctx).
//	        WithLock(consts.LockUpdate).
//	        Get(record, recordID); err != nil {
//	        return err
//	    }
//	    // Update the locked record
//	    record.Status = "processed"
//	    return database.Database[*model.Sample](ctx).Update(record)
//	})
func (db *database[M]) WithLock(mode ...consts.LockMode) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Row locks outside a transaction are released as soon as the statement
	// finishes, which silently defeats the purpose of WithLock. Warn instead of
	// failing so read paths keep working, but the caller should wrap the
	// operation in database.Transaction.
	if _, ok := txFromContext(db.ctx); !ok {
		logger.Database.WithContext(db.ctx, consts.Phase("WithLock")).Warn(
			"WithLock used outside a transaction; locks are released immediately, wrap the operation in database.Transaction",
		)
	}

	strength := "UPDATE"
	options := ""
	if len(mode) > 0 {
		switch mode[0] {
		case consts.LockShare:
			strength = "SHARE"
		case consts.LockUpdateNoWait:
			strength = "UPDATE"
			options = "NOWAIT"
		case consts.LockShareNoWait:
			strength = "SHARE"
			options = "NOWAIT"
		case consts.LockUpdateSkipLocked:
			strength = "UPDATE"
			options = "SKIP LOCKED"
		case consts.LockShareSkipLocked:
			strength = "SHARE"
			options = "SKIP LOCKED"
		}
	}

	db.ins = db.ins.Clauses(clause.Locking{
		Strength: strength,
		Options:  options,
	})
	return db
}

// WithOrder adds ORDER BY terms to sort query results (List, Get, First,
// Last, etc.). Terms apply in the order they are given, so the first one is
// the primary sort key.
//
// Orders are built from the generated column references, which cannot name a
// column the model does not have; the types.Asc and types.Desc constructors
// take a plain column name for code that cannot reference a concrete model.
// Column names are quoted with dialect-appropriate identifiers, and the
// direction comes from a closed set, so neither part can carry SQL.
//
// Examples:
//
//	WithOrder(SampleCols.Name.Asc())                          // ORDER BY `name` ASC
//	WithOrder(SampleCols.CreatedAt.Desc())                    // ORDER BY `created_at` DESC
//	WithOrder(SampleCols.Age.Desc(), SampleCols.Name.Asc())   // ORDER BY `age` DESC, `name` ASC
//	WithOrder(types.Desc("created_at"))                       // same, by column name
//
// Calling WithOrder without any term, or with a term whose column is empty,
// adds nothing.
func (db *database[M]) WithOrder(orders ...types.Order) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()
	for _, order := range orders {
		if len(order.Column) == 0 {
			continue
		}
		db.ins = db.ins.Order(db.orderClause(order))
	}
	return db
}

// orderClause renders one ORDER BY term. A zero direction is read as
// ascending, matching SQL's own default.
func (db *database[M]) orderClause(order types.Order) string {
	direction := types.OrderAsc
	if order.Direction == types.OrderDesc {
		direction = types.OrderDesc
	}
	return db.quoteOrderField(order.Column) + " " + string(direction)
}

// WithPagination applies pagination parameters to the query.
// It calculates the offset based on the page and size parameters and applies
// the OFFSET and LIMIT clauses to the query.
//
// Parameters:
//   - page: The page number (1-based). If page <= 0, it defaults to 1.
//   - size: The number of records per page. If size <= 0, it defaults to defaultLimit.
//
// Examples:
//   - pageStr, _ := c.GetQuery("_page")
//     sizeStr, _ := c.GetQuery("_size")
//     page, _ := strconv.Atoi(pageStr)
//     size, _ := strconv.Atoi(sizeStr)
//     WithPagination(page, size)
func (db *database[M]) WithPagination(page, size int) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = defaultLimit
	}
	offset := (page - 1) * size
	db.ins = db.ins.Scopes(func(d *gorm.DB) *gorm.DB {
		return d.Offset(offset).Limit(size)
	})
	return db
}

// WithLimit adds LIMIT clause to restrict the number of returned records.
// Used for pagination and controlling result set size.
//
// Parameters:
//   - limit: Maximum number of records to return.
//     If limit <= 0, uses defaultLimit (-1, unlimited) to return all records.
//
// Returns the same database instance for method chaining.
//
// Example:
//
//	WithLimit(10)  // Return at most 10 records
//	WithLimit(100).WithOffset(20)  // Pagination: skip 20, take 100
//	WithLimit(0)   // Returns all records (unlimited)
//
// Note: WithLimit only affects SELECT queries (List, Get, First, Last, etc.).
// GORM ignores Limit clause in Create, Update, and Delete operations for cross-database
// compatibility, as INSERT statements don't support LIMIT in most databases.
func (db *database[M]) WithLimit(limit int) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()
	if limit <= 0 {
		limit = defaultLimit
	}
	db.ins = db.ins.Limit(limit)
	return db
}

// WithOffset adds OFFSET clause to skip records before returning query results.
// Used together with WithLimit for offset-based pagination.
//
// Parameters:
//   - offset: Number of records to skip. If offset <= 0, the offset clause is cleared.
//
// Returns the same database instance for method chaining.
//
// Example:
//
//	WithOffset(20).WithLimit(10)  // Skip 20 records and return at most 10 records
//	WithOffset(0)                // Clears any previous offset
//
// Note: WithOffset only affects SELECT queries (List, Get, First, Last, etc.).
func (db *database[M]) WithOffset(offset int) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()
	if offset <= 0 {
		offset = -1
	}
	db.ins = db.ins.Offset(offset)
	return db
}

// WithExpand enables eager loading of specified associations.
// Preloads related data to avoid N+1 query problems.
// It uses GORM's Preload functionality to load associated data in a single query.
//
// Parameters:
//   - expand: Slice of relationship names to preload (e.g., ["Children", "Parent"])
//     Nested relationships can be specified using dot notation (e.g., ["Parent.Parent", "Children.Children"])
//   - orders: Optional ordering for the preloaded relationships. The columns must
//     exist on the associated table, which a self-referencing tree guarantees.
//
// Behavior:
//   - Supports nested relationships using dot notation (e.g., "Parent.Parent")
//   - Automatically expands intermediate relationships for nested paths
//   - If specified depth exceeds available relationships, only expands available depth
//   - Association names are case sensitive
//   - Only works with GORM foreign key relationships
//
// Example:
//
//	// Load user with their posts
//	db.WithExpand([]string{"Posts"})
//
//	// Load user with posts ordered by creation date
//	db.WithExpand([]string{"Posts"}, types.Desc("created_at"))
//
//	// Load nested relationships
//	db.WithExpand([]string{"Posts.Comments", "Profile"})
//
//	// Load category with parent and children (two levels)
//	db.WithExpand([]string{"Parent.Parent", "Children.Children"})
//
// Note: WithExpand only affects SELECT queries (List, Get, First, Last, etc.).
// It does not work with Create, Update, or Delete operations.
// Note: For custom fields without GORM foreign key definitions, use GetAfter/ListAfter hooks instead.
func (db *database[M]) WithExpand(expand []string, orders ...types.Order) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()
	// The order terms sort the preloaded rows of each association, so their
	// columns must exist on the associated table; a self-referencing tree,
	// where parent and child share the schema, is the case this serves.
	withOrder := func(preload *gorm.DB) *gorm.DB {
		for _, order := range orders {
			if len(order.Column) == 0 {
				continue
			}
			preload = preload.Order(db.orderClause(order))
		}
		return preload
	}
	// FIXME: 前端加了 _depth 查询参数, 但是层数不匹配就无法递归排序,
	// _depth 的作用:
	// _depth = 2: Children -> Children.Children
	// _depth = 3: Children -> Children.Children.Children
	// 假设一共有3层, 但是 _depth=5, 则无法递归排序
	//
	// 解决办法:
	// 假设: [Children.Children.Children, Parent]
	// 以前:
	//      db.db = db.db.Preload("Children.Children.Children", withOrder)
	//      db.db = db.db.Preload("Parent", withOrder)
	// 现在: (递归 Children)
	//      db.db = db.db.Preload("Children", withOrder)
	//      db.db = db.db.Preload("Children.Children", withOrder)
	//      db.db = db.db.Preload("Children.Children.Children", withOrder)
	//      db.db = db.db.Preload("Parent", withOrder)

	for i := range expand {
		// preload 排序问题
		// https://www.jianshu.com/p/a88fb2d4b2ef
		// https://gorm.io/docs/preload.html#Custom-Preloading-SQL

		items := strings.Split(expand[i], ".")
		switch len(items) {
		case 0:
		case 1:
			db.ins = db.ins.Preload(expand[i], withOrder)
		default:
			for j := range items {
				// fmt.Println("================== ", strings.Join(items[0:j+1], "."))
				db.ins = db.ins.Preload(strings.Join(items[0:j+1], "."), withOrder)
			}
		}
	}

	return db
}

// WithExclude excludes records that match specified conditions.
// It adds NOT conditions to the query to filter out records with matching values.
// Multiple fields can be excluded, and each field can have multiple values to exclude.
//
// Parameters:
//   - excludes: Map where keys are field names and values are slices of values to exclude.
//     Empty map will not filter any records.
//
// Behavior:
//   - Multiple values for the same field are combined with OR logic (exclude if matches any value)
//   - Multiple fields add separate NOT conditions, so a record is excluded if it matches any excluded filter
//   - Empty exclude map has no effect
//
// Example:
//
//	// Exclude users with specific IDs
//	excludes := map[string][]any{
//		"id": {"user1", "user2", "user3"},
//	}
//	db.WithExclude(excludes).List(&users)
//
//	// Exclude users with specific IDs and names (AND logic)
//	excludes := map[string][]any{
//		"id":   {"user1", "user2"},
//		"name": {"admin", "root"},
//	}
//	db.WithExclude(excludes).List(&users)
//
// Note: This method affects the WHERE clause, not the SELECT clause.
// Use WithOmit() to exclude fields from SELECT queries.
// Note: WithExclude affects SELECT queries (List, Get, First, Last, etc.) and
// also affects Update and Delete operations by adding NOT conditions to WHERE clause.
// It does not affect Create operations (INSERT statements don't support WHERE clause).
func (db *database[M]) WithExclude(excludes map[string][]any) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()
	for k, v := range excludes {
		db.ins = db.ins.Not(k, v)
	}
	return db
}

// WithPurge explicitly controls whether to permanently delete records (hard delete).
// This option has the HIGHEST PRIORITY and overrides the model's default Purge() behavior.
//
// Priority order:
//  1. WithPurge() - explicitly set by user (highest priority)
//  2. model.Purge() - default behavior defined in the model
//  3. false - framework default (soft delete)
//
// Parameters:
//   - enable: Optional boolean flag (default: true if omitted)
//   - true: Hard delete (permanent deletion, bypasses soft delete)
//   - false: Soft delete (only updates deleted_at field)
//
// Usage:
//
//	WithPurge().Delete(&user)        // Hard delete (enable=true by default)
//	WithPurge(true).Delete(&user)    // Hard delete (explicit)
//	WithPurge(false).Delete(&user)   // Soft delete (explicit, overrides model.Purge())
//
// WARNING: Hard delete will permanently remove data from the database and cannot be undone.
// Only works on 'Delete' method.
func (db *database[M]) WithPurge(enable ...bool) types.Database[M] {
	_enable := true
	if len(enable) > 0 {
		_enable = enable[0]
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	db.enablePurge = new(_enable)
	return db
}

// WithOmit excludes specified fields from INSERT, UPDATE, and SELECT operations.
// Useful for skipping auto-generated fields or fields that shouldn't be modified.
//
// Parameters:
//   - columns: Field names to omit from the operation
//
// Behavior:
//   - Create/Update: Excludes specified fields from INSERT/UPDATE statements
//   - Query operations (List, Get, First, Last, Take): Excludes specified fields from SELECT statements
//   - Delete: Not affected (delete operations are based on WHERE conditions, not fields)
//   - Count: Not affected (counts records, not fields)
//
// Example:
//
//	WithOmit("created_at", "updated_at").Create(&user)  // Skip timestamp fields on create
//	WithOmit("id").Update(&user)                        // Skip ID field during update
//	WithOmit("password").List(&users)                   // Exclude password from query results
//	WithOmit("sensitive_data").Get(&user, id)          // Exclude sensitive data from query
//	WithOmit("name", "age").Delete(&user)              // Delete works normally (WithOmit has no effect)
func (db *database[M]) WithOmit(columns ...string) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.ins = db.ins.Omit(columns...)
	return db
}
