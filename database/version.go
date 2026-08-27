package database

import (
	"slices"
	"strings"

	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Optimistic locking mechanics.
//
// The user-facing contract lives on model.Version; this file holds the pieces
// the write paths share. The load-bearing decisions:
//
//   - Update writes the concrete value prev+1 through the normal struct
//     assembly instead of a SET version = version + 1 expression. The two are
//     exactly equivalent — the statement matches WHERE version = prev, so the
//     matched row's version is prev by definition — and the concrete value
//     keeps the write on gorm's standard path: no Dest rewriting, no
//     per-statement schema reflection (the tricks gorm's optimisticlock
//     plugin needs to smuggle an expression into a struct update), and the
//     object naturally carries the new version on success.
//   - UpdateByID and the Upsert conflict branch cannot know the row's
//     version, so they do use the column expression; both build their SET
//     from assignment lists, where an expression is a first-class value.
//   - A failed Update restores the bumped objects, so a rolled-back write
//     leaves every object carrying the version its row still has.

// bumpVersionForUpdate advances obj's carried version by one before its
// UPDATE statement is built, returning the previous value — the one the
// statement must match — and a restore function that undoes the bump.
// The caller has already rejected zero versions, so prev is always >= 1.
func bumpVersionForUpdate[M types.Model](obj M) (prev int64, restore func()) {
	prev, _ = modelregistry.VersionValue(obj)
	modelregistry.SetVersionValue(obj, prev+1)
	return prev, func() { modelregistry.SetVersionValue(obj, prev) }
}

// initializeVersions stamps 1 into every object whose carried version is
// zero: stored versions start at 1, so a zero always identifies an object
// that never was in the database (see model.Version). A non-zero value is
// kept, so imports and sync jobs can carry existing history. Create and
// Upsert both run this over their insert candidates.
func initializeVersions[M types.Model](objs []M) {
	if len(objs) == 0 || !modelregistry.IsVersioned(objs[0]) {
		return
	}
	for i := range objs {
		if v, _ := modelregistry.VersionValue(objs[i]); v == 0 {
			modelregistry.SetVersionValue(objs[i], 1)
		}
	}
}

// ensureVersionSelected widens a narrowed column selection so the bumped
// version still reaches the statement: WithSelect names the caller's columns,
// but a versioned update that did not write the version column would pass its
// check and then leave every other carried version alive.
func ensureVersionSelected(selectColumns []string, versionColumn string) []string {
	if slices.Contains(selectColumns, versionColumn) {
		return selectColumns
	}
	return append(append(make([]string, 0, len(selectColumns)+1), selectColumns...), versionColumn)
}

// versionedOnConflict builds the ON CONFLICT clause Upsert pre-sets for
// versioned models. gorm's slice Save would otherwise add UpdateAll, whose
// expansion assigns "column = VALUES(column)" to every column — version
// included, which would write the object's version over the row's and could
// move a row backwards, silently reviving every stale version out there.
//
// The clause reproduces the UpdateAll expansion verbatim (skip primary keys,
// creation timestamps and defaulted columns; refresh updated_at to now) with
// one divergence: the version column bumps the row's own value. Save adopts a
// pre-set ON CONFLICT clause instead of adding its own, so every other
// slice-save behavior is preserved.
//
// Under WithSelect the conflict update narrows to the selected columns, and
// updated_at is still refreshed: a narrowed upsert that touched a row must
// not leave its update timestamp claiming otherwise.
func (db *database[M]) versionedOnConflict(versionColumn string) (clause.OnConflict, error) {
	stmt := &gorm.Statement{DB: db.ins}
	if err := stmt.Parse(db.m); err != nil {
		return clause.OnConflict{}, err
	}

	selected := make(map[string]struct{}, len(db.selectColumns))
	for _, column := range db.selectColumns {
		selected[column] = struct{}{}
	}

	now := dbruntime.NowUTC()
	// The bump expression qualifies the column with the target table: in
	// postgres an unqualified name inside DO UPDATE SET is ambiguous between
	// the existing row and the excluded pseudo-row (SQLSTATE 42702), and the
	// qualified form reads as the existing row on every dialect.
	doUpdates := clause.Set{{
		Column: clause.Column{Name: versionColumn},
		Value:  clause.Expr{SQL: "? + 1", Vars: []any{clause.Column{Table: clause.CurrentTable, Name: versionColumn}}},
	}}
	columns := make([]string, 0, len(stmt.Schema.DBNames))
	for _, dbName := range stmt.Schema.DBNames {
		field := stmt.Schema.LookUpField(dbName)
		if field == nil || field.PrimaryKey || dbName == versionColumn {
			continue
		}
		// Creation facts belong to the insert; a conflict update keeps them.
		if field.AutoCreateTime > 0 {
			continue
		}
		// gorm's own expansion leaves defaulted columns alone (unless the
		// default is NULL); reproduced so the only divergence from UpdateAll
		// is the version column.
		if field.HasDefaultValue && field.DefaultValueInterface == nil && !strings.EqualFold(field.DefaultValue, "NULL") {
			continue
		}
		if field.AutoUpdateTime > 0 {
			doUpdates = append(doUpdates, clause.Assignment{Column: clause.Column{Name: dbName}, Value: now})
			continue
		}
		if len(selected) > 0 {
			if _, ok := selected[dbName]; !ok {
				continue
			}
		}
		columns = append(columns, dbName)
	}
	doUpdates = append(doUpdates, clause.AssignmentColumns(columns)...)

	onConflict := clause.OnConflict{DoUpdates: doUpdates}
	// The conflict target mirrors gorm's UpdateAll default: the primary key.
	// MySQL ignores it (ON DUPLICATE KEY UPDATE has no target syntax); on
	// postgres and sqlite a conflict on another unique index is not caught,
	// which is the pre-existing Save(slice) behavior, not a regression.
	for _, field := range stmt.Schema.PrimaryFields {
		onConflict.Columns = append(onConflict.Columns, clause.Column{Name: field.DBName})
	}
	return onConflict, nil
}
