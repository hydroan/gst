package database

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
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
//   - config: Optional QueryConfig to control query behavior (fuzzy matching, empty queries, OR logic, raw SQL)
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
//	UseOr:
//	- When true: Combines multiple field conditions with OR instead of AND
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
//	WithQuery(&model.User{Name: "John"}, types.QueryConfig{FuzzyMatch: true})  // WHERE name LIKE '%John%'
//
//	// Fuzzy match - multiple values (REGEXP)
//	WithQuery(&model.User{Name: "John,Jack"}, types.QueryConfig{FuzzyMatch: true})  // WHERE name REGEXP '.*John.*|.*Jack.*'
//
//	// Fuzzy match - empty strings in comma-separated values are skipped
//	WithQuery(&model.User{Name: "John,,Jack"}, types.QueryConfig{FuzzyMatch: true})  // WHERE name REGEXP '.*John.*|.*Jack.*'
//
//	// OR logic to combine conditions
//	WithQuery(&model.User{Name: "John", Email: "john@example.com"}, types.QueryConfig{UseOr: true})
//	// WHERE name IN ('John') OR email IN ('john@example.com')
//
//	// OR logic with fuzzy match
//	WithQuery(&model.User{Name: "John", Email: "example"}, types.QueryConfig{UseOr: true, FuzzyMatch: true})
//	// WHERE name LIKE '%John%' OR email LIKE '%example%'
//
//		// Raw SQL query (can be combined with model fields)
//	WithQuery(&model.User{}, types.QueryConfig{RawQuery: "age > ? AND status = ?", RawQueryArgs: []any{18, "active"}})
//	WithQuery(nil, types.QueryConfig{RawQuery: "created_at BETWEEN ? AND ?", RawQueryArgs: []any{startDate, endDate}})
//	WithQuery(&model.User{Name: "John"}, types.QueryConfig{RawQuery: "age > ?", RawQueryArgs: []any{18}})  // WHERE age > ? AND name IN ('John')
//
//	// Empty query (blocked by default for safety)
//	WithQuery(nil)  // WHERE 1 = 0 (returns no records)
//	WithQuery(&model.User{})  // WHERE 1 = 0 (returns no records)
//	WithQuery(&model.User{Name: "", Email: ""})  // WHERE 1 = 0 (all values are empty)
//
//	// Empty query with AllowEmpty=true (returns all records)
//	WithQuery(nil, types.QueryConfig{AllowEmpty: true})  // Returns all records
//	WithQuery(&model.User{}, types.QueryConfig{AllowEmpty: true})  // Returns all records
//
//	// Query with some empty and some non-empty fields (works normally)
//	WithQuery(&model.User{Name: "John", Email: ""})  // WHERE name IN ('John') (Email is ignored)
//
//	// Combined options
//	WithQuery(&model.User{Name: "John"}, types.QueryConfig{
//	    FuzzyMatch: true,
//	    UseOr:      true,
//	    AllowEmpty: false,
//	})
//
// NOTE: The underlying type must be pointer to struct, otherwise panic will occur.
// NOTE: Empty query conditions (nil or zero value) are blocked by default for safety to prevent
//
//	catastrophic data loss (e.g., deleting all records). Use QueryConfig{AllowEmpty: true} to override.
//
// NOTE: When both RawQuery and model fields are provided, they are combined with AND logic.
// NOTE: REGEXP function may not be available in all databases (e.g., SQLite requires extension).
//
//	For SQLite compatibility, consider using FuzzyMatch with single values (LIKE) or RawQuery.
func (db *database[M]) WithQuery(query M, config ...types.QueryConfig) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Parse query configuration
	var cfg types.QueryConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	// cfg.FuzzyMatch: default false (exact match)
	// cfg.AllowEmpty: default false (block empty queries for safety)

	queryVal := reflect.ValueOf(query)
	// Handle RawQuery first (works even if query is nil)
	// RawQuery will be combined with model fields using AND logic if both are provided
	hasRawQuery := len(cfg.RawQuery) > 0
	if hasRawQuery {
		if cfg.UseOr {
			db.ins = db.ins.Or(cfg.RawQuery, cfg.RawQueryArgs...)
		} else {
			db.ins = db.ins.Where(cfg.RawQuery, cfg.RawQueryArgs...)
		}
	}

	// Check if query is nil or empty
	var empty M
	if queryVal.IsNil() || reflect.DeepEqual(query, empty) {
		// Treat nil/empty as empty query
		// If RawQuery is provided, it's already applied above, so we can return
		// (RawQuery alone is sufficient, no need to check empty query safety)
		if hasRawQuery {
			// RawQuery is already applied, no need to check empty query
			return db
		}
		// No RawQuery and empty query: apply safety check
		if !cfg.AllowEmpty {
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

	structFieldToMap(db.ctx, typ, val, q)
	// fmt.Println("------------- WithQuery", q)

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
	// To allow empty queries, use: WithQuery(nil, QueryConfig{AllowEmpty: true}) or
	//                              WithQuery(&User{}, QueryConfig{AllowEmpty: true})
	if len(q) == 0 {
		// If RawQuery is provided, it's already applied above, so we can return
		// (RawQuery alone is sufficient, no need to check empty query safety)
		if hasRawQuery {
			// RawQuery is already applied, no need to check empty query
			return db
		}
		// No RawQuery and empty query: apply safety check
		if !cfg.AllowEmpty {
			logger.Database.WithContext(db.ctx, consts.Phase("WithQuery")).Warn("all query fields are empty, adding safety condition to prevent matching all records")
			db.ins = db.ins.Where("1 = 0")
			return db
		}
		// AllowEmpty=true: allow matching all records
		logger.Database.WithContext(db.ctx, consts.Phase("WithQuery")).Info("all query fields are empty but AllowEmpty=true, allowing full table scan")
		return db
	}

	if cfg.FuzzyMatch {
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
		isFirstCondition := true
		if len(cfg.RawQuery) > 0 {
			// RawQuery is already applied, no need to check empty query
			isFirstCondition = false
		}
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
				// db.db = db.db.Where(fmt.Sprintf("`%s` REGEXP ?", k), regexpVal)
				if cfg.UseOr && !isFirstCondition {
					db.ins = db.ins.Or(fmt.Sprintf("%s %s ?", db.quoteIdent(k), db.regexpOperator()), regexpVal)
				} else {
					db.ins = db.ins.Where(fmt.Sprintf("%s %s ?", db.quoteIdent(k), db.regexpOperator()), regexpVal)
				}
			} else { // If the query string has only one value, using LIKE
				// db.db = db.db.Where(fmt.Sprintf("`%s` LIKE ?", k), fmt.Sprintf("%%%v%%", v))
				if cfg.UseOr && !isFirstCondition {
					db.ins = db.ins.Or(db.quoteIdent(k)+" LIKE ?", fmt.Sprintf("%%%v%%", v))
				} else {
					db.ins = db.ins.Where(db.quoteIdent(k)+" LIKE ?", fmt.Sprintf("%%%v%%", v))
				}
			}
			isFirstCondition = false
		}
		// CRITICAL: Check if all query values are empty after filtering
		// Even if query map is not empty, all values might be empty strings
		// Example: &User{Name: "", Email: ""} has fields but all values are empty
		if !hasValidCondition {
			if !cfg.AllowEmpty {
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
		isFirstCondition := true
		for k, v := range q {
			items := strings.Split(v, ",")
			if len(strings.Join(items, "")) == 0 {
				continue
			}
			hasValidCondition = true
			// db.db = db.db.Where(fmt.Sprintf("`%s` IN (?)", k), items)
			if cfg.UseOr && !isFirstCondition {
				db.ins = db.ins.Or(db.quoteIdent(k)+" IN ?", items)
			} else {
				db.ins = db.ins.Where(db.quoteIdent(k)+" IN ?", items)
			}
			isFirstCondition = false
		}
		// CRITICAL: Check if all query values are empty after filtering
		// Even if query map is not empty, all values might be empty strings
		// Example: &User{Name: "", Email: ""} has fields but all values are empty
		if !hasValidCondition {
			if !cfg.AllowEmpty {
				logger.Database.WithContext(db.ctx, consts.Phase("WithQuery")).Warn("all query values are empty, adding safety condition to prevent matching all records")
				db.ins = db.ins.Where("1 = 0")
			} else {
				logger.Database.WithContext(db.ctx, consts.Phase("WithQuery")).Info("all query values are empty but AllowEmpty=true, allowing full table scan")
			}
		}
	}
	return db
}

// WithCursor enables cursor-based pagination for efficient large dataset traversal.
// Cursor pagination is more efficient than offset-based pagination for large datasets
// as it avoids performance degradation when skipping many records.
//
// Parameters:
//   - cursorValue: The value of the cursor field from the last record in the previous page.
//     For string fields (like ID), use the field value directly.
//     For time fields, format as "YYYY-MM-DD HH:MM:SS.ffffff".
//     Empty string will be ignored and cursor pagination will be disabled.
//   - next: The direction of pagination.
//   - true: Fetch records after the cursor (next page, ascending order)
//   - false: Fetch records before the cursor (previous page, descending order)
//   - fields: Optional field name(s) to use as cursor. Defaults to "id" if not specified.
//     Currently only the first field is used (multiple fields support is TODO).
//
// Behavior:
//   - When next=true: Returns records where cursorField > cursorValue, ordered by cursorField ASC
//   - When next=false: Returns records where cursorField < cursorValue, ordered by cursorField DESC
//     (results are reversed to maintain original sort order)
//   - Empty cursorValue: Cursor pagination is disabled, returns all records
//   - Default cursor field: "id" if not specified
//
// Example:
//
//	// First page (no cursor)
//	db.Database[*model.User]().WithLimit(10).List(&users)
//
//	// Next page (using last user's ID as cursor)
//	lastID := users[len(users)-1].ID
//	db.Database[*model.User]().WithCursor(lastID, true).WithLimit(10).List(&nextUsers)
//
//	// Next page using custom field (created_at)
//	lastCreatedAt := users[len(users)-1].CreatedAt.Format("2006-01-02 15:04:05.000000")
//	db.Database[*model.User]().WithCursor(lastCreatedAt, true, "created_at").WithLimit(10).List(&nextUsers)
//
//	// Previous page
//	firstID := users[0].ID
//	db.Database[*model.User]().WithCursor(firstID, false).WithLimit(10).List(&prevUsers)
func (db *database[M]) WithCursor(cursorValue string, next bool, fields ...string) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()

	if len(cursorValue) == 0 {
		return db
	}

	db.enableCursor = true
	db.cursorValue = cursorValue
	db.cursorNext = next

	// TODO: support multiple cursor fields
	if len(fields) > 0 {
		db.cursorField = fields[0]
	}
	// Default cursor field is "id" if not specified
	if db.cursorField == "" {
		db.cursorField = "id"
	}

	return db
}

// applyCursorPagination applies cursor-based pagination to the query if cursor is set.
func (db *database[M]) applyCursorPagination() {
	if db.enableCursor {
		// Apply cursor condition based on direction
		if db.cursorNext {
			// Next page: get records after the cursor
			db.ins = db.ins.Where(db.quoteIdent(db.cursorField)+" > ?", db.cursorValue)
			// Order by cursor field ascending for next page
			db.ins = db.ins.Order(db.quoteIdent(db.cursorField) + " ASC")
		} else {
			// Previous page: get records before the cursor
			db.ins = db.ins.Where(db.quoteIdent(db.cursorField)+" < ?", db.cursorValue)
			// Order by cursor field descending for previous page
			db.ins = db.ins.Order(db.quoteIdent(db.cursorField) + " DESC")
		}
	}
}

// WithTimeRange filters records within a specific time range.
// Supports flexible time range queries:
//   - Both times provided: uses BETWEEN clause
//   - Only startTime provided (endTime is zero): uses >= clause
//   - Only endTime provided (startTime is zero): uses <= clause
//   - Both times are zero: returns without filtering
//
// Parameters:
//   - columnName: The name of the time column to filter on
//   - startTime: The start time of the range (inclusive). Use zero value to ignore.
//   - endTime: The end time of the range (inclusive). Use zero value to ignore.
//
// Examples:
//
//	// Range query: created_at BETWEEN start AND end
//	WithTimeRange("created_at", time.Now().AddDate(0, -1, 0), time.Now())
//
//	// After query: created_at >= start
//	WithTimeRange("created_at", time.Now().AddDate(0, -1, 0), time.Time{})
//
//	// Before query: created_at <= end
//	WithTimeRange("created_at", time.Time{}, time.Now())
func (db *database[M]) WithTimeRange(columnName string, startTime time.Time, endTime time.Time) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()
	if len(columnName) == 0 {
		return db
	}

	startIsZero := startTime.IsZero()
	endIsZero := endTime.IsZero()

	// Both times are zero, no filtering
	if startIsZero && endIsZero {
		return db
	}

	// Both times provided, use BETWEEN
	if !startIsZero && !endIsZero {
		db.ins = db.ins.Where(db.quoteIdent(columnName)+" BETWEEN ? AND ?", startTime, endTime)
		return db
	}

	// Only start time provided, use >=
	if !startIsZero && endIsZero {
		db.ins = db.ins.Where(db.quoteIdent(columnName)+" >= ?", startTime)
		return db
	}

	// Only end time provided, use <=
	if startIsZero && !endIsZero {
		db.ins = db.ins.Where(db.quoteIdent(columnName)+" <= ?", endTime)
		return db
	}

	return db
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
// Must be used within a transaction (Transaction or TransactionFunc) to be effective.
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
// Example with Transaction:
//
//	err := database.Database[*model.User](context.Background()).Transaction(func(tx types.Database[*model.User]) error {
//	    // Get and lock user with FOR UPDATE
//	    user := new(model.User)
//	    if err := tx.WithLock(consts.LockUpdate).Get(user, userID); err != nil {
//	        return err
//	    }
//	    // Update the locked user
//	    user.Name = "updated"
//	    return tx.Update(user)
//	})
//
// Example with TransactionFunc:
//
//	err := database.Database[*model.Order](context.Background()).TransactionFunc(func(tx any) error {
//	    // Get and lock order with FOR UPDATE NOWAIT
//	    order := new(model.Order)
//	    if err := database.Database[*model.Order](context.Background()).
//	        WithTx(tx).
//	        WithLock(consts.LockUpdateNoWait).
//	        Get(order, orderID); err != nil {
//	        return err
//	    }
//	    // Update the locked order
//	    order.Status = "processed"
//	    return database.Database[*model.Order](context.Background()).WithTx(tx).Update(order)
//	})
func (db *database[M]) WithLock(mode ...consts.LockMode) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()

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

// WithOrder adds ORDER BY clause to sort query results (List, Get, First, Last, etc.).
// Supports multiple sorting criteria and directions (ASC/DESC).
// Column names are automatically quoted with dialect-appropriate identifiers to handle SQL keywords.
//
// Parameters:
//   - order: Column name(s) with optional direction. Multiple columns separated by commas.
//     Direction can be "ASC" (default) or "DESC" (case-insensitive).
//
// Examples:
//
//	WithOrder("name")                        // Sort by name ascending (default)
//	WithOrder("name ASC")                    // Sort by name ascending (explicit)
//	WithOrder("name asc")                    // Sort by name ascending (case-insensitive)
//	WithOrder("created_at DESC")             // Sort by creation date descending
//	WithOrder("created_at desc")             // Sort by creation date descending (case-insensitive)
//	WithOrder("priority DESC, name ASC")     // Multiple sort criteria
//	WithOrder("priority desc, name asc")     // Multiple sort criteria (case-insensitive)
//	WithOrder("order DESC, limit ASC")       // Handles SQL keywords safely
//
// Note:
//   - Column names are automatically escaped with dialect-specific quotes to prevent SQL injection
//     and handle reserved keywords like "order", "limit", etc.
//   - Direction keywords (ASC/DESC) are case-insensitive and will be converted to uppercase.
func (db *database[M]) WithOrder(order string) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()
	// 可以多多个字段进行排序, 每个字段之间通过逗号分隔.
	// order 的值比如: "field1, field2 desc, field3 asc"
	// 字段会根据不同数据库方言进行引用, 以避免关键字冲突.
	items := strings.SplitSeq(order, ",")
	for item := range items {
		if len(item) > 0 {
			items := strings.Fields(item)
			for i := range items {
				if strings.EqualFold(items[i], "asc") || strings.EqualFold(items[i], "desc") {
					items[i] = strings.ToUpper(items[i])
				} else {
					items[i] = db.quoteOrderField(items[i])
				}
			}
			db.ins = db.ins.Order(strings.Join(items, " "))
			// fmt.Printf("====== %q\n", strings.Join(items, " "))
		}
	}
	return db
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
//   - pageStr, _ := c.GetQuery("page")
//     sizeStr, _ := c.GetQuery("size")
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
//   - order: Optional ordering for the preloaded relationships (e.g., "created_at desc")
//     The first field in the order string will be wrapped with backticks to handle SQL keywords properly.
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
//	db.WithExpand([]string{"Posts"}, "created_at desc")
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
func (db *database[M]) WithExpand(expand []string, order ...string) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()
	var _orders string
	if len(order) > 0 {
		if len(order[0]) > 0 {
			items := strings.Fields(order[0])
			// The first item is the sort field, must be wrapped with backticks
			// because the sort string might be a SQL keyword
			// The second item might be "desc" etc., which doesn't need backticks
			items[0] = "`" + items[0] + "`"
			_orders = strings.Join(items, " ")
		}
	}
	withOrder := func(db *gorm.DB) *gorm.DB {
		if len(_orders) > 0 {
			return db.Order(_orders)
		}
		return db
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
//   - Multiple fields add separate NOT conditions, so a record is excluded if it matches any excluded field condition
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

// WithCache enables query result caching to improve performance.
// Improves performance by storing frequently accessed data in memory.
//
// Parameters:
//   - enable: Optional boolean flag (default: true if omitted)
//   - true: Enable caching for query operations (List, Get, Count, First, Last, Take)
//   - false: Disable caching, always query from database
//
// Behavior:
//   - When enabled, query results are cached and subsequent identical queries return cached data
//   - Cache is automatically cleared on Create, Update, Delete operations
//   - Only affects query operations (List, Get, Count, First, Last, Take)
//   - Does not affect Create, Update, Delete operations (they clear cache instead)
//
// Example:
//
//	WithCache().List(&users)        // Enable cache (default)
//	WithCache(true).Get(&user, id)  // Enable cache explicitly
//	WithCache(false).List(&users)   // Disable cache, always query database
//
// WithCache will make query operations check cache first.
// If cache not found or expired, query from database directly.
func (db *database[M]) WithCache(enable ...bool) types.Database[M] {
	_enable := true
	if len(enable) > 0 {
		_enable = enable[0]
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	db.enableCache = _enable
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
