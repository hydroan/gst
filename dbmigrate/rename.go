package dbmigrate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/sqldef/sqldef/v3/database"
	"github.com/sqldef/sqldef/v3/schema"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Rename is a rename a plan suspects where its statements drop and add: a
// table no model declares beside a table to create that keeps every column
// of it, or a column to drop beside a column to add with the same
// definition. Dropping and adding loses the rows or the values the rename
// keeps, so a plan reports the rename instead of deciding it: ApplyRenames
// carries out the ones a reviewer confirms, and the migration planned again
// then holds neither the drop nor the add.
type Rename struct {
	// Table is the table renamed, or the table holding the renamed column,
	// under the name the database knows it by.
	Table string
	// Column is the column renamed, empty when the table itself is renamed.
	Column string
	// To is the name the models give the table or the column.
	To string
	// Statements carry the rename out, in the syntax of the database.
	Statements []string
}

// String describes the rename: "column samples.title to subject" for a
// column, "table samples to records" for a table.
func (r Rename) String() string {
	if r.Column == "" {
		return fmt.Sprintf("table %s to %s", r.Table, r.To)
	}
	return fmt.Sprintf("column %s.%s to %s", r.Table, r.Column, r.To)
}

// comparison holds what comparing a definition of the models with one of the
// database takes: the dialect, and the parser and default schema sqldef
// compares under.
type comparison struct {
	mode          schema.GeneratorMode
	parser        database.Parser
	defaultSchema string
}

// renames returns the renames between the schema the database has and the
// schema of the models, tables first: a column rename is told inside a table
// both schemas hold, which a renamed table becomes once it is renamed.
//
// A table rename pairs a table of the database no model declares with a
// table of the models the database lacks that keeps every one of its
// columns under the same definition; columns the models add and indexes do
// not count, the migration planned after the rename brings them. A column
// rename pairs, inside one table, a column the models do not declare with
// a column the database lacks of the same definition. Only a pairing that is
// one to one is reported: a table or a column two candidates claim, or one
// claiming two, is reported with none.
func (c comparison) renames(current, desired schemaShape) []Rename {
	dialector := dialectorFor(c.mode)
	renames := make([]Rename, 0)

	undeclared := tablesOnlyIn(current, desired)
	created := tablesOnlyIn(desired, current)
	for _, pair := range oneToOne(len(undeclared), len(created), func(i, j int) bool {
		return c.keepsEveryColumn(undeclared[i], created[j])
	}) {
		from, to := undeclared[pair[0]].name, created[pair[1]].name
		renames = append(renames, Rename{
			Table:      from,
			To:         to,
			Statements: []string{strings.TrimSuffix(dbruntime.RenameTableSQL(dialector, from, to), ";")},
		})
	}

	for _, name := range desired.names {
		want, have := desired.table(name), current.table(name)
		if have == nil {
			continue
		}
		dropped, added := columnsOnlyIn(have, want), columnsOnlyIn(want, have)
		for _, pair := range oneToOne(len(dropped), len(added), func(i, j int) bool {
			return c.sameColumn(dropped[i], added[j])
		}) {
			from, to := dropped[pair[0]].name, added[pair[1]].name
			renames = append(renames, Rename{
				Table:      have.name,
				Column:     from,
				To:         to,
				Statements: []string{renameColumnSQL(dialector, have.name, from, to)},
			})
		}
	}
	return renames
}

// keepsEveryColumn reports whether the table created holds every column of
// the table old under the same definition, so that renaming old into its
// place loses nothing.
func (c comparison) keepsEveryColumn(old, created *tableShape) bool {
	if len(old.columns) == 0 {
		return false
	}
	for _, column := range old.columns {
		kept, ok := created.column(column.name)
		if !ok || !c.sameColumn(column, kept) {
			return false
		}
	}
	return true
}

// sameColumn reports whether a column of the database and a column of the
// models are defined alike, whatever each is named. sqldef is the judge, the
// way it judges the columns of the migration itself: a table of the one
// column planned against a table of the other, each under its primary key
// when it has one, leaves nothing to change, whatever the two spellings,
// varchar(64) collate utf8mb4_bin default null as MySQL exports a column the
// models spell varchar(64). On SQLite sqldef plans nothing for a changed
// column, so the renderings are compared as text there, the way
// refuseSQLiteColumnDrift compares them.
func (c comparison) sameColumn(current, desired columnShape) bool {
	if c.mode == schema.GeneratorModeSQLite3 {
		return current.primary == desired.primary && normalizeDefinition(current.definition) == normalizeDefinition(desired.definition)
	}
	table := func(column columnShape) string {
		if column.primary {
			return "CREATE TABLE gst_column_pair (c " + column.definition + ", PRIMARY KEY (c));"
		}
		return "CREATE TABLE gst_column_pair (c " + column.definition + ");"
	}
	statements, err := schema.GenerateIdempotentDDLs(c.mode, c.parser, table(desired), table(current),
		database.GeneratorConfig{EnableDrop: true}, c.defaultSchema)
	return err == nil && len(statements) == 0
}

// oneToOne returns the pairs (i, j), i below n and j below m, that match
// each other and nothing else: i matches no other j and j no other i.
func oneToOne(n, m int, match func(i, j int) bool) [][2]int {
	matches := make([][]int, n)
	claimed := make([]int, m)
	for i := range n {
		for j := range m {
			if match(i, j) {
				matches[i] = append(matches[i], j)
				claimed[j]++
			}
		}
	}
	pairs := make([][2]int, 0)
	for i, js := range matches {
		if len(js) == 1 && claimed[js[0]] == 1 {
			pairs = append(pairs, [2]int{i, js[0]})
		}
	}
	return pairs
}

// tablesOnlyIn returns the tables of schema that other lacks, in the order
// schema declares them.
func tablesOnlyIn(schema, other schemaShape) []*tableShape {
	tables := make([]*tableShape, 0)
	for _, name := range schema.names {
		if other.table(name) == nil {
			tables = append(tables, schema.table(name))
		}
	}
	return tables
}

// columnsOnlyIn returns the columns of table that other lacks, in the order
// table declares them.
func columnsOnlyIn(table, other *tableShape) []columnShape {
	columns := make([]columnShape, 0)
	for _, column := range table.columns {
		if _, ok := other.column(column.name); !ok {
			columns = append(columns, column)
		}
	}
	return columns
}

// renameColumnSQL returns the statement renaming the column from of table so
// that it is named to, quoted the way dialector quotes: ALTER TABLE
// `samples` RENAME COLUMN `title` TO `subject` on MySQL and SQLite, and
// ALTER TABLE "samples" RENAME COLUMN "title" TO "subject" on PostgreSQL.
// The three databases spell it alike, and each moves the indexes over the
// column along with it.
func renameColumnSQL(dialector gorm.Dialector, table, from, to string) string {
	return fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s",
		dbruntime.QuoteIdentifier(dialector, table),
		dbruntime.QuoteIdentifier(dialector, from),
		dbruntime.QuoteIdentifier(dialector, to))
}

// dialectorFor returns the gorm dialector of the database the generator mode
// targets, connected to nothing: dbruntime quotes the identifiers and spells
// the rename statements with it, the way gorm speaks to that server.
func dialectorFor(mode schema.GeneratorMode) gorm.Dialector {
	switch mode {
	case schema.GeneratorModePostgres:
		return postgres.New(postgres.Config{})
	case schema.GeneratorModeSQLite3:
		return sqlite.New(sqlite.Config{})
	}
	return mysql.New(mysql.Config{})
}

// renamedIndexes returns the indexes the models declare under a name the
// database lacks while it holds, on the same table, an index of the same
// definition under a name no model declares: for each, the name the models
// give it mapped to the name the database knows it by. Only an index a
// statement of its own declares is told, the form the models' Indexes
// method is written in, and only a pairing that is one to one.
func renamedIndexes(current, desired schemaShape) map[string]string {
	renamed := make(map[string]string)
	for _, name := range desired.names {
		want, have := desired.table(name), current.table(name)
		if have == nil {
			continue
		}
		var dropped, added []indexShape
		for _, index := range have.indexes {
			if _, ok := want.index(index.name); !ok {
				dropped = append(dropped, index)
			}
		}
		for _, index := range want.indexes {
			if _, ok := have.index(index.name); !ok && index.standalone {
				added = append(added, index)
			}
		}
		for _, pair := range oneToOne(len(dropped), len(added), func(i, j int) bool {
			return dropped[i].definition == added[j].definition
		}) {
			renamed[added[pair[1]].name] = dropped[pair[0]].name
		}
	}
	return renamed
}

// annotateRenamedIndexes marks, in the models' schema, each index of renamed
// as the rename of the index the database holds, with the annotation sqldef
// reads: for the index uniq_records_code renamed from uniq_samples_code,
//
//	CREATE UNIQUE INDEX `uniq_records_code` ON `records` (`code`);
//
// becomes
//
//	CREATE UNIQUE INDEX `uniq_records_code` /* @renamed from="uniq_samples_code" */ ON `records` (`code`);
//
// and sqldef plans the rename of the index where it would plan dropping and
// building it again. A name sqldef's annotation cannot carry, one holding a
// double quote, is left unmarked.
func annotateRenamedIndexes(ddls string, renamed map[string]string) string {
	for to, from := range renamed {
		if strings.Contains(from, `"`) {
			continue
		}
		statement := regexp.MustCompile("(?i)(CREATE\\s+(?:UNIQUE\\s+)?INDEX\\s+[`\"]?" + regexp.QuoteMeta(to) + "[`\"]?)(\\s+ON\\s)")
		ddls = statement.ReplaceAllString(ddls, "${1} /* @renamed from=\""+strings.ReplaceAll(from, "$", "$$")+"\" */${2}")
	}
	return ddls
}
