package database

import (
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
)

// This file is the ClickHouse write path: Create, Delete, Update and
// UpdateByID dispatch here when the chain's dialect is ClickHouse, so the
// same call site works against an OLTP database and an analytical instance.
// The contract is deliberately weaker than the transactional write path and
// each entry point's doc spells it out: no model hooks, no transaction
// boundary, no unique-key or matched-rows semantics. What stays identical:
// id and timestamp filling, batch splitting, dry-run SQL building, tracing
// and SQL logging.

// clickhouseCreate is Create on a ClickHouse instance: plain batch INSERTs.
// A single INSERT statement lands atomically (within one partition and the
// insert block size), and nothing rolls back earlier batches when a later
// batch fails. ClickHouse has no unique constraints, so a duplicate row is
// stored, never answered with ErrDuplicatedKey.
func (db *database[M]) clickhouseCreate(objs []M) (err error) {
	done, _, _ := db.trace("Create", len(objs))
	defer func() { done(err) }()

	tableName := db.m.GetTableName()
	batchSize := defaultBatchSize
	if db.batchSize > 0 {
		batchSize = db.batchSize
	}

	// No dry-run here: the dialect driver's create callback prepares and
	// executes the INSERT it builds without ever consulting DryRun, so a
	// "dry" run would write real rows. Refusing is the only honest answer
	// until the driver honors DryRun on its prepared-batch path.
	if db.dryRun {
		return errors.Wrap(ErrUnsupportedOnDialect, "dry-run Create on clickhouse")
	}

	for i := range objs {
		objs[i].SetID() // set id when id is empty.
	}
	// Force created_at/updated_at to now (UTC), exactly like the
	// transactional Create; see the timestamp note there.
	now := time.Now().UTC()
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
	return nil
}

// clickhouseDelete is Delete on a ClickHouse instance: a lightweight DELETE
// by primary key. ClickHouse has no application-level soft delete — its own
// lightweight delete already is a mark-then-merge removal — so every delete
// here is physical, regardless of the model's Purge and of WithPurge, and no
// row count is reported.
//
// The statement is built by hand rather than through gorm's Delete: the
// dialect driver renders Delete as the far heavier ALTER TABLE ... DELETE
// mutation, while DELETE FROM completes synchronously by default and leaves
// merges to the background.
func (db *database[M]) clickhouseDelete(objs []M) (err error) {
	// A delete without a primary key would have to guess a row set; fail
	// fast before any statement runs, matching Update's contract.
	ids := make([]string, 0, len(objs))
	for i := range objs {
		id := objs[i].GetID()
		if len(id) == 0 {
			return ErrIDRequired
		}
		ids = append(ids, id)
	}

	done, _, _ := db.trace("Delete", len(objs))
	defer func() { done(err) }()

	batchSize := defaultDeleteBatchSize
	if db.batchSize > 0 {
		batchSize = db.batchSize
	}
	deleteSQL := "DELETE FROM " + db.quoteIdent(db.outerTableName()) + " WHERE " + db.quoteIdent("id") + " IN ?"

	if db.dryRun {
		tx := db.ins.Session(&gorm.Session{DryRun: true}).Exec(deleteSQL, ids)
		return db.collectSQL(tx)
	}
	for i := 0; i < len(ids); i += batchSize {
		end := min(i+batchSize, len(ids))
		if err = db.ins.Session(&gorm.Session{}).Exec(deleteSQL, ids[i:end]).Error; err != nil {
			return err
		}
	}
	return nil
}

// clickhouseUpdate is Update on a ClickHouse instance: one ALTER TABLE ...
// UPDATE mutation per record, rendered by the dialect driver. Mutations are
// asynchronous — a nil error means the mutation was accepted, not that rows
// have been rewritten — and report no matched count, so a record without a
// live row passes silently instead of answering ErrRecordNotFound.
//
// ClickHouse refuses to UPDATE an ORDER BY key column, and the full-row
// semantics of Update touch every column, so an unnarrowed Update fails on
// any table whose key columns it would rewrite. Narrow the write to the
// columns being corrected with WithSelect, or use UpdateByID for one column.
func (db *database[M]) clickhouseUpdate(objs []M) (err error) {
	done, _, _ := db.trace("Update", len(objs))
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

	for i := range objs {
		if err = db.updateRowStatement(db.ins.Session(&gorm.Session{}), tableName, objs[i]).Updates(objs[i]).Error; err != nil {
			return err
		}
	}
	return nil
}
