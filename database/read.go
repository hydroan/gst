package database

import (
	"context"
	"reflect"
	"slices"

	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// nilModel reports whether a read destination is unusable: a nil or invalid
// model pointer cannot be filled by the query.
func nilModel[M types.Model](dest M) bool {
	val := reflect.ValueOf(dest)
	return !val.IsValid() || val.IsNil()
}

// applySelect narrows the read to the selected columns when WithSelect chose
// any; see WithSelect for the default columns that ride along.
func (db *database[M]) applySelect() {
	if len(db.selectColumns) > 0 {
		db.ins = db.ins.Select(db.selectColumns)
	}
}

// List retrieves multiple records from the database based on applied conditions.
// Returns all records if no conditions are specified, or filtered records with WithQuery.
// Supports pagination, sorting, and eager loading of associations.
//
// Parameters:
//   - dest: Pointer to the result slice. The pointer itself must not be nil.
//     The slice value may be nil or preallocated with make. List fully replaces
//     the slice contents with the query result and never merges into or appends
//     onto whatever dest already holds: the underlying GORM Find resets the slice
//     length to 0 before scanning rows, so pre-existing elements are discarded.
//     After a successful call len(*dest) equals the number of rows returned.
//     A "dirty" or reused dest therefore cannot leak stale rows into the result,
//     but callers should still pass an empty slice: the ListBefore model hook runs
//     over the pre-existing elements before the query overwrites them, so leftover
//     data would trigger useless hook invocations.
//
// Features:
//   - Supports pagination with WithLimit/WithOffset
//   - Supports sorting with WithOrder
//   - Supports filtering with WithQuery
//   - Supports eager loading with WithExpand
//
// Example:
//
//	var users []*User
//	List(&users)  // Get all users
//
//	users := make([]*User, 0)
//	WithQuery(&User{Status: "active"}).List(&users)  // Get active users
//	WithLimit(10).WithOffset(20).List(&users)  // Paginated results
func (db *database[M]) List(dest *[]M) (err error) {
	defer db.reset()

	if err = db.prepare(); err != nil {
		return err
	}
	done, span := db.trace(consts.PHASE_LIST)
	defer func() { done(err) }()
	if dest == nil {
		return ErrNilDest
	}

	db.applySelect()
	if db.dryRun {
		tableName := db.m.GetTableName()
		db.applyCursorPagination()
		tx := dryRunSession(db.ins).Table(tableName).Find(dest)
		return db.collectSQL(tx)
	}
	// Invoke model hook: ListBefore.
	if !db.noHook {
		if err = traceModelHook[M](db.ctx, consts.PHASE_LIST_BEFORE, span, func(spanCtx context.Context) error {
			for i := range *dest {
				if !nilModel((*dest)[i]) {
					if err = (*dest)[i].ListBefore(spanCtx); err != nil {
						return err
					}
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	tableName := db.m.GetTableName()
	// apply cursor-based pagination.
	db.applyCursorPagination()
	if err = db.ins.Table(tableName).Find(dest).Error; err != nil {
		return err
	}
	// A backward cursor read walks the feed in reverse, so the rows come back
	// upside down; reversing them restores the feed's own order.
	if db.cursor.Enabled() && db.cursor.Backward {
		slices.Reverse(*dest)
	}

	// Invoke model hook: ListAfter()
	if !db.noHook {
		if err = traceModelHook[M](db.ctx, consts.PHASE_LIST_AFTER, span, func(spanCtx context.Context) error {
			for i := range *dest {
				if !nilModel((*dest)[i]) {
					if err = (*dest)[i].ListAfter(spanCtx); err != nil {
						return err
					}
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

// Get retrieves a single record from the database by its primary key (ID).
// Executes GetBefore and GetAfter model hooks unless disabled with WithoutHook.
//
// Parameters:
//   - dest: Non-nil pointer where the result will be stored. When M is *T,
//     both &value and new(T) are valid destinations; a nil *T returns ErrNilDest.
//   - id: Primary key value of the record to retrieve
//
// Returns ErrIDRequired if id is empty. Returns ErrRecordNotFound when no
// matching record exists.
//
// Features:
//   - Supports eager loading with WithExpand
//   - Supports field selection with WithSelect
//
// Destination forms when M is *T:
//   - &value, where value is an addressable T
//   - new(T)
//
// Do not pass a nil *T.
func (db *database[M]) Get(dest M, id string) (err error) {
	defer db.reset()

	if nilModel(dest) {
		return ErrNilDest
	}
	if len(id) == 0 {
		return ErrIDRequired
	}
	// Normalize the id through the model's own ID semantics before it can
	// reach SQL. An id the model rejects cannot match any row, and answering
	// "record not found" here keeps the database from applying implicit
	// string-to-integer coercion on integer primary keys (MySQL matches id=7
	// for '7abc'). Base accepts any non-empty string and passes through
	// unchanged; AutoBase only accepts decimal digits. The probe clones dest,
	// which must stay untouched until the query fills it.
	var ok bool
	if id, ok = normalizeModelID(dest, id); !ok {
		return ErrRecordNotFound
	}
	if err = db.prepare(); err != nil {
		return err
	}
	done, span := db.trace(consts.PHASE_GET)
	defer func() { done(err) }()

	db.applySelect()
	if db.dryRun {
		tableName := db.m.GetTableName()
		dryRunDest := cloneDryRunModel(dest)
		dryRunDest.ClearID()
		if len(tableName) == 0 {
			tx := dryRunSession(db.ins).Where("id = ?", id).Find(dryRunDest)
			return db.collectSQL(tx)
		}
		tx := dryRunSession(db.ins).Table(tableName).Where(db.quoteTableColumn(tableName, "id")+" = ?", id).Find(dryRunDest)
		return db.collectSQL(tx)
	}
	// Invoke model hook: GetBefore.
	if !db.noHook {
		if err = traceModelHook[M](db.ctx, consts.PHASE_GET_BEFORE, span, func(spanCtx context.Context) error {
			return dest.GetBefore(spanCtx)
		}); err != nil {
			return err
		}
	}
	tableName := db.m.GetTableName()
	// Use an explicit WHERE clause instead of relying on primary key fields
	// already present on dest.
	dest.ClearID()
	tx := db.ins.Table(tableName).Where(db.quoteTableColumn(tableName, "id")+" = ?", id).Find(dest)
	if err = tx.Error; err != nil {
		return err
	}
	if tx.RowsAffected == 0 {
		return ErrRecordNotFound
	}
	// Invoke model hook: GetAfter.
	if !db.noHook {
		if err = traceModelHook[M](db.ctx, consts.PHASE_GET_AFTER, span, func(spanCtx context.Context) error {
			return dest.GetAfter(spanCtx)
		}); err != nil {
			return err
		}
	}
	return nil
}

// Count returns the total number of records matching the current query conditions.
// Applies all previously set query conditions (WHERE, JOIN, etc.) to the count operation.
//
// Parameters:
//   - count: Pointer to int where the result count will be stored
//
// Returns database errors if the query fails.
//
// Features:
//   - Respects query modifiers such as WHERE and JOIN
//   - Clears LIMIT and OFFSET so that paging left on the chain cannot reach the
//     count query. A count query answers a single row, which an OFFSET would
//     skip and turn into a silent zero.
//
// Example:
//
//	var total int
//	WithQuery(&User{Status: "active"}).Count(&total)  // Count active records
//	WithQuery(&User{Name: "john"}).Count(&total)      // Count records matching name
//
// Note: The count parameter must be a non-nil pointer to int.
func (db *database[M]) Count(count *int) (err error) {
	defer db.reset()

	if count == nil {
		return ErrNilCount
	}
	if err = db.prepare(); err != nil {
		return err
	}
	done, _ := db.trace(phaseCount)
	defer func() { done(err) }()

	// GORM's Count only accepts *int64, so bridge through a local variable.
	var count64 int64
	if db.dryRun {
		tableName := db.m.GetTableName()
		tx := dryRunSession(db.ins).Table(tableName).Model(*new(M)).Limit(-1).Offset(-1).Count(&count64)
		return db.collectSQL(tx)
	}
	tableName := db.m.GetTableName()
	if err = db.ins.Table(tableName).Model(*new(M)).Limit(-1).Offset(-1).Count(&count64).Error; err != nil {
		return err
	}
	*count = int(count64)
	return nil
}

// First retrieves the first record from the database ordered by primary key.
// Applies all previously set query conditions and returns the first matching record.
//
// Parameters:
//   - dest: Pointer to model instance where the result will be stored
//
// Returns ErrRecordNotFound when no matching record exists; the error is
// raised by the underlying GORM First call. Returns other database errors
// if the query fails.
//
// Features:
//   - Supports all query modifiers (WHERE, ORDER BY, etc.)
//   - Supports eager loading with WithExpand
//   - Supports field selection with WithSelect
//
// Example:
//
//	var user User
//	First(&user)  // Get first user by primary key
//	WithQuery(&User{Status: "active"}).First(&user)  // Get first active user
//	WithOrder(types.Desc("created_at")).First(&user)  // Get newest user
func (db *database[M]) First(dest M) (err error) {
	defer db.reset()

	if nilModel(dest) {
		return ErrNilDest
	}
	if err = db.prepare(); err != nil {
		return err
	}
	done, span := db.trace(phaseFirst)
	defer func() { done(err) }()

	db.applySelect()
	if db.dryRun {
		tableName := db.m.GetTableName()
		tx := dryRunSession(db.ins).Table(tableName).First(dest)
		return db.collectSQL(tx)
	}
	// Invoke model hook: GetBefore
	if !db.noHook {
		if err = traceModelHook[M](db.ctx, consts.PHASE_GET_BEFORE, span, func(spanCtx context.Context) error {
			return dest.GetBefore(spanCtx)
		}); err != nil {
			return err
		}
	}
	tableName := db.m.GetTableName()
	if err = db.ins.Table(tableName).First(dest).Error; err != nil {
		return err
	}
	// Invoke model hook: GetAfter
	if !db.noHook {
		if err = traceModelHook[M](db.ctx, consts.PHASE_GET_AFTER, span, func(spanCtx context.Context) error {
			return dest.GetAfter(spanCtx)
		}); err != nil {
			return err
		}
	}
	return nil
}

// Last retrieves the last record from the database ordered by primary key.
// Applies all previously set query conditions and returns the last matching record.
//
// Parameters:
//   - dest: Pointer to model instance where the result will be stored
//
// Returns ErrRecordNotFound when no matching record exists; the error is
// raised by the underlying GORM Last call. Returns other database errors
// if the query fails.
//
// Features:
//   - Supports all query modifiers (WHERE, ORDER BY, etc.)
//   - Supports eager loading with WithExpand
//   - Supports field selection with WithSelect
//   - Executes GetBefore and GetAfter model hooks unless disabled
//
// Example:
//
//	var user User
//	Last(&user)  // Get last user by primary key
//	WithQuery(&User{Status: "active"}).Last(&user)  // Get last active user
//	WithOrder(types.Asc("created_at")).Last(&user)  // Get oldest user (with custom order)
func (db *database[M]) Last(dest M) (err error) {
	defer db.reset()

	if nilModel(dest) {
		return ErrNilDest
	}
	if err = db.prepare(); err != nil {
		return err
	}
	done, span := db.trace(phaseLast)
	defer func() { done(err) }()

	db.applySelect()
	if db.dryRun {
		tableName := db.m.GetTableName()
		tx := dryRunSession(db.ins).Table(tableName).Last(dest)
		return db.collectSQL(tx)
	}
	// Invoke model hook: GetBefore.
	if !db.noHook {
		if err = traceModelHook[M](db.ctx, consts.PHASE_GET_BEFORE, span, func(spanCtx context.Context) error {
			return dest.GetBefore(spanCtx)
		}); err != nil {
			return err
		}
	}
	tableName := db.m.GetTableName()
	if err = db.ins.Table(tableName).Last(dest).Error; err != nil {
		return err
	}
	// Invoke model hook: GetAfter
	if !db.noHook {
		if err = traceModelHook[M](db.ctx, consts.PHASE_GET_AFTER, span, func(spanCtx context.Context) error {
			return dest.GetAfter(spanCtx)
		}); err != nil {
			return err
		}
	}
	return nil
}

// Take retrieves the first record from the database in no specified order.
// Unlike First/Last which order by primary key, Take returns any matching record.
//
// Parameters:
//   - dest: Pointer to model instance where the result will be stored
//
// Returns ErrRecordNotFound when no matching record exists; the error is
// raised by the underlying GORM Take call. Returns other database errors
// if the query fails.
//
// Features:
//   - Supports all query modifiers (WHERE, JOIN, etc.)
//   - Supports eager loading with WithExpand
//   - Supports field selection with WithSelect
//   - Executes GetBefore and GetAfter model hooks unless disabled
//   - No ordering applied (fastest single record retrieval)
//
// Example:
//
//	var user User
//	Take(&user)  // Get any user record
//	WithQuery(&User{Status: "active"}).Take(&user)  // Get any active user
func (db *database[M]) Take(dest M) (err error) {
	defer db.reset()

	if nilModel(dest) {
		return ErrNilDest
	}
	if err = db.prepare(); err != nil {
		return err
	}
	done, span := db.trace(phaseTake)
	defer func() { done(err) }()

	db.applySelect()
	if db.dryRun {
		tableName := db.m.GetTableName()
		tx := dryRunSession(db.ins).Table(tableName).Take(dest)
		return db.collectSQL(tx)
	}
	// Invoke model hook: GetBefore.
	if !db.noHook {
		if err = traceModelHook[M](db.ctx, consts.PHASE_GET_BEFORE, span, func(spanCtx context.Context) error {
			return dest.GetBefore(spanCtx)
		}); err != nil {
			return err
		}
	}
	tableName := db.m.GetTableName()
	if err = db.ins.Table(tableName).Take(dest).Error; err != nil {
		return err
	}
	// Invoke model hook: GetAfter.
	if !db.noHook {
		if err = traceModelHook[M](db.ctx, consts.PHASE_GET_AFTER, span, func(spanCtx context.Context) error {
			return dest.GetAfter(spanCtx)
		}); err != nil {
			return err
		}
	}
	return nil
}
