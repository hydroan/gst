package database

import (
	"context"
	"reflect"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"gorm.io/gorm"
)

// compactModels drops the zero-value models from a write's input. Writes
// accept variadic models, so an empty call and explicitly zero elements both
// mean "nothing to write" rather than an error; every write entry point runs
// its input through here before touching the database.
func compactModels[M types.Model](objs []M) []M {
	if len(objs) == 0 {
		return nil
	}
	var empty M
	compacted := make([]M, 0, len(objs))
	for _, obj := range objs {
		if reflect.DeepEqual(obj, empty) {
			continue
		}
		compacted = append(compacted, obj)
	}
	return compacted
}

// Create inserts one or multiple records into the database.
// It is a pure INSERT: a record whose primary key or any unique key collides
// with an existing row fails with ErrDuplicatedKey instead of silently
// updating that row. Use Upsert for deliberate insert-or-update semantics.
// Executes CreateBefore and CreateAfter model hooks unless disabled with WithoutHook or WithDryRun.
// Supports batch processing for large datasets using configurable batch sizes.
//
// Parameters:
//   - objs: One or more model instances to create. Empty objects are automatically filtered out.
//
// Behavior:
//   - Automatically generates ID if empty using SetID()
//   - Forces created_at and updated_at to the current time. Values carried by
//     objs are deliberately ignored: HTTP controllers bind client JSON straight
//     into models, so honoring caller-supplied timestamps would let clients
//     forge audit fields.
//   - Runs hooks and all batches in one transaction: a failure in any batch or
//     hook rolls back the whole call (all-or-nothing), joining the transaction
//     carried by ctx when present.
//   - Returns nil if no valid objects provided (empty slice or all objects are empty)
//
// Returns ErrDuplicatedKey when a primary or unique key already exists, or an
// error when hooks or other database constraints fail.
// WithDryRun builds SQL only and does not execute hooks, database I/O, or object field filling.
//
// On a ClickHouse instance the contract is weaker (see clickhouseCreate):
// plain batch INSERTs with no hooks, no transaction across batches, and no
// ErrDuplicatedKey — ClickHouse has no unique constraints, so duplicates are
// stored.
//
// Example:
//
//	Create(&User{Name: "John", Email: "john@example.com"})  // Create single record
//	Create(user1, user2, user3)  // Batch create multiple records
func (db *database[M]) Create(objs ...M) (err error) {
	defer db.reset()

	objs = compactModels(objs)
	if len(objs) == 0 {
		return nil
	}

	if err = db.prepare(); err != nil {
		return err
	}
	if db.dialect() == dialectClickHouse {
		return db.clickhouseCreate(objs)
	}
	done, span := db.trace("Create", len(objs))
	defer func() { done(err) }()

	if db.dryRun {
		tableName := db.m.GetTableName()
		batchSize := defaultBatchSize
		if db.batchSize > 0 {
			batchSize = db.batchSize
		}
		dryRunObjs := cloneDryRunModels(objs)
		for i := 0; i < len(dryRunObjs); i += batchSize {
			end := min(i+batchSize, len(dryRunObjs))
			tx := db.ins.Session(&gorm.Session{DryRun: true}).Table(tableName).Create(dryRunObjs[i:end])
			if err = db.collectSQL(tx); err != nil {
				return err
			}
		}
		return nil
	}

	return db.withWriteTransaction(func() error {
		// Invoke model hook: CreateBefore for the entire batch.
		if !db.noHook {
			if err = traceModelHook[M](db.ctx, consts.PHASE_CREATE_BEFORE, span, func(spanCtx context.Context) error {
				for i := range objs {
					if err = objs[i].CreateBefore(spanCtx); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				return err
			}
		}
		for i := range objs {
			objs[i].SetID() // set id when id is empty.
		}

		tableName := db.m.GetTableName()
		batchSize := defaultBatchSize
		if db.batchSize > 0 {
			batchSize = db.batchSize
		}
		// Force created_at/updated_at to now; see the timestamp note in the doc
		// comment. dbruntime.NowUTC owns the time base (UTC, millisecond
		// precision) so the values handed back equal what a later read returns.
		now := dbruntime.NowUTC()
		for i := range objs {
			objs[i].SetCreatedAt(now)
			objs[i].SetUpdatedAt(now)
		}
		for i := 0; i < len(objs); i += batchSize {
			end := min(i+batchSize, len(objs))
			if err = db.ins.Session(&gorm.Session{}).Table(tableName).Create(objs[i:end]).Error; err != nil {
				return err
			}
		}
		// Invoke model hook: CreateAfter for the entire batch.
		if !db.noHook {
			if err = traceModelHook[M](db.ctx, consts.PHASE_CREATE_AFTER, span, func(spanCtx context.Context) error {
				for i := range objs {
					if err = objs[i].CreateAfter(spanCtx); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// Delete removes one or multiple records from the database.
// By default performs soft delete (sets deleted_at timestamp).
// Use WithPurge() for permanent deletion (hard delete).
// Executes DeleteBefore and DeleteAfter model hooks unless disabled with WithoutHook or WithDryRun.
//
// Parameters:
//   - objs: One or more model instances to delete. Empty objects are automatically filtered out.
//
// Behavior:
//   - Soft delete (default): Sets deleted_at field, records remain in database but are hidden from normal queries
//   - Hard delete (with WithPurge): Permanently removes records from database
//   - Soft-deleted records are automatically excluded from List, Get, First, Last, Count, and other query operations
//   - Supports batch processing for performance
//   - Returns nil if no valid objects provided (empty slice or all objects are empty)
//   - WithDryRun builds SQL only and does not execute hooks, database I/O, or object field filling
//
// On a ClickHouse instance the contract is weaker (see clickhouseDelete):
// a lightweight DELETE by primary key with no hooks and no transaction, and
// always physical — ClickHouse has no application-level soft delete, so the
// model's Purge and WithPurge are ignored there.
//
// Example:
//
//	Delete(&user)  // Soft delete by primary key
//	Delete(user1, user2, user3)  // Batch soft delete multiple records
//	WithQuery(params).Delete(&User{})  // Delete with conditions
//	WithPurge().Delete(&user)  // Permanent deletion
func (db *database[M]) Delete(objs ...M) (err error) {
	defer db.reset()

	objs = compactModels(objs)
	if len(objs) == 0 {
		return nil
	}

	if err = db.prepare(); err != nil {
		return err
	}
	if db.dialect() == dialectClickHouse {
		return db.clickhouseDelete(objs)
	}
	done, span := db.trace("Delete", len(objs))
	defer func() { done(err) }()

	if db.dryRun {
		tableName := db.m.GetTableName()
		batchSize := defaultDeleteBatchSize
		if db.batchSize > 0 {
			batchSize = db.batchSize
		}
		dryRunObjs := cloneDryRunModels(objs)
		for i := 0; i < len(dryRunObjs); i += batchSize {
			end := min(i+batchSize, len(dryRunObjs))
			if util.Deref(db.enablePurge) {
				tx := db.ins.Session(&gorm.Session{DryRun: true}).Table(tableName).Unscoped().Delete(dryRunObjs[i:end])
				if err = db.collectSQL(tx); err != nil {
					return err
				}
				continue
			}
			tx := db.ins.Session(&gorm.Session{DryRun: true}).Table(tableName).Delete(dryRunObjs[i:end])
			if err = db.collectSQL(tx); err != nil {
				return err
			}
		}
		return nil
	}

	return db.withWriteTransaction(func() error {
		// Invoke model hook: DeleteBefore.
		if !db.noHook {
			if err = traceModelHook[M](db.ctx, consts.PHASE_DELETE_BEFORE, span, func(spanCtx context.Context) error {
				for i := range objs {
					if err = objs[i].DeleteBefore(spanCtx); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				return err
			}
		}
		tableName := db.m.GetTableName()
		if util.Deref(db.enablePurge) {
			// delete permanently.
			batchSize := defaultDeleteBatchSize
			if db.batchSize > 0 {
				batchSize = db.batchSize
			}
			for i := 0; i < len(objs); i += batchSize {
				end := min(i+batchSize, len(objs))
				if err = db.ins.Session(&gorm.Session{}).Table(tableName).Unscoped().Delete(objs[i:end]).Error; err != nil {
					return err
				}
			}
		} else {
			// Soft delete: only set "deleted_at" to the current time. The row keeps
			// occupying its unique keys, so a later Create with the same unique key
			// fails with ErrDuplicatedKey; only Upsert can update such a row again.
			batchSize := defaultDeleteBatchSize
			if db.batchSize > 0 {
				batchSize = db.batchSize
			}
			for i := 0; i < len(objs); i += batchSize {
				end := min(i+batchSize, len(objs))
				if err = db.ins.Session(&gorm.Session{}).Table(tableName).Delete(objs[i:end]).Error; err != nil {
					return err
				}
			}
		}
		// Invoke model hook: DeleteAfter.
		if !db.noHook {
			if err = traceModelHook[M](db.ctx, consts.PHASE_DELETE_AFTER, span, func(spanCtx context.Context) error {
				for i := range objs {
					if err = objs[i].DeleteAfter(spanCtx); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// Update saves the full state of one or multiple existing records.
// It is a pure UPDATE by primary key: it never inserts, and every record must
// already exist. Use Upsert for deliberate insert-or-update semantics.
// Executes UpdateBefore and UpdateAfter model hooks unless disabled with WithoutHook or WithDryRun.
//
// Parameters:
//   - objs: One or more model instances to update. Empty objects are automatically filtered out.
//
// Behavior:
//   - Every object must carry a non-empty ID, otherwise ErrIDRequired is
//     returned before any database work.
//   - Writes the full row including zero values, or only the columns chosen
//     with WithSelect.
//   - Timestamp and audit columns are framework-managed and cannot be forged
//     by callers: created_at/created_by are never written (creation facts),
//     deleted_at is never written (rows cannot be soft-deleted or resurrected
//     through Update), and updated_at is always refreshed to the current UTC
//     time by GORM (dbruntime.NowUTC) regardless of the value carried by objs.
//   - A record matching no live row (missing or soft deleted) fails with
//     ErrRecordNotFound. Detection relies on matched-rows semantics: the
//     framework MySQL DSN enables clientFoundRows=true so an update that
//     changes nothing still counts as matched instead of being misread as
//     missing. Custom database connections must keep that flag.
//   - Runs hooks and all row updates in one transaction: any missing record or
//     failed hook rolls back the whole call (all-or-nothing), joining the
//     transaction carried by ctx when present.
//   - Returns nil if no valid objects provided (empty slice or all objects are empty)
//
// Returns ErrIDRequired when an object has no ID, ErrRecordNotFound when a
// record does not exist (or is soft deleted), and ErrDuplicatedKey when the new
// values collide with a unique key owned by another row.
// WithDryRun builds SQL only and does not execute hooks, database I/O, or object field filling.
//
// On a ClickHouse instance the contract is weaker (see clickhouseUpdate):
// each record becomes an asynchronous ALTER TABLE ... UPDATE mutation — heavy,
// meant for low-frequency data correction — with no hooks, no transaction,
// and no existence detection: a nil error means accepted, not rewritten, and
// a missing record passes silently instead of ErrRecordNotFound. ClickHouse
// refuses to UPDATE an ORDER BY key column, so narrow the write with
// WithSelect to the columns being corrected.
//
// Example:
//
//	user.Name = "Updated Name"
//	Update(&user)  // Update single record
//	Update(user1, user2, user3)  // Batch update multiple records
func (db *database[M]) Update(objs ...M) (err error) {
	defer db.reset()

	objs = compactModels(objs)
	if len(objs) == 0 {
		return nil
	}
	// A pure UPDATE needs a primary key on every record; fail fast before any
	// database work so a partially valid batch never starts writing.
	for i := range objs {
		if len(objs[i].GetID()) == 0 {
			return ErrIDRequired
		}
	}

	if err = db.prepare(); err != nil {
		return err
	}
	if db.dialect() == dialectClickHouse {
		return db.clickhouseUpdate(objs)
	}
	done, span := db.trace("Update", len(objs))
	defer func() { done(err) }()

	tableName := db.m.GetTableName()

	if db.dryRun {
		dryRunObjs := cloneDryRunModels(objs)
		for i := range dryRunObjs {
			tx := db.updateRowStatement(db.ins.Session(&gorm.Session{DryRun: true}), tableName, dryRunObjs[i]).Updates(dryRunObjs[i])
			if err = db.collectSQL(tx); err != nil {
				return err
			}
		}
		return nil
	}

	return db.withWriteTransaction(func() error {
		// Invoke model hook: UpdateBefore.
		if !db.noHook {
			if err = traceModelHook[M](db.ctx, consts.PHASE_UPDATE_BEFORE, span, func(spanCtx context.Context) error {
				for i := range objs {
					if err = objs[i].UpdateBefore(spanCtx); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				return err
			}
		}
		for i := range objs {
			res := db.updateRowStatement(db.ins.Session(&gorm.Session{}), tableName, objs[i]).Updates(objs[i])
			if res.Error != nil {
				return res.Error
			}
			// Zero matched rows means no live row has this id; matched-rows
			// semantics make this reliable even when nothing changed (see the
			// doc comment).
			if res.RowsAffected == 0 {
				return errors.Wrapf(ErrRecordNotFound, "update %s id=%s", tableName, objs[i].GetID())
			}
		}
		// Invoke model hook: UpdateAfter.
		if !db.noHook {
			if err = traceModelHook[M](db.ctx, consts.PHASE_UPDATE_AFTER, span, func(spanCtx context.Context) error {
				for i := range objs {
					if err = objs[i].UpdateAfter(spanCtx); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// updateRowStatement builds the single-row UPDATE statement Update issues per
// record: full-row semantics by default (Select("*") writes zero values too),
// narrowed by WithSelect when provided, with framework-managed audit columns
// excluded. created_at/created_by belong to creation and deleted_at belongs to
// Delete; omitting them means callers cannot forge creation audit data,
// soft-delete a row, or resurrect one through Update. updated_at stays
// writable because GORM's auto-update-time handling always overwrites it with
// the current time, even under a narrowed WithSelect.
func (db *database[M]) updateRowStatement(session *gorm.DB, tableName string, obj M) *gorm.DB {
	tx := session.Table(tableName).Model(obj)
	if len(db.selectColumns) > 0 {
		tx = tx.Select(db.selectColumns)
	} else {
		tx = tx.Select("*")
	}
	return tx.Omit("created_at", "created_by", "deleted_at")
}

// UpdateByID updates a specific field of a single record identified by ID.
// This is a lightweight update operation that bypasses model hooks for performance.
// Only updates the specified field without triggering validation or business logic.
//
// Parameters:
//   - id: The primary key of the record to update. Must not be empty.
//   - column: The database column to update. Must not be empty.
//   - value: The new value for the column. Must not be nil.
//
// Behavior:
//   - Automatically updates the updated_at timestamp
//   - Does not invoke UpdateBefore/UpdateAfter hooks for performance reasons
//   - Returns ErrIDRequired if id is empty
//   - Returns ErrEmptyFieldName if column is empty
//   - Returns ErrNilValue if value is nil
//   - Returns nil (no error) if the record with the given ID does not exist
//   - On a ClickHouse instance the statement is an asynchronous ALTER TABLE
//     ... UPDATE mutation: a nil error means accepted, not rewritten
//
// Example:
//
//	UpdateByID("user123", "status", "active")  // Update user status
//	UpdateByID("record456", "score", 99.99)    // Update record score
func (db *database[M]) UpdateByID(id string, column string, value any) (err error) {
	defer db.reset()

	if len(id) == 0 {
		return ErrIDRequired
	}
	if len(column) == 0 {
		return ErrEmptyFieldName
	}
	if value == nil {
		return ErrNilValue
	}

	if err = db.prepare(); err != nil {
		return err
	}
	// The probe clones the shared model metadata, which must keep a zero ID;
	// see normalizeModelID for the coercion hazard it guards against.
	var ok bool
	if id, ok = normalizeModelID(db.m, id); !ok {
		return ErrRecordNotFound
	}
	done, _ := db.trace("UpdateByID")
	defer func() { done(err) }()

	tableName := db.m.GetTableName()

	if db.dryRun {
		tx := db.ins.Session(&gorm.Session{DryRun: true}).Table(tableName).Model(*new(M)).Where("id = ?", id).Update(column, value)
		return db.collectSQL(tx)
	}

	if err = db.ins.Session(&gorm.Session{}).Table(tableName).Model(*new(M)).Where("id = ?", id).Update(column, value).Error; err != nil {
		return err
	}
	return nil
}

// cloneDryRunModels returns shallow copies so GORM dry-run callbacks can build SQL without
// mutating caller-owned model fields such as ID, timestamps, or soft-delete markers.
func cloneDryRunModels[M types.Model](objs []M) []M {
	cloned := make([]M, 0, len(objs))
	for _, obj := range objs {
		cloned = append(cloned, cloneDryRunModel(obj))
	}
	return cloned
}

func cloneDryRunModel[M types.Model](obj M) M {
	value := reflect.ValueOf(obj)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() {
		return obj
	}
	elem := value.Elem()
	if !elem.IsValid() || elem.Kind() != reflect.Struct {
		return obj
	}
	cloned := reflect.New(elem.Type())
	cloned.Elem().Set(elem)
	model, ok := cloned.Interface().(M)
	if !ok {
		return obj
	}
	return model
}
