package database

import (
	"context"
	"reflect"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/util"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// caller returns the id of the user the context carries, whom the audit
// columns name as creator and updater, or "" when it carries none, as the
// context of a job or a seeding does: the objects then keep what they hold.
func (db *database[M]) caller() string {
	return requestctx.FromContext(db.ctx).UserID()
}

// Create inserts one or multiple records into the database. It is a pure
// INSERT: a record whose primary key or any unique key collides with an
// existing row fails with ErrDuplicatedKey instead of silently updating that
// row. Use Upsert for deliberate insert-or-update semantics. It writes the
// model's own table alone: an association an object carries, a field declared
// for queries to preload, is neither written nor kept on the object, which
// hands back what was written (see clearAssociations). Executes CreateBefore
// and CreateAfter model hooks unless disabled with WithoutHook or WithDryRun.
// Supports batch processing for large datasets using configurable batch sizes.
//
// Parameters:
//   - objs: One or more model instances to create. Empty objects are
//     automatically filtered out.
//
// Behavior:
//   - Automatically generates ID if empty using SetID()
//   - Forces created_at and updated_at to the current time, and created_by
//     and updated_by to the id of the caller the context carries, when it
//     carries one, whatever objs hold. A context carrying no caller, a job's
//     or a seeding's, leaves objs what they hold in those two. A client
//     cannot forge them either way: the request flows clear what the client
//     sent in them before any hook sees the record.
//   - Runs hooks and all batches in one transaction: a failure in any batch or
//     hook rolls back the whole call (all-or-nothing), joining the transaction
//     carried by ctx when present. A call that fits one batch and runs no
//     overridden create hooks skips the wrapping transaction: the single
//     INSERT's own atomicity is the whole contract.
//   - Returns nil if no valid objects provided (empty slice or all objects are
//     empty)
//
// Returns ErrDuplicatedKey when a primary or unique key already exists, or an
// error when hooks or other database constraints fail. WithDryRun builds SQL
// only and does not execute hooks, database I/O, or object field filling.
//
// On a ClickHouse instance the contract is weaker (see clickhouseCreate):
// plain batch INSERTs with no hooks, no transaction across batches, and no
// ErrDuplicatedKey — ClickHouse has no unique constraints, so duplicates are
// stored. A versioned model (model.Version) answers ErrUnsupportedOnDialect
// there.
//
// Example:
//
//	Create(&Sample{Name: "alpha", Code: "a-1"})  // Create single record
//	Create(user1, user2, user3)  // Batch create multiple records
func (db *database[M]) Create(objs ...M) (err error) {
	defer db.reset()

	if db.includeDeleted {
		return errors.Wrap(ErrWithDeletedOnWrite, "Create")
	}
	if db.replicaRead != nil {
		return errors.Wrap(ErrWithReplicaOnWrite, "Create")
	}
	objs = compactModels(objs)
	if len(objs) == 0 {
		return nil
	}

	if err = db.prepare(); err != nil {
		return err
	}
	if err = db.clickhouseVersioned("Create"); err != nil {
		return err
	}
	// A dry run builds its statements from copies, so the objects the caller
	// passed stay as they are (see WithDryRun).
	if db.dryRun {
		objs = cloneDryRunModels(objs)
	}
	// The caller is who created the records: created_by and updated_by take
	// the id the context carries, when it carries one, whatever the objects
	// hold, on every dialect.
	if caller := db.caller(); caller != "" {
		for i := range objs {
			objs[i].SetCreatedBy(caller)
			objs[i].SetUpdatedBy(caller)
		}
	}
	if db.dialect() == dialectClickHouse {
		return db.clickhouseCreate(objs)
	}
	done, span := db.trace(consts.Create, len(objs))
	defer func() { done(err) }()

	batchSize := defaultBatchSize
	if db.batchSize > 0 {
		batchSize = db.batchSize
	}

	if db.dryRun {
		initializeVersions(objs)
		for i := 0; i < len(objs); i += batchSize {
			end := min(i+batchSize, len(objs))
			tx := dryRunSession(db.ins).Omit(clause.Associations).Create(objs[i:end])
			if err = db.collectSQL(tx); err != nil {
				return err
			}
		}
		return nil
	}

	write := func() error {
		// Invoke model hook: CreateBefore for the entire batch.
		if !db.noHook {
			if err = traceModelHook[M](db.ctx, consts.CreateBefore, span, func(spanCtx context.Context) error {
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
		fillForInsert(objs)
		for i := 0; i < len(objs); i += batchSize {
			end := min(i+batchSize, len(objs))
			if err = db.ins.Session(&gorm.Session{}).Omit(clause.Associations).Create(objs[i:end]).Error; err != nil {
				// First-hand exit of a stack-less GORM/driver error: embed the
				// run-time stack here so the error_stack log field can locate
				// any caller — service code, model hooks, cron jobs — without
				// each call site logging or wrapping. See the error-stack
				// contract in doc.go.
				return errors.WithStack(err)
			}
		}
		// Invoke model hook: CreateAfter for the entire batch.
		if !db.noHook {
			if err = traceModelHook[M](db.ctx, consts.CreateAfter, span, func(spanCtx context.Context) error {
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
	}
	// One batch is one multi-row INSERT; without overridden create hooks the
	// statement's own atomicity is the whole contract.
	if (db.noHook || !modelregistry.OverridesCreateHooks(db.m)) && len(objs) <= batchSize {
		return db.withSingleStatementWrite(write)
	}
	return db.withWriteTransaction(write)
}

// Delete removes one or multiple records from the database. By default performs
// soft delete (sets deleted_at timestamp). Use WithPurge() for permanent
// deletion (hard delete). Executes DeleteBefore and DeleteAfter model hooks
// unless disabled with WithoutHook or WithDryRun.
//
// Every record named by its id must match a live row: a record that is
// missing, or soft deleted already, fails the call with ErrRecordNotFound,
// and the batch deletes nothing. A record is named once however many times
// the objects repeat it. Detection relies on matched-rows semantics, like
// Update's: the framework MySQL DSN enables clientFoundRows=true, and custom
// connections must keep that flag. WithAllowMissing lifts the check, for a
// delete that may run after the rows are gone.
//
// A delete by conditions passes no record: WithQuery(...).Delete() removes
// the rows the conditions match, and matching none of them is not a missing
// record. Every row of the table is spelled the one way a query spells it,
// WithQuery(nil, QueryOptions{AllowEmpty: true}).Delete(): on a
// tenant-scoped model that is every row of the caller's tenant, and the rows
// soft delete or purge as the model and WithPurge decide. Records and
// conditions do not combine: a call carrying both fails with
// ErrConditionsWithRecords, since a mix leaves it unclear which rows are
// meant, and a named record the conditions do not match would be skipped in
// silence or reported as missing.
//
// The model's delete hooks run once per record a delete names, and a delete
// by conditions names none: on a model overriding DeleteBefore or
// DeleteAfter it fails with ErrConditionsWithDeleteHooks before any
// statement, unless WithoutHook skips the hooks. Passing the records instead
// hands the hooks the rows they act on.
//
// A versioned record (a model declaring model.Version) is checked when it
// carries a non-zero version — the statement matches it, and a miss fails
// with ErrStaleObject: a delete decided over stale data must fail like a
// stale update. WithAllowMissing does not lift the check, a gone row
// missing it like a moved-on one (see WithAllowMissing). An object without
// a version (a bare-id shell) deletes unconditionally, which is the
// deliberate way around the lock for cascade cleanup and programmatic
// deletion, and passes over a gone row under WithAllowMissing. See
// model.Version.
//
// Parameters:
//   - objs: One or more model instances to delete. Empty objects are
//     automatically filtered out.
//
// Behavior:
//   - Soft delete (default): Sets deleted_at field, records remain in database
//     but are hidden from normal queries
//   - Hard delete (with WithPurge): Permanently removes records from database
//   - Soft-deleted records are automatically excluded from List, Get, First,
//     Last, Count, and other query operations
//   - Supports batch processing for performance
//   - Runs hooks and all batches in one transaction, joining the transaction
//     carried by ctx when present. A call that fits one batch, runs no
//     overridden delete hooks and names at most one record (or allows
//     missing ones) skips the wrapping transaction: the single UPDATE ... IN
//     / DELETE ... IN statement's own atomicity is the whole contract. A
//     batch naming several records that must all exist runs in a
//     transaction, so a statement that found fewer rows than named is rolled
//     back and the batch deletes nothing, and so does a delete of several
//     records carrying versions, a statement each: a stale one rolls back
//     the ones before it.
//   - Returns nil if no valid objects provided (empty slice or all objects are
//     empty) and the chain carries no conditions
//   - WithDryRun builds SQL only and does not execute hooks, database I/O, or
//     object field filling
//
// On a ClickHouse instance the contract is weaker (see clickhouseDelete):
// a lightweight DELETE by primary key with no hooks and no transaction, and
// always physical — ClickHouse has no application-level soft delete, so the
// model's Purge and WithPurge are ignored there — and a missing record
// passes silently instead of ErrRecordNotFound. A delete by conditions, and
// any delete of a versioned model (model.Version), answers
// ErrUnsupportedOnDialect there.
//
// Example:
//
//	Delete(&sample)  // Soft delete by primary key
//	Delete(user1, user2, user3)  // Batch soft delete multiple records
//	WithQuery(params).Delete()  // Delete the rows the conditions match
//	WithQuery(nil, QueryOptions{AllowEmpty: true}).Delete()  // Delete every row of the model's table
//	WithPurge().Delete(&sample)  // Permanent deletion
func (db *database[M]) Delete(objs ...M) (err error) {
	defer db.reset()

	if db.includeDeleted {
		return errors.Wrap(ErrWithDeletedOnWrite, "Delete")
	}
	if db.replicaRead != nil {
		return errors.Wrap(ErrWithReplicaOnWrite, "Delete")
	}
	if err = db.prepare(); err != nil {
		return err
	}
	// Conditions on the chain (WithQuery) name rows, every row of the table
	// when AllowEmpty declared it; records name themselves. A delete passes
	// one or the other.
	conditioned := db.matchAll
	if db.ins.Statement != nil {
		if _, where := db.ins.Statement.Clauses["WHERE"]; where {
			conditioned = true
		}
	}
	objs = compactModels(objs)
	if len(objs) == 0 && !conditioned {
		return nil
	}
	// The records the call names, each once: what the statements must match
	// unless the caller allows missing records.
	named := make(map[string]struct{}, len(objs))
	for i := range objs {
		if id := objs[i].GetID(); id != "" {
			named[id] = struct{}{}
		}
	}
	expected := int64(len(named))
	if conditioned && expected > 0 {
		return errors.Wrap(ErrConditionsWithRecords, "Delete")
	}
	if len(objs) == 0 {
		// A delete by conditions addresses the rows the conditions match
		// through a zero model, which names no record.
		objs = []M{reflect.New(db.typ).Interface().(M)} //nolint:errcheck
	}
	strict := !db.allowMissing && expected > 0
	tableName := db.m.TableName()
	// The delete of every row of the table, declared through AllowEmpty, is
	// the one statement allowed to run without a condition of its own.
	session := &gorm.Session{AllowGlobalUpdate: db.matchAll}

	if err = db.clickhouseVersioned("Delete"); err != nil {
		return err
	}
	if db.dialect() == dialectClickHouse {
		// The lightweight DELETE goes by primary key, and a delete by
		// conditions names no record to go by.
		if conditioned {
			return errors.Wrap(ErrUnsupportedOnDialect, "Delete by conditions on clickhouse")
		}
		return db.clickhouseDelete(objs)
	}
	// A delete by conditions hands the hooks no record of the rows its
	// statement removes (see ErrConditionsWithDeleteHooks). ClickHouse runs
	// no hooks, so the check follows its branch.
	if conditioned && !db.noHook && modelregistry.OverridesDeleteHooks(db.m) {
		return errors.Wrap(ErrConditionsWithDeleteHooks, "Delete")
	}
	done, span := db.trace(consts.Delete, len(objs))
	defer func() { done(err) }()

	batchSize := defaultDeleteBatchSize
	if db.batchSize > 0 {
		batchSize = db.batchSize
	}

	// A delete over a versioned model splits by what the objects carry (see
	// model.Version): when any object carries a non-zero version, every row
	// gets its own statement so each version is checked individually; when
	// none does, the delete is the deliberate unconditional kind and keeps
	// the batched IN statements below.
	versionColumn, versioned := modelschema.VersionColumn(db.m)
	guarded := false
	if versioned {
		for i := range objs {
			if v, _ := modelschema.VersionValue(objs[i]); v > 0 {
				guarded = true
				break
			}
		}
	}

	if db.dryRun {
		dryRunObjs := cloneDryRunModels(objs)
		if guarded {
			for i := range dryRunObjs {
				tx := dryRunSession(db.ins).Session(session)
				if v, _ := modelschema.VersionValue(dryRunObjs[i]); v > 0 {
					tx = tx.Where(db.quoteIdent(versionColumn)+" = ?", v)
				}
				if util.Deref(db.enablePurge) {
					tx = tx.Unscoped()
				}
				if err = db.collectSQL(tx.Delete(dryRunObjs[i])); err != nil {
					return err
				}
			}
			return nil
		}
		for i := 0; i < len(dryRunObjs); i += batchSize {
			end := min(i+batchSize, len(dryRunObjs))
			if util.Deref(db.enablePurge) {
				tx := dryRunSession(db.ins).Session(session).Unscoped().Delete(dryRunObjs[i:end])
				if err = db.collectSQL(tx); err != nil {
					return err
				}
				continue
			}
			tx := dryRunSession(db.ins).Session(session).Delete(dryRunObjs[i:end])
			if err = db.collectSQL(tx); err != nil {
				return err
			}
		}
		return nil
	}

	write := func() error {
		// Invoke model hook: DeleteBefore.
		if !db.noHook {
			if err = traceModelHook[M](db.ctx, consts.DeleteBefore, span, func(spanCtx context.Context) error {
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
		// matched counts the rows the statements found, which must reach the
		// records named unless missing ones are allowed.
		var matched int64
		switch {
		case guarded:
			// Per-row statements, each matching the version its object
			// carries; an object without one deletes unconditionally by id.
			// A carried version that matches nothing means the row was
			// modified or removed after this caller read it — the delete
			// decision was made over stale data and must fail like a stale
			// update. The version column is never bumped here: a soft-deleted
			// row is unreachable to every later write already, and a purged
			// row is gone.
			for i := range objs {
				tx := db.ins.Session(session)
				v, _ := modelschema.VersionValue(objs[i])
				if v > 0 {
					tx = tx.Where(db.quoteIdent(versionColumn)+" = ?", v)
				}
				if util.Deref(db.enablePurge) {
					tx = tx.Unscoped()
				}
				res := tx.Delete(objs[i])
				if res.Error != nil {
					return errors.WithStack(res.Error)
				}
				if v > 0 && res.RowsAffected == 0 {
					return errors.Wrapf(ErrStaleObject, "delete %s id=%s version=%d", tableName, objs[i].GetID(), v)
				}
				matched += res.RowsAffected
			}
		case util.Deref(db.enablePurge):
			// delete permanently.
			for i := 0; i < len(objs); i += batchSize {
				end := min(i+batchSize, len(objs))
				res := db.ins.Session(session).Unscoped().Delete(objs[i:end])
				if res.Error != nil {
					return errors.WithStack(res.Error)
				}
				matched += res.RowsAffected
			}
		default:
			// Soft delete: only set "deleted_at" to the current time. The row
			// keeps occupying its unique keys, so a later Create with the same
			// unique key fails with ErrDuplicatedKey; only Upsert can update
			// such a row again.
			for i := 0; i < len(objs); i += batchSize {
				end := min(i+batchSize, len(objs))
				res := db.ins.Session(session).Delete(objs[i:end])
				if res.Error != nil {
					return errors.WithStack(res.Error)
				}
				matched += res.RowsAffected
			}
		}
		if strict && matched < expected {
			return errors.Wrapf(ErrRecordNotFound, "delete %s: %d of %d records matched", tableName, matched, expected)
		}
		// Invoke model hook: DeleteAfter.
		if !db.noHook {
			if err = traceModelHook[M](db.ctx, consts.DeleteAfter, span, func(spanCtx context.Context) error {
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
	}
	// One batch is one UPDATE ... IN (soft delete) or DELETE ... IN (purge);
	// without overridden delete hooks the statement's own atomicity is the
	// whole contract, as long as the call is one statement that cannot be
	// partly right: one naming several records that must all exist runs in a
	// transaction, so finding fewer rows than named rolls it back, and so do
	// several records checked by version, a statement each.
	if (db.noHook || !modelregistry.OverridesDeleteHooks(db.m)) && len(objs) <= batchSize && (!strict || expected <= 1) && (!guarded || len(objs) <= 1) {
		return db.withSingleStatementWrite(write)
	}
	return db.withWriteTransaction(write)
}

// Update saves the full state of one or multiple existing records. It is a pure
// UPDATE by primary key: it never inserts, and every record must already exist.
// Use Upsert for deliberate insert-or-update semantics. It writes the model's
// own table alone, the associations an object carries zeroed rather than
// written (see clearAssociations). Executes UpdateBefore and UpdateAfter model
// hooks unless disabled with WithoutHook or WithDryRun.
//
// Parameters:
//   - objs: One or more model instances to update. Empty objects are
//     automatically filtered out.
//
// Behavior:
//   - Every object must carry a non-empty ID, otherwise ErrIDRequired is
//     returned before any database work.
//   - Writes the full row including zero values, or only the columns chosen
//     with WithSelect.
//   - Timestamp and audit columns are framework-managed: created_at and
//     created_by are never written (creation facts), deleted_at is never
//     written (rows cannot be soft-deleted or resurrected through Update),
//     updated_at is always refreshed to the current UTC time by GORM
//     (dbruntime.NowUTC) regardless of the value carried by objs, and
//     updated_by is the id of the caller the context carries, when it
//     carries one, regardless of the value carried by objs; with no caller,
//     a job's, it is what objs hold, which the request flows clear of what a
//     client sent.
//   - A record matching no live row (missing or soft deleted) fails with
//     ErrRecordNotFound. Detection relies on matched-rows semantics: the
//     framework MySQL DSN enables clientFoundRows=true so an update that
//     changes nothing still counts as matched instead of being misread as
//     missing. Custom database connections must keep that flag.
//   - A versioned record (a model declaring model.Version) must carry the
//     version it was read with: a zero version fails with
//     ErrVersionRequired, the statement additionally matches that version,
//     and a miss fails with ErrStaleObject instead of ErrRecordNotFound —
//     the row was modified or deleted by someone else after this caller
//     read it. On success the version is bumped by one, in the row and in
//     the object; WithSelect updates bump too. See model.Version for the
//     full contract.
//   - Runs hooks and all row updates in one transaction: any missing record or
//     failed hook rolls back the whole call (all-or-nothing), joining the
//     transaction carried by ctx when present. A single-record call that runs
//     no overridden update hooks skips the wrapping transaction: the one
//     UPDATE statement's own atomicity is the whole contract.
//   - Returns nil if no valid objects provided (empty slice or all objects are
//     empty)
//
// Returns ErrIDRequired when an object has no ID, ErrRecordNotFound when a
// record does not exist (or is soft deleted), and ErrDuplicatedKey when the new
// values collide with a unique key owned by another row. WithDryRun builds SQL
// only and does not execute hooks, database I/O, or object field filling.
//
// On a ClickHouse instance the contract is weaker (see clickhouseUpdate):
// each record becomes an asynchronous ALTER TABLE ... UPDATE mutation — heavy,
// meant for low-frequency data correction — with no hooks, no transaction,
// and no existence detection: a nil error means accepted, not rewritten, and
// a missing record passes silently instead of ErrRecordNotFound. ClickHouse
// refuses to UPDATE an ORDER BY key column, so narrow the write with
// WithSelect to the columns being corrected. A versioned model
// (model.Version) answers ErrUnsupportedOnDialect there.
//
// Example:
//
//	sample.Name = "Updated Name"
//	Update(&sample)  // Update single record
//	Update(first, second, third)  // Update several records in one call
func (db *database[M]) Update(objs ...M) (err error) {
	defer db.reset()

	if db.includeDeleted {
		return errors.Wrap(ErrWithDeletedOnWrite, "Update")
	}
	if db.replicaRead != nil {
		return errors.Wrap(ErrWithReplicaOnWrite, "Update")
	}
	objs = compactModels(objs)
	if len(objs) == 0 {
		return nil
	}
	// A pure UPDATE needs a primary key on every record, and a versioned
	// record must carry the version it was read with (see model.Version);
	// fail fast before any database work so a partially valid batch never
	// starts writing.
	versionColumn, versioned := modelschema.VersionColumn(objs[0])
	for i := range objs {
		if len(objs[i].GetID()) == 0 {
			return ErrIDRequired
		}
		if versioned {
			if v, _ := modelschema.VersionValue(objs[i]); v == 0 {
				return errors.Wrapf(ErrVersionRequired, "update id=%s", objs[i].GetID())
			}
		}
	}

	if err = db.prepare(); err != nil {
		return err
	}
	if err = db.clickhouseVersioned("Update"); err != nil {
		return err
	}
	// A dry run builds its statements from copies, so the objects the caller
	// passed stay as they are (see WithDryRun).
	if db.dryRun {
		objs = cloneDryRunModels(objs)
	}
	// The caller is who updated the records: updated_by takes the id the
	// context carries, when it carries one, whatever the objects hold, on
	// every dialect, and a narrowed update writes it along with its columns,
	// as it does updated_at.
	if caller := db.caller(); caller != "" {
		for i := range objs {
			objs[i].SetUpdatedBy(caller)
		}
		if len(db.selectColumns) > 0 {
			db.selectColumns = ensureSelected(db.selectColumns, "updated_by")
		}
	}
	if db.dialect() == dialectClickHouse {
		return db.clickhouseUpdate(objs)
	}
	done, span := db.trace(consts.Update, len(objs))
	defer func() { done(err) }()

	tableName := db.m.TableName()

	// The bump happens per statement below; on any failure the whole batch is
	// rolled back, so every bumped object is restored to the version its row
	// still has.
	var restoreVersions []func()
	defer func() {
		if err != nil {
			for _, restore := range restoreVersions {
				restore()
			}
		}
	}()
	if versioned && len(db.selectColumns) > 0 {
		// A narrowed update still bumps: leaving the version column out of
		// the SET would pass the check and keep every other carried version
		// alive. See ensureSelected.
		db.selectColumns = ensureSelected(db.selectColumns, versionColumn)
	}

	if db.dryRun {
		for i := range objs {
			session := dryRunSession(db.ins)
			if versioned {
				prev, _ := bumpVersionForUpdate(objs[i])
				session = session.Where(db.quoteIdent(versionColumn)+" = ?", prev)
			}
			tx := db.updateRowStatement(session, objs[i]).Updates(objs[i])
			if err = db.collectSQL(tx); err != nil {
				return err
			}
		}
		return nil
	}

	write := func() error {
		// Invoke model hook: UpdateBefore.
		if !db.noHook {
			if err = traceModelHook[M](db.ctx, consts.UpdateBefore, span, func(spanCtx context.Context) error {
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
		clearAssociations(objs)
		for i := range objs {
			session := db.ins.Session(&gorm.Session{})
			var prev int64
			if versioned {
				var restore func()
				prev, restore = bumpVersionForUpdate(objs[i])
				restoreVersions = append(restoreVersions, restore)
				session = session.Where(db.quoteIdent(versionColumn)+" = ?", prev)
			}
			res := db.updateRowStatement(session, objs[i]).Updates(objs[i])
			if res.Error != nil {
				return errors.WithStack(res.Error)
			}
			// Zero matched rows means no live row satisfies the WHERE;
			// matched-rows semantics make this reliable even when nothing
			// changed (see the doc comment). For a versioned record the
			// condition includes the carried version, so the miss means
			// "modified or deleted by someone else" — ErrStaleObject — while
			// an unversioned miss can only mean the row is gone.
			if res.RowsAffected == 0 {
				if versioned {
					return errors.Wrapf(ErrStaleObject, "update %s id=%s version=%d", tableName, objs[i].GetID(), prev)
				}
				return errors.Wrapf(ErrRecordNotFound, "update %s id=%s", tableName, objs[i].GetID())
			}
		}
		// Invoke model hook: UpdateAfter.
		if !db.noHook {
			if err = traceModelHook[M](db.ctx, consts.UpdateAfter, span, func(spanCtx context.Context) error {
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
	}
	// Update issues one UPDATE per record and relies on rollback when a later
	// record turns out missing, so only a single-record call is a single
	// statement; without overridden update hooks its own atomicity is the
	// whole contract.
	if (db.noHook || !modelregistry.OverridesUpdateHooks(db.m)) && len(objs) == 1 {
		return db.withSingleStatementWrite(write)
	}
	return db.withWriteTransaction(write)
}

// updateRowStatement builds the single-row UPDATE statement Update issues per
// record: full-row semantics by default (Select("*") writes zero values too),
// narrowed by WithSelect when provided, with deleted_at and the associations
// excluded. deleted_at belongs to Delete; omitting it means callers cannot
// soft-delete a row or resurrect one through Update. created_at and
// created_by need no omission: gorm's create-only permission on them (see
// modelregistry.Base) keeps the statement off them, Select("*") included, so
// callers cannot forge creation audit data either.
// updated_at stays writable because GORM's auto-update-time handling always
// overwrites it with the current time, even under a narrowed WithSelect, and
// updated_by stays writable because Update sets it to the caller first (see
// caller). The associations are omitted as well as zeroed (see
// clearAssociations), so the statement never reaches a related table.
func (db *database[M]) updateRowStatement(session *gorm.DB, obj M) *gorm.DB {
	tx := session.Model(obj)
	if len(db.selectColumns) > 0 {
		tx = tx.Select(db.selectColumns)
	} else {
		tx = tx.Select("*")
	}
	return tx.Omit("deleted_at", clause.Associations)
}

// fillForInsert fills objects in for an INSERT, through Create and Upsert
// alike and on every dialect: an id is generated where one is missing
// (SetID), the association fields are zeroed (see clearAssociations),
// created_at and updated_at are forced to the current time whatever the
// objects hold, and a zero version is raised to 1 (see initializeVersions).
// dbruntime.NowUTC owns the time base (UTC, millisecond precision), so the
// timestamps handed back equal what a later read returns. It returns the time
// it forced, for a statement whose own clock has to agree with it.
func fillForInsert[M types.Model](objs []M) time.Time {
	for i := range objs {
		objs[i].SetID() // set id when id is empty.
	}
	clearAssociations(objs)
	now := dbruntime.NowUTC()
	for i := range objs {
		objs[i].SetCreatedAt(now)
		objs[i].SetUpdatedAt(now)
	}
	initializeVersions(objs)
	return now
}

// clearAssociations zeroes the association fields of every object, the
// fields declared for queries to preload (see modelschema.AssociationFields):
// Create, Update and Upsert write the model's own table alone, so a related
// record an object carries is neither written nor handed back as if it
// were. gorm would otherwise write it along, inserting it or, on a conflict,
// leaving it as it is, outside the hooks and the validation the model's own
// rows go through.
func clearAssociations[M types.Model](objs []M) {
	paths := modelschema.AssociationFields(reflect.TypeOf(*new(M)))
	if len(paths) == 0 {
		return
	}
	for i := range objs {
		value := reflect.ValueOf(objs[i]).Elem()
		for _, path := range paths {
			if field, err := value.FieldByIndexErr(path); err == nil {
				field.Set(reflect.Zero(field.Type()))
			}
		}
	}
}

// UpdateByID updates database columns of a single record identified by ID in
// one UPDATE statement. This is a lightweight update operation that bypasses
// model hooks for performance: it writes the assigned columns without
// triggering validation or business logic.
//
// Parameters:
//   - id: The primary key of the record to update. Must not be empty.
//   - assignments: The column-value writes, built through the generated
//     column references (SampleCols.Status.Set(v)), or in generic code through
//     a reference minted for the type parameter. At least one is required.
//
// Behavior:
//   - Automatically updates the updated_at timestamp, and updated_by to the
//     id of the caller the context carries, when it carries one
//   - Does not invoke UpdateBefore/UpdateAfter hooks for performance reasons
//   - Runs as one bare UPDATE without any transaction wrapper; inside an
//     ambient transaction it joins that transaction unchanged
//   - Returns ErrIDRequired if id is empty
//   - Returns ErrNoAssignments without any assignment
//   - Returns ErrEmptyFieldName if an assignment names an empty column
//   - Returns ErrNilValue if an assignment carries a nil value
//   - Returns ErrDuplicateColumn if one column is assigned twice
//   - Returns ErrColumnTable if an assignment was built from another model's
//     column reference, even when this model has a column of the same name
//   - Returns nil (no error) if the record with the given ID does not exist
//   - On a versioned model (model.Version) the version check is waived — the
//     caller holds no object to compare — but the statement still bumps the
//     version column so every carried version out there expires; an explicit
//     version assignment takes the bump over
//   - On a ClickHouse instance the statement is an asynchronous ALTER TABLE
//     ... UPDATE mutation: a nil error means accepted, not rewritten; a
//     versioned model answers ErrUnsupportedOnDialect there
//
// Example:
//
//	UpdateByID(id, SampleCols.Status.Set("active"))
//	UpdateByID("record456", RecordCols.Score.Set(99.99), RecordCols.Kind.Set("exam"))
func (db *database[M]) UpdateByID(id string, assignments ...types.Assignment) (err error) {
	defer db.reset()

	if db.includeDeleted {
		return errors.Wrap(ErrWithDeletedOnWrite, "UpdateByID")
	}
	if db.replicaRead != nil {
		return errors.Wrap(ErrWithReplicaOnWrite, "UpdateByID")
	}
	if len(id) == 0 {
		return ErrIDRequired
	}
	if len(assignments) == 0 {
		return ErrNoAssignments
	}
	updates := make(map[string]any, len(assignments))
	for _, assignment := range assignments {
		if len(assignment.Column()) == 0 {
			return ErrEmptyFieldName
		}
		// The reference names the table it was generated for; a column of
		// another model is refused even when this model has a column of the
		// same name, because that is a wrong-model write, not a typo.
		if len(assignment.Table()) > 0 && assignment.Table() != db.outerTableName() {
			return errors.Wrapf(ErrColumnTable, "UpdateByID column %q belongs to table %q, model %s writes %q", assignment.Column(), assignment.Table(), reflect.TypeOf(*new(M)).Elem().Name(), db.outerTableName())
		}
		if assignment.Value() == nil {
			return ErrNilValue
		}
		if _, exists := updates[assignment.Column()]; exists {
			return ErrDuplicateColumn
		}
		updates[assignment.Column()] = assignment.Value()
	}
	// The caller is who updated the record: updated_by takes the id the
	// context carries, when it carries one, over any assignment of it.
	if caller := db.caller(); caller != "" {
		updates["updated_by"] = caller
	}

	if err = db.prepare(); err != nil {
		return err
	}
	if err = db.clickhouseVersioned("UpdateByID"); err != nil {
		return err
	}
	// The probe clones the shared model metadata, which must keep a zero ID;
	// see normalizeModelID for the coercion hazard it guards against.
	var ok bool
	if id, ok = normalizeModelID(db.m, id); !ok {
		return errors.WithStack(ErrRecordNotFound)
	}
	// A versioned model bumps its version column even here, where the check
	// itself is waived (the caller holds no object to compare): the write
	// must still expire every carried version out there, or a later full
	// Update would silently overwrite what this call wrote. The bump is the
	// column expression because the row's current version is unknown. An
	// explicit version assignment takes the write over; see model.Version.
	if versionColumn, isVersioned := modelschema.VersionColumn(db.m); isVersioned {
		if _, userAssigned := updates[versionColumn]; !userAssigned {
			updates[versionColumn] = gorm.Expr(db.quoteIdent(versionColumn) + " + 1")
		}
	}
	done, _ := db.trace(phaseUpdateByID)
	defer func() { done(err) }()

	if db.dryRun {
		tx := dryRunSession(db.ins).Model(*new(M)).Where("id = ?", id).Updates(updates)
		return db.collectSQL(tx)
	}

	// One UPDATE with no hooks around it: its own atomicity is the whole
	// contract, so GORM's default per-statement transaction is pure overhead.
	// Inside an ambient transaction db.ins is already that transaction's
	// handle and the flag changes nothing.
	if err = db.ins.Session(&gorm.Session{SkipDefaultTransaction: true}).Model(*new(M)).Where("id = ?", id).Updates(updates).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// cloneDryRunModels returns shallow copies so a dry run builds its statements —
// the fields the framework writes first and GORM's dry-run callbacks alike —
// without mutating caller-owned model fields such as ID, timestamps, creator
// and updater, or soft-delete markers.
func cloneDryRunModels[M types.Model](objs []M) []M {
	cloned := make([]M, 0, len(objs))
	for _, obj := range objs {
		cloned = append(cloned, cloneDryRunModel(obj))
	}
	return cloned
}

// cloneDryRunModel copies the struct obj points to into a new one; an obj that
// is not a non-nil pointer to a struct comes back as it is.
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
	model, ok := reflect.TypeAssert[M](cloned)
	if !ok {
		return obj
	}
	return model
}
