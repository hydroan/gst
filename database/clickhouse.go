package database

import (
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/dbruntime"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// This file is the ClickHouse write path: Create, Delete, Update and
// UpdateByID dispatch here when the chain's dialect is ClickHouse, so the
// same call site works against an OLTP database and an analytical instance.
// The contract is deliberately weaker than the transactional write path and
// each entry point's doc spells it out: no model hooks, no transaction
// boundary, no unique-key or matched-rows semantics. What stays identical:
// id and timestamp filling, batch splitting, dry-run SQL building, tracing
// and SQL logging, and the clauses a column type attaches to a statement,
// the tenant predicate among them (see tenant.ID).

// clickhouseCreate is Create on a ClickHouse instance: plain batch INSERTs.
// A single INSERT statement lands atomically (within one partition and the
// insert block size), and nothing rolls back earlier batches when a later
// batch fails. ClickHouse has no unique constraints, so a duplicate row is
// stored, never answered with ErrDuplicatedKey.
func (db *database[M]) clickhouseCreate(objs []M) (err error) {
	done, _ := db.trace(consts.Create, len(objs))
	defer func() { done(err) }()

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
	// Force created_at/updated_at to now (dbruntime.NowUTC), exactly like the
	// transactional Create; see the timestamp note there.
	now := dbruntime.NowUTC()
	for i := range objs {
		objs[i].SetCreatedAt(now)
		objs[i].SetUpdatedAt(now)
	}
	for i := 0; i < len(objs); i += batchSize {
		end := min(i+batchSize, len(objs))
		if err = db.ins.Session(&gorm.Session{}).Create(objs[i:end]).Error; err != nil {
			// First-hand exit of a stack-less GORM/driver error; see the
			// error-stack contract in doc.go.
			return errors.WithStack(err)
		}
	}
	return nil
}

// clickhouseDelete is Delete on a ClickHouse instance: a lightweight DELETE
// by primary key. ClickHouse has no application-level soft delete — its own
// lightweight delete already is a mark-then-merge removal — so every delete
// here is physical, regardless of the model's Purge and of WithPurge, and no
// row count is reported: a named row the statement does not reach, one of
// another tenant for instance, passes silently, where the transactional
// path answers ErrRecordNotFound.
//
// The DELETE FROM is spelled by hand rather than rendered through gorm's
// Delete: the dialect driver renders Delete as the far heavier ALTER TABLE
// ... DELETE mutation, while DELETE FROM completes synchronously by default
// and leaves merges to the background. Its WHERE is gorm's own rendering,
// see clickhouseDeleteStatement.
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

	done, _ := db.trace(consts.Delete, len(objs))
	defer func() { done(err) }()

	batchSize := defaultDeleteBatchSize
	if db.batchSize > 0 {
		batchSize = db.batchSize
	}
	for i := 0; i < len(ids); i += batchSize {
		end := min(i+batchSize, len(ids))
		deleteSQL, vars, err := db.clickhouseDeleteStatement(ids[i:end])
		if err != nil {
			return err
		}
		if db.dryRun {
			if err = db.collectSQL(dryRunSession(db.ins).Exec(deleteSQL, vars...)); err != nil {
				return err
			}
			continue
		}
		if err = db.ins.Session(&gorm.Session{}).Exec(deleteSQL, vars...).Error; err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// clickhouseDeleteStatement renders the lightweight DELETE of the rows ids
// name, with the values the placeholders take. The WHERE is gorm's own: the
// statement takes the id list and then, as gorm's delete callback does, the
// clauses the model's schema attaches to a delete, which is how the tenant
// predicate reaches the statement. The soft-delete clause among them is
// kept from turning the statement into an UPDATE by Unscoped, every delete
// here being physical; the tenant predicate ignores Unscoped on purpose
// (see tenant.ID). The columns render without their table qualifier (see
// unqualified). For the id shared-id of a tenant-scoped model on the table
// test_tenant_soft_delete_items, in the tenant tenant-a, the statement
// reads
//
//	DELETE FROM `test_tenant_soft_delete_items` WHERE `id` = ? AND `tenant_id` = ?
//
// with the values shared-id and tenant-a; gorm renders the IN of one id as
// that equality, and several ids as `id` IN (?,?).
func (db *database[M]) clickhouseDeleteStatement(ids []string) (string, []any, error) {
	stmt := db.ins.Session(&gorm.Session{NewDB: true}).Statement
	if err := stmt.Parse(db.m); err != nil {
		return "", nil, errors.WithStack(err)
	}
	stmt.Table = db.outerTableName()
	stmt.Unscoped = true
	values := make([]any, 0, len(ids))
	for _, id := range ids {
		values = append(values, id)
	}
	stmt.AddClause(clause.Where{Exprs: []clause.Expression{clause.IN{Column: clause.PrimaryColumn, Values: values}}})
	for _, c := range stmt.Schema.DeleteClauses {
		stmt.AddClause(c)
	}
	where, err := unqualified(stmt.Clauses["WHERE"].Expression)
	if err != nil {
		return "", nil, err
	}
	stmt.Clauses["WHERE"] = clause.Clause{Name: "WHERE", Expression: where}
	stmt.Build("WHERE")
	return "DELETE FROM " + db.quoteIdent(db.outerTableName()) + " " + stmt.SQL.String(), stmt.Vars, nil
}

// unqualified returns expr with every column naming the current table
// stripped of its table qualifier. The lightweight DELETE of clickhouse
// resolves no qualified column in its WHERE, while gorm renders a column a
// clause names on the current table as table.column without exception. An
// expression of a shape this cannot see into is an error rather than a
// statement clickhouse would then refuse.
func unqualified(expr clause.Expression) (clause.Expression, error) {
	unqualifyAll := func(exprs []clause.Expression) ([]clause.Expression, error) {
		out := make([]clause.Expression, 0, len(exprs))
		for _, e := range exprs {
			u, err := unqualified(e)
			if err != nil {
				return nil, err
			}
			out = append(out, u)
		}
		return out, nil
	}
	unqualifyColumn := func(column any) any {
		if col, ok := column.(clause.Column); ok && col.Table == clause.CurrentTable {
			col.Table = ""
			return col
		}
		return column
	}
	var err error
	switch e := expr.(type) {
	case clause.Where:
		e.Exprs, err = unqualifyAll(e.Exprs)
		return e, err
	case clause.AndConditions:
		e.Exprs, err = unqualifyAll(e.Exprs)
		return e, err
	case clause.OrConditions:
		e.Exprs, err = unqualifyAll(e.Exprs)
		return e, err
	case clause.NotConditions:
		e.Exprs, err = unqualifyAll(e.Exprs)
		return e, err
	case clause.IN:
		e.Column = unqualifyColumn(e.Column)
		return e, nil
	case clause.Eq:
		e.Column = unqualifyColumn(e.Column)
		return e, nil
	case clause.Neq:
		e.Column = unqualifyColumn(e.Column)
		return e, nil
	case clause.Gt:
		e.Column = unqualifyColumn(e.Column)
		return e, nil
	case clause.Gte:
		e.Column = unqualifyColumn(e.Column)
		return e, nil
	case clause.Lt:
		e.Column = unqualifyColumn(e.Column)
		return e, nil
	case clause.Lte:
		e.Column = unqualifyColumn(e.Column)
		return e, nil
	case clause.Like:
		e.Column = unqualifyColumn(e.Column)
		return e, nil
	}
	return nil, errors.Newf("clickhouse delete: cannot render a %T condition without table qualifiers", expr)
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
	done, _ := db.trace(consts.Update, len(objs))
	defer func() { done(err) }()

	if db.dryRun {
		dryRunObjs := cloneDryRunModels(objs)
		for i := range dryRunObjs {
			tx := db.updateRowStatement(dryRunSession(db.ins), dryRunObjs[i]).Updates(dryRunObjs[i])
			if err = db.collectSQL(tx); err != nil {
				return err
			}
		}
		return nil
	}

	for i := range objs {
		if err = db.updateRowStatement(db.ins.Session(&gorm.Session{}), objs[i]).Updates(objs[i]).Error; err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}
