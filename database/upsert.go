package database

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/tenant"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
//   - The conflict update is gorm's own, the UpdateAll expansion of its
//     slice Save: a column gorm's write permissions keep off a create or an
//     update keeps the row's value (a read-only column, a create-only one
//     such as a tenant column), a defaulted column is left alone, and an
//     update-time column is refreshed in its own unit.
//   - created_at and created_by are preserved on conflict updates (both are
//     create-only columns, see model.Base); on inserted rows
//     created_at/updated_at are forced to the current time and
//     created_by/updated_by to the id of the caller the context carries,
//     when it carries one, exactly like Create: whatever the objects hold
//     then, and what they hold when no caller is carried; a conflict update
//     refreshes updated_at and updated_by the same way.
//   - On a model whose rows belong to a tenant, a key held by another
//     tenant's row fails the call with ErrDuplicatedKey instead of merging
//     into that row (see refuseForeignTenantRows): the conflict update is no
//     UPDATE statement, so the tenant predicate every update carries would
//     not reach it.
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
// A versioned model (model.Version) is exempt from the optimistic-lock check
// — merge-overwrite is the point of Upsert — but the version column stays
// honest on both paths: inserted rows start at version 1, and a conflict
// update bumps the ROW's own version instead of writing the object's, so an
// upsert can never move a row's version backwards and every carried version
// out there expires. See model.Version.
//
// On a ClickHouse instance Upsert answers ErrUnsupportedOnDialect: there are
// no conflict semantics to build on.
//
// All batches run in one transaction (all-or-nothing), joining the transaction
// carried by ctx when present. WithSelect narrows the written columns, in the
// insert and in the conflict update alike; the update-time columns stay
// written. With clientFoundRows enabled on MySQL, the reported affected count
// is 1 per row whether it was inserted, updated, or left unchanged.
// WithDryRun builds SQL only and does not execute database I/O or object
// field filling.
func (db *database[M]) Upsert(objs ...M) (err error) {
	defer db.reset()

	if db.includeDeleted {
		return errors.Wrap(ErrWithDeletedOnWrite, "Upsert")
	}
	if db.replicaRead != nil {
		return errors.Wrap(ErrWithReplicaOnWrite, "Upsert")
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

	// A dry run builds its statements from copies, so the objects the caller
	// passed stay as they are (see WithDryRun).
	if db.dryRun {
		objs = cloneDryRunModels(objs)
	}
	// The caller is who wrote the records: created_by and updated_by take the
	// id the context carries, when it carries one, whatever the objects hold;
	// the values land on inserted rows, and the conflict update keeps
	// created_by and takes updated_by. A narrowed upsert carries both along.
	if caller := db.caller(); caller != "" {
		for i := range objs {
			objs[i].SetCreatedBy(caller)
			objs[i].SetUpdatedBy(caller)
		}
		if len(db.selectColumns) > 0 {
			db.selectColumns = ensureSelected(ensureSelected(db.selectColumns, "created_by"), "updated_by")
		}
	}

	// The conflict update is gorm's own: the UpdateAll expansion its slice Save
	// adds. A narrowed upsert that touched a row must not leave its update
	// time claiming otherwise, so the columns gorm refreshes on every write
	// join the selection, and gorm refreshes them in the conflict update too.
	// On a versioned model the version column alone diverges, bumping the
	// row's own version rather than writing the object's; see
	// versionBumpingConflict and model.Version.
	versionColumn, versioned := modelschema.VersionColumn(db.m)
	if len(db.selectColumns) > 0 {
		stmt := &gorm.Statement{DB: db.ins}
		if err = stmt.Parse(db.m); err != nil {
			return err
		}
		for _, field := range stmt.Schema.Fields {
			if field.AutoUpdateTime > 0 {
				db.selectColumns = ensureSelected(db.selectColumns, field.DBName)
			}
		}
	}

	if db.dryRun {
		if len(db.selectColumns) > 0 {
			db.ins = db.ins.Select(db.selectColumns)
		}
		initializeVersions(objs)
		for i := 0; i < len(objs); i += batchSize {
			end := min(i+batchSize, len(objs))
			tx := dryRunSession(db.ins).Omit(clause.Associations)
			if versioned {
				var onConflict clause.OnConflict
				if onConflict, err = versionBumpingConflict(tx, objs[i:end], versionColumn); err != nil {
					return err
				}
				tx = tx.Clauses(onConflict)
			}
			if err = db.collectSQL(tx.Save(objs[i:end])); err != nil {
				return err
			}
		}
		return nil
	}

	return db.withWriteTransaction(func() error {
		for i := range objs {
			objs[i].SetID() // set id when id is empty.
		}
		clearAssociations(objs)
		// Force created_at/updated_at like Create (dbruntime.NowUTC, the same
		// one-time-base reason): the values only land on inserted rows,
		// conflict updates keep the existing created_at.
		now := dbruntime.NowUTC()
		for i := range objs {
			objs[i].SetCreatedAt(now)
			objs[i].SetUpdatedAt(now)
		}
		// Insert candidates start at version 1; a conflict never reads the
		// object's version (the pre-set clause bumps the row's own instead).
		initializeVersions(objs)
		if len(db.selectColumns) > 0 {
			db.ins = db.ins.Select(db.selectColumns)
		}
		// gorm stamps the update times of the statement from the session's
		// clock; pinning it to now keeps them equal to the values forced above,
		// in the objects and in a versioned model's conflict update alike.
		clock := func() time.Time { return now }
		for i := 0; i < len(objs); i += batchSize {
			end := min(i+batchSize, len(objs))
			// The keys are read batch by batch, so the read before a write is
			// bounded by the batch size the way the write is.
			if err = db.refuseForeignTenantRows(tableName, objs[i:end]); err != nil {
				return err
			}
			tx := db.ins.Session(&gorm.Session{NowFunc: clock}).Omit(clause.Associations)
			if versioned {
				var onConflict clause.OnConflict
				if onConflict, err = versionBumpingConflict(tx, objs[i:end], versionColumn); err != nil {
					return err
				}
				tx = tx.Clauses(onConflict)
			}
			if err = tx.Save(objs[i:end]).Error; err != nil {
				// First-hand exit of a stack-less GORM/driver error; see the
				// error-stack contract in doc.go.
				return errors.WithStack(err)
			}
			if err = db.syncSaveResultsByUniqueIndexes(tableName, objs[i:end]); err != nil {
				return err
			}
		}
		return nil
	})
}

// refuseForeignTenantRows refuses a batch of an Upsert whose rows would merge
// into another tenant's. The conflict update of an INSERT is no UPDATE
// statement, so the tenant predicate every update carries (see tenant.ID)
// never reaches it: a row keyed like another tenant's would be rewritten and
// the call answered with success, where an Update of the same key answers
// that the row is not there. The rows the batch could merge into are read
// first, across tenants: by primary key, and by every complete unique index
// value, since MySQL merges on any unique key. One held by another tenant
// fails the call with ErrDuplicatedKey, the answer a key already taken gets;
// a soft-deleted row counts, since a merge would resurrect it. A model whose
// rows belong to no tenant, and a caller acting across every tenant, pay no
// read.
//
// Each key is one IN over the values the batch holds for it (see keyIn), the
// primary key included. A primary key zero in every column is left out of its
// IN: the database numbers that row when it inserts it, so no row holds the
// key yet.
//
// The read locks nothing and answers for the rows that exist when it runs. A
// row another tenant puts under one of the keys between the read and the
// write — inserted, or inserted in place of one deleted meanwhile — is
// merged into. A locking read would close that window only for rows that
// exist, and on MySQL under the repeatable-read level it defaults to it would
// also lock the index gap a key with no row yet falls into, so that writers
// of distinct keys deadlock on it (error 1213); the window stays open
// instead. SQLite has one writer at a time, so it has no window.
func (db *database[M]) refuseForeignTenantRows(tableName string, objs []M) error {
	columns, err := modelschema.Columns(reflect.TypeOf(db.m))
	if err != nil {
		return err
	}
	column := tenantColumn(columns)
	if column == "" {
		return nil
	}
	acting, one := tenant.From(db.ctx)
	if !one {
		return nil
	}

	stmt := &gorm.Statement{DB: db.ins}
	if err = stmt.Parse(db.m); err != nil {
		return err
	}
	indexes, err := db.uniqueIndexes()
	if err != nil {
		return err
	}
	conditions := make([]clause.Expression, 0, 1+len(indexes))
	// The primary keys are read here rather than through gorm's
	// GetIdentityFieldValuesMap, which leaves out the same all-zero keys but
	// takes two keys for one when their values joined by "_" read the same,
	// ("a_b", "c") and ("a", "b_c"), and so leaves a key out of the read.
	primaryKeys := make([][]any, 0, len(objs))
	for _, obj := range objs {
		value := reflect.ValueOf(obj)
		key := make([]any, len(stmt.Schema.PrimaryFields))
		numbered := false
		for i, field := range stmt.Schema.PrimaryFields {
			var zero bool
			key[i], zero = field.ValueOf(db.ctx, value)
			numbered = numbered || !zero
		}
		if numbered {
			primaryKeys = append(primaryKeys, key)
		}
	}
	if len(primaryKeys) > 0 {
		conditions = append(conditions, keyIn(tableName, stmt.Schema.PrimaryFieldDBNames, primaryKeys))
	}
	for _, index := range indexes {
		indexKeys := make([][]any, 0, len(objs))
		for _, obj := range objs {
			if values, ok := uniqueIndexValues(db.ctx, index, obj); ok {
				indexKeys = append(indexKeys, values)
			}
		}
		if len(indexKeys) > 0 {
			conditions = append(conditions, keyIn(tableName, indexColumns(index), indexKeys))
		}
	}
	if len(conditions) == 0 {
		return nil
	}

	held := make([]string, 0)
	query := db.ins.Session(&gorm.Session{NewDB: true}).Table(tableName).Where(clause.Or(conditions...))
	if err := query.Pluck(column, &held).Error; err != nil {
		// First-hand exit of a stack-less GORM/driver error; see the
		// error-stack contract in doc.go.
		return errors.WithStack(err)
	}
	for _, owner := range held {
		if owner != acting {
			return errors.Wrapf(ErrDuplicatedKey, "upsert into %s: a key of the batch is held by a row of tenant %q", tableName, owner)
		}
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
//   - models without non-primary unique indexes pay no extra query cost, and
//     the indexes themselves are resolved once per model type;
//   - only complete unique-index values are used for lookup, each index read
//     with one IN over the distinct values of the batch (see keyIn);
//   - only GORM-persistent fields are copied back, preserving gorm:"-" values
//     that hooks or controllers may have placed on the object; they are read
//     whole, whatever columns WithSelect narrowed the write to, so the object
//     holds the row's values.
func (db *database[M]) syncSaveResultsByUniqueIndexes(tableName string, objs []M) error {
	if len(objs) == 0 {
		return nil
	}

	indexes, err := db.uniqueIndexes()
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
		keys := make([][]any, 0, len(objs))
		for _, obj := range objs {
			if _, synced := syncedIDs[obj.GetID()]; synced {
				continue
			}
			values, ok := uniqueIndexValues(db.ctx, index, obj)
			if !ok {
				continue
			}

			key := uniqueIndexKey(values)
			if _, seen := candidatesByKey[key]; !seen {
				keys = append(keys, values)
			}
			candidatesByKey[key] = append(candidatesByKey[key], obj)
		}
		if len(keys) == 0 {
			continue
		}

		// A session of its own, so the row is read whole: the chain's
		// selection names the columns WithSelect narrowed the write to.
		query := db.ins.Session(&gorm.Session{NewDB: true})
		if len(tableName) > 0 {
			query = query.Table(tableName)
		}
		persisted := make([]M, 0, len(keys))
		if err := query.Where(keyIn(tableName, indexColumns(index), keys)).Find(&persisted).Error; err != nil {
			return errors.WithStack(err)
		}
		for _, current := range persisted {
			values, ok := uniqueIndexValues(db.ctx, index, current)
			if !ok {
				continue
			}
			for _, candidate := range candidatesByKey[uniqueIndexKey(values)] {
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

// uniqueIndexCache memoizes the resolved unique indexes per model type. The
// key space is bounded: keys are the canonical reflect.Type singletons of the
// model structs compiled into the binary, the same shape as the per-type
// caches gorm and internal/modelschema keep, so the map never grows past the
// number of model types. Caching is sound because both index sources are
// static: struct tags are fixed at compile time and Indexes is a pure
// declaration invoked on zero-value instances.
var uniqueIndexCache sync.Map

// uniqueIndexes returns the unique indexes of db's model a key of an Upsert
// batch collides on besides the primary key (see uniqueIndexUsable): the read
// before a tenant model's write and the sync after it look through them. They
// are resolved on the first call per model type, and every later call is
// answered from the cache: gorm re-parses tag indexes on every ParseIndexes
// call, so without the cache each Upsert batch would pay the full tag parse
// plus the index plan validation again. Resolution errors are returned
// without being cached.
func (db *database[M]) uniqueIndexes() ([]*gormschema.Index, error) {
	typ := reflect.TypeOf(db.m)
	if cached, ok := uniqueIndexCache.Load(typ); ok {
		return cached.([]*gormschema.Index), nil //nolint:errcheck
	}

	stmt := &gorm.Statement{DB: db.ins}
	if err := stmt.Parse(db.m); err != nil {
		return nil, err
	}
	plans, err := modelschema.ParseIndexPlans(db.ins, db.m)
	if err != nil {
		return nil, err
	}
	indexes, err := collectUniqueIndexes(stmt.Schema, plans)
	if err != nil {
		return nil, err
	}
	uniqueIndexCache.Store(typ, indexes)
	return indexes, nil
}

// collectUniqueIndexes gathers the model's unique indexes from every
// declaration source: composite tag indexes parsed by gorm, single column
// unique tags, and the plans resolved from the model's Indexes method, which
// never appear in the gorm schema. The sources merge without deduplication:
// plan declarations duplicating a parsed tag index are already rejected at
// resolution (checkTagIndexConflicts), and the sync marks reconciled objects,
// so a redundant index entry could never re-sync one.
func collectUniqueIndexes(schema *gormschema.Schema, plans []modelschema.IndexPlan) ([]*gormschema.Index, error) {
	indexes := make([]*gormschema.Index, 0)
	for _, index := range schema.ParseIndexes() {
		if !uniqueIndexUsable(index) {
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
		if !uniqueIndexUsable(index) {
			continue
		}
		indexes = append(indexes, index)
	}
	return indexes, nil
}

// uniqueIndexUsable reports whether an index is a unique key rows can be
// looked up by: unique, unconditional, over plain columns, and holding a
// column besides the primary key, which every reader takes as a key of its
// own.
func uniqueIndexUsable(index *gormschema.Index) bool {
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

// uniqueIndexValues returns the values obj holds for the columns of index, in
// order, or false when one of them is nil: a unique index tells NULL apart
// from every value, so a key holding one collides with no row.
func uniqueIndexValues(ctx context.Context, index *gormschema.Index, obj any) ([]any, bool) {
	modelValue := reflect.ValueOf(obj)
	values := make([]any, 0, len(index.Fields))
	for _, field := range index.Fields {
		value, _ := field.Field.ValueOf(ctx, modelValue)
		if isNilValue(value) {
			return nil, false
		}
		values = append(values, value)
	}
	return values, true
}

func isNilValue(value any) bool {
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

// indexColumns lists the columns of an index in the order it declares them.
func indexColumns(index *gormschema.Index) []string {
	columns := make([]string, 0, len(index.Fields))
	for _, field := range index.Fields {
		columns = append(columns, field.Field.DBName)
	}
	return columns
}

// keyIn matches the rows of tableName holding one of keys, each key the
// values of columns in order. It is the lookup by a set of keys gorm runs for
// a preload, schema.ToQueryValues into clause.IN: column IN (...) on a key of
// one column, (a, b) IN ((...), ...) on a key of several. A key adds a
// parameter per column and no nesting, where a condition per key chained
// with OR nests a level per key, which SQLite refuses from about a thousand
// keys on.
func keyIn(tableName string, columns []string, keys [][]any) clause.IN {
	column, values := gormschema.ToQueryValues(tableName, columns, keys)
	return clause.IN{Column: column, Values: values}
}

func uniqueIndexKey(values []any) string {
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
