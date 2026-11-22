package dbmigrate

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
)

// sqliteRebuildReference documents the rebuild SQLite prescribes for the
// column changes its ALTER TABLE cannot make.
const sqliteRebuildReference = "https://www.sqlite.org/lang_altertable.html#otheralter"

// refuseSQLiteColumnDrift fails when a column present in both the models'
// schema and the schema the database exports has a different definition on
// the two sides — type, nullability, default or check — or when the two
// sides declare different table-level CHECK constraints. sqldef's SQLite
// generator plans no statement for either, because SQLite's ALTER TABLE cannot
// express them, so without this refusal the migration would answer that the
// database is up to date. The error names every table and column that
// differs, with both definitions, and points to the rebuild SQLite documents.
//
// Columns are compared by the definition the parser renders for them, folded
// to lower case: SQLite reads type names and keywords without regard to case,
// and the two sides come from the same renderer anyway, the schema the models
// dump and the statement SQLite stored when that schema was applied. Columns
// one side lacks are not drift: sqldef adds or drops them.
func refuseSQLiteColumnDrift(desired, current schemaShape) error {
	tables := make([]string, 0, len(desired.names))
	for _, name := range desired.names {
		if current.table(name) != nil {
			tables = append(tables, strings.ToLower(name))
		}
	}
	slices.Sort(tables)

	drifts := make([]string, 0)
	for _, table := range tables {
		want, have := desired.table(table), current.table(table)
		for _, column := range want.columns {
			existing, ok := have.column(column.name)
			wanted, held := normalizeDefinition(column.definition), normalizeDefinition(existing.definition)
			if !ok || held == wanted {
				continue
			}
			drifts = append(drifts, fmt.Sprintf("column %q of table %q is %q in the database and %q in the model", strings.ToLower(column.name), table, held, wanted))
		}
		for _, check := range want.checks {
			if !slices.Contains(have.checks, check) {
				drifts = append(drifts, fmt.Sprintf("table %q lacks the model's constraint %q", table, check))
			}
		}
		for _, check := range have.checks {
			if !slices.Contains(want.checks, check) {
				drifts = append(drifts, fmt.Sprintf("table %q carries the constraint %q the model does not declare", table, check))
			}
		}
	}
	if len(drifts) == 0 {
		return nil
	}
	return errors.Newf("the sqlite database cannot be migrated in place: %s. SQLite's ALTER TABLE cannot change a column's type, nullability, default or check, so rebuild each table: create it under a new name from the model, copy the rows over, drop the old table and rename the new one (%s)", strings.Join(drifts, "; "), sqliteRebuildReference)
}
