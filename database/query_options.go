package database

import (
	"reflect"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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
// needs no special case at the call site. A time-typed cursor value is the
// UTC wall clock formatted as "YYYY-MM-DD HH:MM:SS.ffffff" — UTC is the one
// wall clock the framework stores on every dialect, so a boundary read back
// from a row formats as row.CreatedAt.UTC().
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
		cursor.Order.Column = modelregistry.DefaultCursorColumn
	}
	db.cursor = cursor

	return db
}

// applyCursorPagination applies cursor-based pagination to the query if a
// cursor is set. Traveling backward reads the feed in reverse, so both the
// boundary comparison and the ORDER BY flip; List reverses the rows afterwards
// to hand them back in the feed's own order. A boundary on a time column goes
// through timeComparableExpr on both sides, so the comparison agrees across
// storage spellings.
func (db *database[M]) applyCursorPagination() {
	if !db.cursor.Enabled() {
		return
	}
	direction := db.cursor.Order.Direction
	if db.cursor.Backward {
		direction = direction.Flip()
	}
	operator := " > "
	if direction == types.OrderDesc {
		operator = " < "
	}
	lhs, rhs := db.quoteOrderField(db.cursor.Order.Column), "?"
	if _, isTime := timeColumnSet(reflect.TypeOf(*new(M)))[db.cursor.Order.Column]; isTime {
		lhs, rhs = db.timeComparableExpr(lhs), db.timeComparableExpr(rhs)
	}
	db.ins = db.ins.Where(lhs+operator+rhs, db.cursor.Value)
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
	if len(_columns) == 0 {
		return db
	}
	db.selectColumns = append(db.selectColumns, _columns...)
	db.selectColumns = append(db.selectColumns, defaultsColumns...)
	return db
}

// WithLock adds row-level locking to the query for concurrent access control.
// Uses SELECT ... FOR UPDATE to prevent other transactions from modifying selected rows.
// It requires a transaction. Outside one the lock would be released as soon as
// the statement finished, so the chain fails with ErrLockOutsideTransaction
// rather than returning rows the caller would wrongly believe it holds.
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
// SQLite has no row-level locks -- the whole database is a single writer --
// and its GORM driver drops the locking clause, so every mode is a no-op
// there. The transaction requirement still applies: the code stays portable,
// and the database-level write lock the transaction takes is what serializes
// SQLite writers.
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

	// ClickHouse has no row locks at all — unlike SQLite, where the single
	// writer makes the lock's guarantee hold on its own, here nothing stands
	// in for it — so the chain fails per the capability-miss rule.
	if db.dialect() == dialectClickHouse {
		db.err = errors.Wrap(ErrUnsupportedOnDialect, "WithLock on clickhouse")
		return db
	}

	// A row lock outside a transaction is released the moment the statement
	// finishes, so the query returns rows that nothing is holding: the caller
	// believes it has exclusive access and does not. Whether that is the case is
	// decidable right here, so it is refused rather than warned about — a
	// warning leaves the wrong behavior running and the guarantee to the
	// caller's memory.
	if _, ok := dbruntime.TxFromContext(db.ctx, db.base); !ok {
		db.err = ErrLockOutsideTransaction
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
//
// The clauses land on the chain right away instead of through a GORM scope.
// Scopes run when the statement executes, which is after a terminal operation
// has built its own statement, so a scope would override the LIMIT and OFFSET
// reset that Count applies to keep paging out of a count query.
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
	if offset <= 0 {
		// GORM keeps a previously set offset when merging a zero one, so clear
		// it the same way WithOffset does.
		offset = -1
	}
	db.ins = db.ins.Offset(offset).Limit(size)
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
	// A nested path preloads level by level rather than in one call: the
	// requested depth may exceed the real number of levels, and per-level
	// preloading keeps the ordering applied to every level that does exist.
	for i := range expand {
		items := strings.Split(expand[i], ".")
		switch len(items) {
		case 0:
		case 1:
			db.ins = db.ins.Preload(expand[i], withOrder)
		default:
			for j := range items {
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

// contains checks if a string item exists in a string slice.
// Uses a map-based approach for O(n) time complexity with O(n) space complexity.
// More efficient than linear search for larger slices.
//
// Parameters:
//   - slice: The string slice to search in
//   - item: The string item to search for
//
// Returns true if the item is found, false otherwise.
func contains(slice []string, item string) bool {
	set := make(map[string]struct{}, len(slice))
	for _, s := range slice {
		set[s] = struct{}{}
	}
	_, ok := set[item]
	return ok
}
