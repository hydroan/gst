package database

import (
	"slices"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/types"
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
//     version, so they do use the column expression: UpdateByID in its
//     assignment map, Upsert in place of the version assignment of gorm's own
//     conflict update (see versionBumpingConflict).
//   - A failed Update restores the bumped objects, so a rolled-back write
//     leaves every object carrying the version its row still has.

// bumpVersionForUpdate advances obj's carried version by one before its
// UPDATE statement is built, returning the previous value — the one the
// statement must match — and a restore function that undoes the bump.
// The caller has already rejected zero versions, so prev is always >= 1.
func bumpVersionForUpdate[M types.Model](obj M) (prev int64, restore func()) {
	prev, _ = modelschema.VersionValue(obj)
	modelschema.SetVersionValue(obj, prev+1)
	return prev, func() { modelschema.SetVersionValue(obj, prev) }
}

// initializeVersions stamps 1 into every object whose carried version is
// zero: stored versions start at 1, so a zero always identifies an object
// that never was in the database (see model.Version). A non-zero value is
// kept, so imports and sync jobs can carry existing history. Create and
// Upsert both run this over their insert candidates.
func initializeVersions[M types.Model](objs []M) {
	if len(objs) == 0 || !modelschema.IsVersioned(objs[0]) {
		return
	}
	for i := range objs {
		if v, _ := modelschema.VersionValue(objs[i]); v == 0 {
			modelschema.SetVersionValue(objs[i], 1)
		}
	}
}

// ensureSelected widens a narrowed column selection by column, a
// framework-managed one the write must carry: WithSelect names the caller's
// columns, but a versioned update that did not write the version column would
// pass its check and then leave every other carried version alive, and an
// update that did not write updated_by would leave the row naming the wrong
// updater.
func ensureSelected(selectColumns []string, column string) []string {
	if slices.Contains(selectColumns, column) {
		return selectColumns
	}
	return append(append(make([]string, 0, len(selectColumns)+1), selectColumns...), column)
}

// versionBumpingConflict returns the conflict update Upsert pre-sets for a
// batch of a versioned model: gorm's own, the UpdateAll expansion its slice
// Save adds, with the version column's assignment swapped for a bump of the
// row's own value. UpdateAll writes the object's version, which would move a
// row backwards and revive every stale version out there.
//
// gorm offers no conflict update that writes one column through an
// expression and expands the rest, so the expansion is asked of gorm itself:
// a dry run of the same statement builds it the way Save would and leaves it
// on the statement, where gorm's own SQL Server dialect reads it too. Which
// columns a merge writes, with their permissions, defaults and time units,
// stays gorm's; only the version column diverges. tx carries the write's
// columns and clock, so the dry run and the write agree on both.
func versionBumpingConflict[M any](tx *gorm.DB, batch []M, versionColumn string) (clause.OnConflict, error) {
	dry := dryRunSession(tx).Session(&gorm.Session{SkipHooks: true}).Clauses(clause.OnConflict{UpdateAll: true}).Save(batch)
	if dry.Error != nil {
		// First-hand exit of a stack-less GORM build error; see the
		// error-stack contract in doc.go.
		return clause.OnConflict{}, errors.WithStack(dry.Error)
	}
	onConflict, ok := dry.Statement.Clauses["ON CONFLICT"].Expression.(clause.OnConflict)
	if !ok {
		return clause.OnConflict{}, errors.New("upsert: gorm built no conflict update")
	}
	// The bump qualifies the column with the target table: in postgres an
	// unqualified name inside DO UPDATE SET is ambiguous between the existing
	// row and the excluded pseudo-row (SQLSTATE 42702), and the qualified form
	// reads as the existing row on every dialect.
	updates := clause.Set{{
		Column: clause.Column{Name: versionColumn},
		Value:  clause.Expr{SQL: "? + 1", Vars: []any{clause.Column{Table: clause.CurrentTable, Name: versionColumn}}},
	}}
	for _, assignment := range onConflict.DoUpdates {
		if assignment.Column.Name != versionColumn {
			updates = append(updates, assignment)
		}
	}
	onConflict.DoUpdates = updates
	onConflict.UpdateAll = false
	onConflict.DoNothing = false
	return onConflict, nil
}
