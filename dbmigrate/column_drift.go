package dbmigrate

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/sqldef/sqldef/v3/database"
	"github.com/sqldef/sqldef/v3/parser"
)

// sqliteRebuildReference documents the rebuild SQLite prescribes for the
// column changes its ALTER TABLE cannot make.
const sqliteRebuildReference = "https://www.sqlite.org/lang_altertable.html#otheralter"

// refuseSQLiteColumnDrift fails when a column present in both the models'
// schema and the exported current schema has a different definition on the
// two sides — type, nullability, default or check — or when the two sides
// declare different table-level CHECK constraints. sqldef's SQLite generator
// plans no statement for either, because SQLite's ALTER TABLE cannot express
// them, so without this refusal the migration would answer that the database
// is up to date. The error names every table and column that differs, with
// both definitions, and points to the rebuild SQLite documents.
//
// Columns are compared by the definition the parser renders for them, folded
// to lower case: SQLite reads type names and keywords without regard to case,
// and the two sides come from the same renderer anyway, the schema the models
// dump and the statement SQLite stored when that schema was applied. Columns
// one side lacks are not drift: sqldef adds or drops them.
func refuseSQLiteColumnDrift(sqlParser database.Parser, desiredDDLs, currentDDLs string) error {
	desired, err := parseTableColumns(sqlParser, desiredDDLs)
	if err != nil {
		return errors.Wrap(err, "failed to parse the models' schema")
	}
	current, err := parseTableColumns(sqlParser, currentDDLs)
	if err != nil {
		return errors.Wrap(err, "failed to parse the current schema")
	}

	tables := make([]string, 0, len(desired))
	for table := range desired {
		if _, ok := current[table]; ok {
			tables = append(tables, table)
		}
	}
	slices.Sort(tables)

	drifts := make([]string, 0)
	for _, table := range tables {
		want, have := desired[table], current[table]
		for _, column := range want.columns {
			definition, ok := have.definitions[column]
			if !ok || definition == want.definitions[column] {
				continue
			}
			drifts = append(drifts, fmt.Sprintf("column %q of table %q is %q in the database and %q in the model", column, table, definition, want.definitions[column]))
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

// tableColumns is one CREATE TABLE statement reduced to what the drift check
// compares: its columns in declaration order with the definition the parser
// renders for each, and its table-level CHECK constraints.
type tableColumns struct {
	columns     []string
	definitions map[string]string
	checks      []string
}

// parseTableColumns reads every CREATE TABLE statement of ddls into its
// columns and constraints, keyed by the bare table name in lower case.
func parseTableColumns(sqlParser database.Parser, ddls string) (map[string]tableColumns, error) {
	statements, err := sqlParser.Parse(ddls)
	if err != nil {
		return nil, err
	}
	tables := make(map[string]tableColumns)
	for _, statement := range statements {
		ddl, ok := statement.Statement.(*parser.DDL)
		if !ok || ddl.Action != parser.CreateTable || ddl.TableSpec == nil {
			continue
		}
		table := tableColumns{definitions: make(map[string]string)}
		for _, column := range ddl.TableSpec.Columns {
			name := strings.ToLower(column.Name.Name)
			table.columns = append(table.columns, name)
			table.definitions[name] = normalizeDefinition(parser.String(&column.Type))
		}
		for _, check := range ddl.TableSpec.Checks {
			table.checks = append(table.checks, normalizeDefinition(parser.String(check)))
		}
		tables[strings.ToLower(ddl.NewName.Name.Name)] = table
	}
	return tables, nil
}

// normalizeDefinition folds a rendered definition onto one line, in lower
// case, so two renderings of one definition compare equal.
func normalizeDefinition(definition string) string {
	return strings.ToLower(strings.Join(strings.Fields(definition), " "))
}
