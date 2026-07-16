package database

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/hydroan/gst/cache"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
	"gorm.io/gorm"
	gormschema "gorm.io/gorm/schema"
)

// Create inserts one or multiple records into the database.
// Automatically sets ID (if empty), created_at, and updated_at timestamps.
// Executes CreateBefore and CreateAfter model hooks unless disabled with WithoutHook or WithDryRun.
// Supports batch processing for large datasets using configurable batch sizes.
//
// Parameters:
//   - objs: One or more model instances to create. Empty objects are automatically filtered out.
//
// Behavior:
//   - Automatically generates ID if empty using SetID()
//   - Sets created_at and updated_at timestamps to current time
//   - Supports batch processing for performance
//   - Clears related cache entries unless WithDryRun is enabled
//   - Returns nil if no valid objects provided (empty slice or all objects are empty)
//
// Returns error if validation fails, database constraints are violated, or hooks return errors.
// WithDryRun builds SQL only and does not execute hooks, database I/O, cache mutation, or object field filling.
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
	done, ctx, span := db.trace("Create", len(objs))
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
			tx := db.ins.Session(&gorm.Session{DryRun: true}).Table(tableName).Save(dryRunObjs[i:end])
			if err = db.collectSQL(tx); err != nil {
				return err
			}
		}
		return nil
	}

	if db.enableCache {
		defer cache.Cache[[]M]().WithContext(ctx).Clear()
	}

	return db.withModelHookTransaction(func() error {
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

		// if err = db.db.Save(objs).Error; err != nil {
		// if err = db.db.Table(db.tableName).Save(objs).Error; err != nil {
		// 	return err
		// }
		//
		tableName := db.m.GetTableName()
		if len(db.tableName) > 0 {
			tableName = db.tableName
		}
		batchSize := defaultBatchSize
		if db.batchSize > 0 {
			batchSize = db.batchSize
		}
		// update "created_at" and "updated_at"
		now := time.Now()
		for i := range objs {
			objs[i].SetCreatedAt(now)
			objs[i].SetUpdatedAt(now)
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
		if db.enableCache {
			for i := range objs {
				_ = cache.Cache[M]().WithContext(ctx).Delete(objs[i].GetID())
			}
		}

		// // because db.db.Delete method just update field "delete_at" to current time,
		// // not really delete it(soft delete).
		// // If record already exists, Update method update all fields but exclude "created_at" by
		// // mysql "ON DUPLICATE KEY UPDATE" mechanism. so we should update the "created_at" field manually.
		// for i := range objs {
		// 	// 有些 model 重写 SetID 为一个空函数, 则 GetID() 的值为空字符串. 更新 created_at 则会报错
		// 	// 例如 casbin_rule 表/结构体: 这张表的 ID 总是 integer 类型, 并且有 autoincrement 属性, 所以必须重写 SetID.
		// 	if len(objs[i].GetID()) == 0 {
		// 		continue
		// 	}
		//
		// 	// 这里要重新创建一个 gorm.DB 实例, 否则会出现这种语句, id 出现多次了.
		// 	// UPDATE `assets` SET `created_at`='2023-11-12 14:35:42.604',`updated_at`='2023-11-12 14:35:42.604' WHERE id = '010103NU000020' AND `assets`.`deleted_at` IS NULL AND id = '010103NU000021' AND id = '010103NU000022' LIMIT 1000
		// 	var _db *gorm.DB
		// 	if strings.ToLower(config.App.Logger.Level) == "debug" {
		// 		_db = DB.Debug()
		// 	} else {
		// 		_db = DB
		// 	}
		// 	createdAt := time.Now()
		// 	if err = _db.Table(tableName).Model(*new(M)).Where("id = ?", objs[i].GetID()).Update("created_at", createdAt).Error; err != nil {
		// 		return err
		// 	}
		// }

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
//   - Clears related cache entries unless WithDryRun is enabled
//   - Returns nil if no valid objects provided (empty slice or all objects are empty)
//   - WithDryRun builds SQL only and does not execute hooks, database I/O, cache mutation, or object field filling
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
	done, ctx, span := db.trace("Delete", len(objs))
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

	if db.enableCache {
		defer cache.Cache[[]M]().WithContext(ctx).Clear()
	}

	return db.withModelHookTransaction(func() error {
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
			// if err = db.db.Unscoped().Delete(objs).Error; err != nil {
			// if err = db.db.Table(db.tableName).Unscoped().Delete(objs).Error; err != nil {
			// 	return err
			// }
			//
			batchSize := defaultDeleteBatchSize
			if db.batchSize > 0 {
				batchSize = db.batchSize
			}
			for i := 0; i < len(objs); i += batchSize {
				end := min(i+batchSize, len(objs))
				if err = db.ins.Session(&gorm.Session{}).Table(tableName).Unscoped().Delete(objs[i:end]).Error; err != nil {
					return err
				}
				if db.enableCache {
					for j := i; j < end; j++ {
						_ = cache.Cache[M]().WithContext(ctx).Delete(objs[j].GetID())
					}
				}
			}
		} else {
			// Delete() method just update field "delete_at" to currrent time.
			// DO NOT FORGET update the "created_at" field when create/update if record already exists.
			// if err = db.db.Delete(objs).Error; err != nil {
			// if err = db.db.Table(db.tableName).Delete(objs).Error; err != nil {
			// 	return err
			// }
			//
			batchSize := defaultDeleteBatchSize
			if db.batchSize > 0 {
				batchSize = db.batchSize
			}
			for i := 0; i < len(objs); i += batchSize {
				end := min(i+batchSize, len(objs))
				if err = db.ins.Session(&gorm.Session{}).Table(tableName).Delete(objs[i:end]).Error; err != nil {
					return err
				}
				if db.enableCache {
					for j := i; j < end; j++ {
						_ = cache.Cache[M]().WithContext(ctx).Delete(objs[j].GetID())
					}
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

// Update modifies one or multiple records in the database.
// Automatically updates the updated_at timestamp for each record.
// Executes UpdateBefore and UpdateAfter model hooks unless disabled with WithoutHook or WithDryRun.
// Uses GORM's Save method which performs INSERT or UPDATE based on primary key existence.
//
// Parameters:
//   - objs: One or more model instances to update. Empty objects are automatically filtered out.
//
// Behavior:
//   - If ID is empty: Generates a new ID and creates a new record (INSERT)
//   - If ID is not empty: Updates the existing record (UPDATE)
//   - Automatically updates the updated_at timestamp
//   - Preserves created_at timestamp (not modified during update)
//   - Updates all fields of the model
//   - Supports batch processing for performance
//   - Clears related cache entries unless WithDryRun is enabled
//   - Returns nil if no valid objects provided (empty slice or all objects are empty)
//   - WithDryRun builds SQL only and does not execute hooks, database I/O, cache mutation, or object field filling
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

	if err = db.prepare(); err != nil {
		return err
	}
	done, ctx, span := db.trace("Update", len(objs))
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
		if len(db.selectColumns) > 0 {
			db.ins = db.ins.Select(db.selectColumns)
		}
		dryRunObjs := cloneDryRunModels(objs)
		for i := 0; i < len(dryRunObjs); i += batchSize {
			end := min(i+batchSize, len(dryRunObjs))
			tx := db.ins.Session(&gorm.Session{DryRun: true}).Table(tableName).Save(dryRunObjs[i:end])
			if err = db.collectSQL(tx); err != nil {
				zap.S().Error(err)
				return err
			}
		}
		return nil
	}

	if db.enableCache {
		defer cache.Cache[[]M]().WithContext(ctx).Clear()
	}

	return db.withModelHookTransaction(func() error {
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
			objs[i].SetID() // set id when id is empty
		}
		// if err = db.db.Save(objs).Error; err != nil {
		// if err = db.db.Table(db.tableName).Save(objs).Error; err != nil {
		// 	return err
		// }
		//
		tableName := db.m.GetTableName()
		if len(db.tableName) > 0 {
			tableName = db.tableName
		}
		batchSize := defaultBatchSize
		if db.batchSize > 0 {
			batchSize = db.batchSize
		}
		// set selected columns.
		if len(db.selectColumns) > 0 {
			db.ins = db.ins.Select(db.selectColumns)
		}
		for i := 0; i < len(objs); i += batchSize {
			end := min(i+batchSize, len(objs))
			if err = db.ins.Session(&gorm.Session{}).Table(tableName).Save(objs[i:end]).Error; err != nil {
				zap.S().Error(err)
				return err
			}
			if err = db.syncSaveResultsByUniqueIndexes(tableName, objs[i:end]); err != nil {
				return err
			}
			if db.enableCache {
				for j := i; j < end; j++ {
					_ = cache.Cache[M]().WithContext(ctx).Delete(objs[j].GetID())
				}
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
//   - Does not mutate cache when WithDryRun is enabled
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
	done, ctx, _ := db.trace("UpdateByID")
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

	if db.enableCache {
		defer cache.Cache[[]M]().WithContext(ctx).Clear()
	}

	if err = db.ins.Session(&gorm.Session{}).Table(tableName).Model(*new(M)).Where("id = ?", id).Update(column, value).Error; err != nil {
		return err
	}
	if db.enableCache {
		_ = cache.Cache[M]().WithContext(ctx).Delete(id)
	}
	return nil
}

// syncSaveResultsByUniqueIndexes refreshes caller-owned objects after GORM
// turns Save(slice) into an upsert.
//
// Context: database.Create and database.Update both persist batches through
// GORM Save(slice). For slice values, GORM builds an INSERT ... ON DUPLICATE
// KEY UPDATE / ON CONFLICT UPDATE statement. If the conflict is on a
// non-primary unique index, the database updates the already-existing row, but
// GORM leaves the Go object with the ID supplied by the caller. For Create that
// ID is usually freshly generated; for Update it may be a stale or incorrect
// ID. The controller and service layers later reuse that same object for
// CreateAfter/UpdateAfter hooks, cache invalidation, operation logs, and HTTP
// responses, so the object must be reconciled before any post-save behavior
// observes it.
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
