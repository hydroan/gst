package database

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/modelregistry"
	"gorm.io/gorm"
	gormschema "gorm.io/gorm/schema"
)

// Upsert saves one or multiple records with insert-or-update semantics. It is
// the only write that merges: Create rejects duplicates and Update rejects
// missing rows, so reach for Upsert only when a flow deliberately wants
// "insert the row, or overwrite whichever row owns the colliding key"
// (imports, sync jobs, seed-style maintenance).
//
// It relies on the database's conflict resolution, and WHICH collisions merge
// is a per-dialect contract, because the dialects disagree about the conflict
// target and no portable spelling exists (the same split every major ORM
// documents rather than papers over):
//   - MySQL ("INSERT ... ON DUPLICATE KEY UPDATE"): a collision on ANY unique
//     key, not only the primary key, turns the insert into an update of the
//     conflicting row. The target cannot be narrowed; on tables with several
//     unique keys, which row gets updated follows index-selection rules.
//   - PostgreSQL and SQLite ("ON CONFLICT (primary key) DO UPDATE"): only a
//     primary-key collision merges. A collision on any other unique key fails
//     the call with ErrDuplicatedKey — simulating the MySQL behavior with a
//     read-then-write would carry a race no transaction level closes, so the
//     error is surfaced instead. A flow that must merge on a non-primary key
//     there reads the existing row first and writes through Update.
//
// The remaining sharp edges the caller owns on every dialect:
//   - A merging collision with a soft-deleted row updates that row and clears
//     its deleted_at, resurrecting it.
//   - created_at is preserved on conflict updates (auto-create-time columns
//     are excluded from the conflict update set); on inserted rows
//     created_at/updated_at are forced to the current time exactly like
//     Create, so caller-supplied timestamps are never honored.
//   - After each batch, caller-owned objects are re-synced from the database
//     by complete unique-index values: an object that collided exposes the
//     persisted row's ID instead of the one generated for the insert attempt.
//     The sync exists for MySQL's any-unique-key merges; primary-key merges
//     keep the caller's ID and leave it nothing to reconcile.
//
// Upsert cannot know whether a row was inserted or updated, so it runs NO
// model hooks — create/update hooks would lie for one of the two paths — and
// must not be used to smuggle business writes past hook logic.
//
// On a ClickHouse instance Upsert answers ErrUnsupportedOnDialect: there are
// no conflict semantics to build on.
//
// All batches run in one transaction (all-or-nothing), joining the transaction
// carried by ctx when present. WithSelect narrows the written columns. With
// clientFoundRows enabled on MySQL, the reported affected count is 1 per row
// whether it was inserted, updated, or left unchanged.
// WithDryRun builds SQL only and does not execute database I/O or object
// field filling.
func (db *database[M]) Upsert(objs ...M) (err error) {
	defer db.reset()

	if db.includeDeleted {
		return errors.Wrap(ErrWithDeletedOnWrite, "Upsert")
	}
	objs = compactModels(objs)
	if len(objs) == 0 {
		return nil
	}

	if err = db.prepare(); err != nil {
		return err
	}
	// ClickHouse carries no conflict semantics for Upsert to build on: it has
	// no unique constraints, and ReplacingMergeTree deduplication is an engine
	// property, not a statement contract. Fails per the capability-miss rule.
	if db.dialect() == dialectClickHouse {
		return errors.Wrap(ErrUnsupportedOnDialect, "Upsert on clickhouse")
	}
	done, _ := db.trace(phaseUpsert, len(objs))
	defer func() { done(err) }()

	tableName := db.m.TableName()
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
			tx := dryRunSession(db.ins).Save(dryRunObjs[i:end])
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
		// Force created_at/updated_at like Create (dbruntime.NowUTC, the same
		// one-time-base reason): the values only land on inserted rows,
		// conflict updates keep the existing created_at.
		now := dbruntime.NowUTC()
		for i := range objs {
			objs[i].SetCreatedAt(now)
			objs[i].SetUpdatedAt(now)
		}
		if len(db.selectColumns) > 0 {
			db.ins = db.ins.Select(db.selectColumns)
		}
		for i := 0; i < len(objs); i += batchSize {
			end := min(i+batchSize, len(objs))
			if err = db.ins.Session(&gorm.Session{}).Save(objs[i:end]).Error; err != nil {
				return err
			}
			if err = db.syncSaveResultsByUniqueIndexes(tableName, objs[i:end]); err != nil {
				return err
			}
		}
		return nil
	})
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
//   - models without non-primary unique indexes pay no extra query cost, and
//     the indexes themselves are resolved once per model type;
//   - only complete unique-index values are used for lookup;
//   - only GORM-persistent fields are copied back, preserving gorm:"-" values
//     that hooks or controllers may have placed on the object.
func (db *database[M]) syncSaveResultsByUniqueIndexes(tableName string, objs []M) error {
	if len(objs) == 0 {
		return nil
	}

	indexes, err := db.saveResultSyncUniqueIndexes()
	if err != nil {
		return err
	}
	if len(indexes) == 0 {
		return nil
	}

	stmt := &gorm.Statement{DB: db.ins}
	if err := stmt.Parse(db.m); err != nil {
		return err
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

// saveResultSyncUniqueIndexCache memoizes the resolved unique indexes per
// model type. The key space is bounded: keys are the canonical reflect.Type
// singletons of the model structs compiled into the binary, the same shape as
// the per-type caches gorm and internal/modelschema keep, so the map never
// grows past the number of model types. Caching is sound because both index
// sources are static: struct tags are fixed at compile time and Indexes is a
// pure declaration invoked on zero-value instances.
var saveResultSyncUniqueIndexCache sync.Map

// saveResultSyncUniqueIndexes returns the unique indexes the save-result sync
// must look through for db's model, resolving them on the first call per
// model type and answering every later call from the cache. gorm re-parses
// tag indexes on every ParseIndexes call, so without the cache each Upsert
// batch would pay the full tag parse plus the index plan validation again.
// Resolution errors are returned without being cached.
func (db *database[M]) saveResultSyncUniqueIndexes() ([]*gormschema.Index, error) {
	typ := reflect.TypeOf(db.m)
	if cached, ok := saveResultSyncUniqueIndexCache.Load(typ); ok {
		return cached.([]*gormschema.Index), nil //nolint:errcheck
	}

	stmt := &gorm.Statement{DB: db.ins}
	if err := stmt.Parse(db.m); err != nil {
		return nil, err
	}
	plans, err := modelregistry.ParseIndexPlans(db.ins, db.m)
	if err != nil {
		return nil, err
	}
	indexes, err := collectSaveResultSyncUniqueIndexes(stmt.Schema, plans)
	if err != nil {
		return nil, err
	}
	saveResultSyncUniqueIndexCache.Store(typ, indexes)
	return indexes, nil
}

// collectSaveResultSyncUniqueIndexes gathers the model's unique indexes from
// every declaration source: composite tag indexes parsed by gorm, single
// column unique tags, and the plans resolved from the model's Indexes method,
// which never appear in the gorm schema. The sources merge without
// deduplication: plan declarations duplicating a parsed tag index are already
// rejected at resolution (checkTagIndexConflicts), and the sync marks
// reconciled objects, so a redundant index entry could never re-sync one.
func collectSaveResultSyncUniqueIndexes(schema *gormschema.Schema, plans []modelregistry.IndexPlan) ([]*gormschema.Index, error) {
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

	for _, plan := range plans {
		if !plan.Unique {
			continue
		}
		fields := make([]gormschema.IndexOption, 0, len(plan.Columns))
		for _, column := range plan.Columns {
			field := schema.FieldsByDBName[column]
			if field == nil {
				// Plans are resolved against the same parsed schema, so a
				// missing column is a broken invariant rather than a user
				// mistake; failing beats silently skipping the index and
				// shipping unreconciled ids.
				return nil, errors.Newf("unique index %q names column %q that the schema of %s does not carry", plan.Name, column, schema.Name)
			}
			fields = append(fields, gormschema.IndexOption{Field: field})
		}
		// The usability filter also drops declarations covering only primary
		// key columns: a primary-key merge keeps the caller's id and needs no
		// reconciliation.
		index := &gormschema.Index{Name: plan.Name, Class: "UNIQUE", Fields: fields}
		if !saveResultSyncUniqueIndexUsable(index) {
			continue
		}
		indexes = append(indexes, index)
	}
	return indexes, nil
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
