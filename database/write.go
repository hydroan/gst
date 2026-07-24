package database

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"gorm.io/gorm"
	gormschema "gorm.io/gorm/schema"
)

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
// Example:
//
//	Create(&User{Name: "John", Email: "john@example.com"})  // Create single record
//	Create(user1, user2, user3)  // Batch create multiple records
func (db *database[M]) Create(_objs ...M) (err error) {
	defer db.reset()

	if len(_objs) == 0 {
		return nil
	}
	var empty M
	objs := make([]M, 0, len(_objs))
	for _, obj := range _objs {
		if reflect.DeepEqual(obj, empty) {
			continue
		}
		objs = append(objs, obj)
	}
	if len(objs) == 0 {
		return nil
	}

	if err = db.prepare(); err != nil {
		return err
	}
	done, _, span := db.trace("Create", len(objs))
	defer done(err)

	if db.dryRun {
		tableName := db.m.GetTableName()
		if len(db.tableName) > 0 {
			tableName = db.tableName
		}
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
		if len(db.tableName) > 0 {
			tableName = db.tableName
		}
		batchSize := defaultBatchSize
		if db.batchSize > 0 {
			batchSize = db.batchSize
		}
		// Force created_at/updated_at to now; see the timestamp note in the doc comment.
		now := time.Now()
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
// Example:
//
//	Delete(&user)  // Soft delete by primary key
//	Delete(user1, user2, user3)  // Batch soft delete multiple records
//	WithQuery(params).Delete(&User{})  // Delete with conditions
//	WithPurge().Delete(&user)  // Permanent deletion
func (db *database[M]) Delete(_objs ...M) (err error) {
	defer db.reset()

	if len(_objs) == 0 {
		return nil
	}
	var empty M
	objs := make([]M, 0, len(_objs))
	for _, obj := range _objs {
		if reflect.DeepEqual(obj, empty) {
			continue
		}
		objs = append(objs, obj)
	}
	if len(objs) == 0 {
		return nil
	}

	if err = db.prepare(); err != nil {
		return err
	}
	done, _, span := db.trace("Delete", len(objs))
	defer done(err)

	if db.dryRun {
		tableName := db.m.GetTableName()
		if len(db.tableName) > 0 {
			tableName = db.tableName
		}
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
		if len(db.tableName) > 0 {
			tableName = db.tableName
		}
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
//     through Update), and updated_at is always refreshed to the current time
//     by GORM regardless of the value carried by objs.
//   - A record matching no live row (missing or soft deleted) fails with
//     ErrRecordNotFound. Detection relies on matched-rows semantics: the
//     framework MySQL DSN enables clientFoundRows=true so an update that
//     changes nothing still counts as matched instead of being misread as
//     missing. Custom connections passed via WithDB must keep that flag.
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
// Example:
//
//	user.Name = "Updated Name"
//	Update(&user)  // Update single record
//	Update(user1, user2, user3)  // Batch update multiple records
func (db *database[M]) Update(_objs ...M) (err error) {
	defer db.reset()

	if len(_objs) == 0 {
		return nil
	}
	var empty M
	objs := make([]M, 0, len(_objs))
	for _, obj := range _objs {
		if reflect.DeepEqual(obj, empty) {
			continue
		}
		objs = append(objs, obj)
	}
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
	done, _, span := db.trace("Update", len(objs))
	defer done(err)

	tableName := db.m.GetTableName()
	if len(db.tableName) > 0 {
		tableName = db.tableName
	}

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

// Upsert saves one or multiple records with insert-or-update semantics. It is
// the only write that merges: Create rejects duplicates and Update rejects
// missing rows, so reach for Upsert only when a flow deliberately wants
// "insert the row, or overwrite whichever row owns the colliding unique key"
// (imports, sync jobs, seed-style maintenance).
//
// It relies on the database's conflict resolution (MySQL
// "INSERT ... ON DUPLICATE KEY UPDATE", SQLite/Postgres "ON CONFLICT DO
// UPDATE"), which has sharp edges the caller owns:
//   - The conflict target cannot be chosen: a collision on ANY unique key, not
//     only the primary key, turns the insert into an update of the conflicting
//     row. On tables with several unique keys, which row gets updated follows
//     database index-selection rules.
//   - A collision with a soft-deleted row updates that row and clears its
//     deleted_at, resurrecting it.
//   - created_at is preserved on conflict updates (auto-create-time columns
//     are excluded from the conflict update set); on inserted rows
//     created_at/updated_at are forced to the current time exactly like
//     Create, so caller-supplied timestamps are never honored.
//   - After each batch, caller-owned objects are re-synced from the database
//     by complete unique-index values: an object that collided exposes the
//     persisted row's ID instead of the one generated for the insert attempt.
//
// Upsert cannot know whether a row was inserted or updated, so it runs NO
// model hooks — create/update hooks would lie for one of the two paths — and
// must not be used to smuggle business writes past hook logic.
//
// All batches run in one transaction (all-or-nothing), joining the transaction
// carried by ctx when present. WithSelect narrows the written columns. With
// clientFoundRows enabled on MySQL, the reported affected count is 1 per row
// whether it was inserted, updated, or left unchanged.
// WithDryRun builds SQL only and does not execute database I/O or object
// field filling.
func (db *database[M]) Upsert(_objs ...M) (err error) {
	defer db.reset()

	if len(_objs) == 0 {
		return nil
	}
	var empty M
	objs := make([]M, 0, len(_objs))
	for _, obj := range _objs {
		if reflect.DeepEqual(obj, empty) {
			continue
		}
		objs = append(objs, obj)
	}
	if len(objs) == 0 {
		return nil
	}

	if err = db.prepare(); err != nil {
		return err
	}
	done, _, _ := db.trace("Upsert", len(objs))
	defer done(err)

	tableName := db.m.GetTableName()
	if len(db.tableName) > 0 {
		tableName = db.tableName
	}
	batchSize := defaultBatchSize
	if db.batchSize > 0 {
		batchSize = db.batchSize
	}

	if db.dryRun {
		if len(db.selectColumns) > 0 {
			db.ins = db.ins.Select(db.selectColumns)
		}
		dryRunObjs := cloneDryRunModels(objs)
		for i := 0; i < len(dryRunObjs); i += batchSize {
			end := min(i+batchSize, len(dryRunObjs))
			tx := db.ins.Session(&gorm.Session{DryRun: true}).Table(tableName).Save(dryRunObjs[i:end])
			if err = db.collectSQL(tx); err != nil {
				return err
			}
		}
		return nil
	}

	return db.withWriteTransaction(func() error {
		for i := range objs {
			objs[i].SetID() // set id when id is empty.
		}
		// Force created_at/updated_at like Create: the values only land on
		// inserted rows, conflict updates keep the existing created_at.
		now := time.Now()
		for i := range objs {
			objs[i].SetCreatedAt(now)
			objs[i].SetUpdatedAt(now)
		}
		if len(db.selectColumns) > 0 {
			db.ins = db.ins.Select(db.selectColumns)
		}
		for i := 0; i < len(objs); i += batchSize {
			end := min(i+batchSize, len(objs))
			if err = db.ins.Session(&gorm.Session{}).Table(tableName).Save(objs[i:end]).Error; err != nil {
				return err
			}
			if err = db.syncSaveResultsByUniqueIndexes(tableName, objs[i:end]); err != nil {
				return err
			}
		}
		return nil
	})
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
//
// Example:
//
//	UpdateByID("user123", "status", "active")  // Update user status
//	UpdateByID("order456", "amount", 99.99)    // Update order amount
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
	done, _, _ := db.trace("UpdateByID")
	defer done(err)

	// return db.db.Model(*new(M)).Where("id = ?", id).Update(column, value).Error
	tableName := db.m.GetTableName()
	if len(db.tableName) > 0 {
		tableName = db.tableName
	}

	if db.dryRun {
		tx := db.ins.Session(&gorm.Session{DryRun: true}).Table(tableName).Model(*new(M)).Where("id = ?", id).Update(column, value)
		return db.collectSQL(tx)
	}

	if err = db.ins.Session(&gorm.Session{}).Table(tableName).Model(*new(M)).Where("id = ?", id).Update(column, value).Error; err != nil {
		return err
	}
	return nil
}

// syncSaveResultsByUniqueIndexes refreshes caller-owned objects after GORM
// turns Save(slice) into an upsert.
//
// Context: database.Upsert persists batches through GORM Save(slice). For
// slice values, GORM builds an INSERT ... ON DUPLICATE KEY UPDATE /
// ON CONFLICT UPDATE statement. If the conflict is on a non-primary unique
// index, the database updates the already-existing row, but GORM leaves the Go
// object with the ID supplied by the caller — usually one freshly generated
// for the insert attempt. Callers reuse that same object for operation
// logs and HTTP responses, so the object must be
// reconciled before any post-save behavior observes it. Create and Update do
// not need this: a pure INSERT keeps the caller IDs and a pure UPDATE never
// moves to another row.
//
// The reconciliation is intentionally narrow:
//   - models without non-primary unique indexes pay no extra query cost;
//   - only complete unique-index values are used for lookup;
//   - only GORM-persistent fields are copied back, preserving gorm:"-" values
//     that hooks or controllers may have placed on the object.
func (db *database[M]) syncSaveResultsByUniqueIndexes(tableName string, objs []M) error {
	if len(objs) == 0 {
		return nil
	}

	stmt := &gorm.Statement{DB: db.ins}
	if err := stmt.Parse(db.m); err != nil {
		return err
	}
	indexes := saveResultSyncUniqueIndexes(stmt.Schema)
	if len(indexes) == 0 {
		return nil
	}

	syncedIDs := make(map[string]struct{}, len(objs))
	for _, index := range indexes {
		candidatesByKey := make(map[string][]M)
		query := db.ins.Session(&gorm.Session{})
		if len(tableName) > 0 {
			query = query.Table(tableName)
		}
		query = query.Limit(-1)

		var hasCondition bool
		for _, obj := range objs {
			if _, synced := syncedIDs[obj.GetID()]; synced {
				continue
			}
			values, ok := saveResultSyncUniqueValues(db.ctx, index, obj)
			if !ok {
				continue
			}

			condition, args := db.saveResultSyncUniqueCondition(tableName, index, values)
			if !hasCondition {
				query = query.Where(condition, args...)
				hasCondition = true
			} else {
				query = query.Or(condition, args...)
			}
			key := saveResultSyncUniqueKey(values)
			candidatesByKey[key] = append(candidatesByKey[key], obj)
		}
		if !hasCondition {
			continue
		}

		persisted := make([]M, 0, len(candidatesByKey))
		if err := query.Find(&persisted).Error; err != nil {
			return err
		}
		for _, current := range persisted {
			values, ok := saveResultSyncUniqueValues(db.ctx, index, current)
			if !ok {
				continue
			}
			for _, candidate := range candidatesByKey[saveResultSyncUniqueKey(values)] {
				originalID := candidate.GetID()
				if err := copySaveResultPersistentFields(db.ctx, stmt.Schema, candidate, current); err != nil {
					return err
				}
				syncedIDs[originalID] = struct{}{}
				syncedIDs[candidate.GetID()] = struct{}{}
			}
		}
	}

	return nil
}

func saveResultSyncUniqueIndexes(schema *gormschema.Schema) []*gormschema.Index {
	indexes := make([]*gormschema.Index, 0)
	for _, index := range schema.ParseIndexes() {
		if !saveResultSyncUniqueIndexUsable(index) {
			continue
		}
		indexes = append(indexes, index)
	}

	for _, field := range schema.Fields {
		if !field.Unique || field.UniqueIndex != "" || field.PrimaryKey || field.DBName == "" {
			continue
		}
		indexes = append(indexes, &gormschema.Index{
			Name:  "unique:" + field.DBName,
			Class: "UNIQUE",
			Fields: []gormschema.IndexOption{
				{Field: field},
			},
		})
	}
	return indexes
}

func saveResultSyncUniqueIndexUsable(index *gormschema.Index) bool {
	if index == nil || index.Class != "UNIQUE" || index.Where != "" || len(index.Fields) == 0 {
		return false
	}

	var hasNonPrimaryField bool
	for _, field := range index.Fields {
		if field.Field == nil || field.Field.DBName == "" || field.Expression != "" {
			return false
		}
		if !field.Field.PrimaryKey {
			hasNonPrimaryField = true
		}
	}
	return hasNonPrimaryField
}

func saveResultSyncUniqueValues(ctx context.Context, index *gormschema.Index, obj any) ([]any, bool) {
	modelValue := reflect.ValueOf(obj)
	values := make([]any, 0, len(index.Fields))
	for _, field := range index.Fields {
		value, _ := field.Field.ValueOf(ctx, modelValue)
		if saveResultSyncValueIsNil(value) {
			return nil, false
		}
		values = append(values, value)
	}
	return values, true
}

func saveResultSyncValueIsNil(value any) bool {
	if value == nil {
		return true
	}
	val := reflect.ValueOf(value)
	switch val.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return val.IsNil()
	default:
		return false
	}
}

func (db *database[M]) saveResultSyncUniqueCondition(tableName string, index *gormschema.Index, values []any) (string, []any) {
	parts := make([]string, 0, len(index.Fields))
	args := make([]any, 0, len(index.Fields))
	for i, field := range index.Fields {
		parts = append(parts, db.quoteTableColumn(tableName, field.Field.DBName)+" = ?")
		args = append(args, values[i])
	}
	return strings.Join(parts, " AND "), args
}

func saveResultSyncUniqueKey(values []any) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%T:%#v", value, value))
	}
	return strings.Join(parts, "\x00")
}

func copySaveResultPersistentFields(ctx context.Context, schema *gormschema.Schema, dst any, src any) error {
	dstValue := reflect.ValueOf(dst)
	srcValue := reflect.ValueOf(src)
	for _, field := range schema.Fields {
		if field.DBName == "" {
			continue
		}
		value, _ := field.ValueOf(ctx, srcValue)
		if err := field.Set(ctx, dstValue, value); err != nil {
			return err
		}
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
