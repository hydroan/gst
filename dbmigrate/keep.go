package dbmigrate

import (
	"regexp"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/sqldef/sqldef/v3/database"
	"github.com/sqldef/sqldef/v3/parser"
)

// This file holds the indexes a migration is told to keep (see
// MigrateOptions.IgnoreIndexes): which of the entries name one, and the
// schema of the database without them, which is what the plan is made from.

// keepIndexes resolves the entries of MigrateOptions.IgnoreIndexes, each
// table.index, against the schema the database holds and the one the models
// declare. kept lists the indexes to keep, the ones the database holds on a
// table of the models that no model declares, each as table.index the way
// the database spells it; redundant lists the entries naming an index the
// models declare, which the models decide, each the way the models spell
// it. An entry naming an index the database lacks, or a table no model
// declares, names nothing, and one naming an index a second time counts
// once. An entry that is no table.index fails the migration.
func keepIndexes(current, desired schemaShape, entries []string) (kept, redundant []string, err error) {
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		table, index, ok := strings.Cut(strings.TrimSpace(entry), ".")
		if !ok || table == "" || index == "" {
			return nil, nil, errors.Newf("an index to ignore is named table.index, which %q is not", entry)
		}
		key := strings.ToLower(table + "." + index)
		if seen[key] {
			continue
		}
		seen[key] = true
		want := desired.table(table)
		if want == nil {
			continue
		}
		if declared, isDeclared := want.index(index); isDeclared {
			redundant = append(redundant, want.name+"."+declared.name)
			continue
		}
		have := current.table(table)
		if have == nil {
			continue
		}
		if held, isHeld := have.index(index); isHeld {
			kept = append(kept, have.name+"."+held.name)
		}
	}
	return kept, redundant, nil
}

// withoutIndexes returns ddls, the schema a database exports, without the
// indexes of kept, each table.index as keepIndexes names it: a statement
// declaring one by itself, CREATE INDEX or ALTER TABLE ... ADD, is left
// out, and the line declaring one inside the CREATE TABLE of its table, the
// way MySQL exports an index, is taken out of that statement (see
// withoutIndexLine). The other statements stand as the export wrote them,
// rejoined the way a schema of several statements is written. An index that
// is still there once the result is read again fails the migration, which
// would otherwise drop what it is told to keep.
func withoutIndexes(sqlParser database.Parser, ddls string, kept []string) (string, error) {
	statements, err := sqlParser.Parse(ddls)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse the current schema")
	}
	hidden := func(table, index string) bool {
		return slices.ContainsFunc(kept, func(name string) bool { return strings.EqualFold(name, table+"."+index) })
	}
	left := make([]string, 0, len(statements))
	for _, statement := range statements {
		text := strings.TrimRight(strings.TrimSpace(statement.DDL), ";")
		if ddl, isDDL := statement.Statement.(*parser.DDL); isDDL {
			switch ddl.Action {
			case parser.CreateIndex, parser.AddIndex:
				if ddl.IndexSpec != nil && hidden(ddl.Table.Name.Name, ddl.IndexSpec.Name.Name) {
					continue
				}
			case parser.CreateTable:
				if ddl.TableSpec == nil {
					break
				}
				for _, index := range ddl.TableSpec.Indexes {
					if index.Info != nil && hidden(ddl.NewName.Name.Name, index.Info.Name.Name) {
						text = withoutIndexLine(text, index.Info.Name.Name)
					}
				}
			}
		}
		left = append(left, text)
	}
	without := strings.Join(left, ";\n") + ";"

	shape, err := readShape(sqlParser, without)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse the current schema without the indexes kept")
	}
	for _, name := range kept {
		table, index, _ := strings.Cut(name, ".")
		if held := shape.table(table); held != nil {
			if _, still := held.index(index); still {
				return "", errors.Newf("the migration cannot keep the index %s: the schema the database exports declares it in a form the migration cannot set aside; drop the entry and declare the index in its model, or migrate the table by hand", name)
			}
		}
	}
	return without, nil
}

// withoutIndexLine returns createTable, the CREATE TABLE statement of a
// table as MySQL exports it, one definition to a line, without the line
// declaring the index named. The definitions are separated by commas, so
// taking out the last one takes the comma off the one before it. From
//
//	CREATE TABLE `samples` (
//	  `id` char(36) NOT NULL,
//	  `note` varchar(64) NOT NULL,
//	  PRIMARY KEY (`id`),
//	  FULLTEXT KEY `ft_samples_note` (`note`)
//	) ENGINE=InnoDB
//
// and ft_samples_note it returns
//
//	CREATE TABLE `samples` (
//	  `id` char(36) NOT NULL,
//	  `note` varchar(64) NOT NULL,
//	  PRIMARY KEY (`id`)
//	) ENGINE=InnoDB
//
// and the statement as it is when no line of it declares the index that way.
func withoutIndexLine(createTable, index string) string {
	declares := regexp.MustCompile("(?i)^\\s*(?:UNIQUE\\s+|FULLTEXT\\s+|SPATIAL\\s+)?(?:KEY|INDEX)\\s+`" + regexp.QuoteMeta(index) + "`[\\s(]")
	lines := strings.Split(createTable, "\n")
	for i, line := range lines {
		if !declares.MatchString(line) {
			continue
		}
		if i > 0 && !strings.HasSuffix(strings.TrimRight(line, " \t\r"), ",") {
			lines[i-1] = strings.TrimSuffix(strings.TrimRight(lines[i-1], " \t\r"), ",")
		}
		return strings.Join(slices.Delete(lines, i, i+1), "\n")
	}
	return createTable
}
