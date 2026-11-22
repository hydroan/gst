package dbmigrate_test

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/dbmigrate"
	"github.com/stretchr/testify/require"
)

// migrationTarget is one database the tests of a plan migrate, an empty one
// of each dialect, with what a test needs to write a schema in the dialect's
// spelling and to read the database back.
type migrationTarget struct {
	name string
	typ  config.DBType
	cfg  *dbmigrate.DatabaseConfig
	// exec runs a statement against the database, and rows returns each row
	// of a query as its values in order, joined by a comma.
	exec func(statement string)
	rows func(query string) []string
	// quote quotes an identifier, and text is the type of a short text column.
	quote func(name string) string
	text  string
	// tail closes a CREATE TABLE statement.
	tail string
}

// migrationTargets returns an empty database of each dialect, dropped when
// the test ends.
func migrationTargets(t *testing.T) []migrationTarget {
	t.Helper()
	stamp := time.Now().UnixNano()
	rows := func(driver, dsn string) func(string) []string {
		return func(query string) []string {
			db, err := sql.Open(driver, dsn)
			require.NoError(t, err)
			defer db.Close()
			result, err := db.Query(query)
			require.NoError(t, err, query)
			defer result.Close()
			columns, err := result.Columns()
			require.NoError(t, err)
			read := make([]string, 0)
			for result.Next() {
				values := make([]sql.NullString, len(columns))
				targets := make([]any, len(columns))
				for i := range values {
					targets[i] = &values[i]
				}
				require.NoError(t, result.Scan(targets...))
				row := make([]string, len(values))
				for i, value := range values {
					row[i] = strings.TrimSpace(value.String)
				}
				read = append(read, strings.Join(row, ","))
			}
			require.NoError(t, result.Err())
			return read
		}
	}
	backtick := func(name string) string { return "`" + name + "`" }

	mysqlName := fmt.Sprintf("gst_dbmigrate_plan_%d", stamp)
	createMySQLDatabase(t, mysqlDatabaseConfig(), mysqlName)
	t.Cleanup(func() { dropMySQLDatabase(t, mysqlDatabaseConfig(), mysqlName) })
	mysqlConfig := mysqlDatabaseConfig()
	mysqlConfig.Database = mysqlName

	postgresName := fmt.Sprintf("gst_dbmigrate_plan_pg_%d", stamp)
	admin := postgresDatabaseConfig(os.Getenv(config.POSTGRES_DATABASE))
	createPostgresDatabase(t, admin, postgresName)
	t.Cleanup(func() { dropPostgresDatabase(t, admin, postgresName) })
	postgresConfig := postgresDatabaseConfig(postgresName)

	file := filepath.Join(t.TempDir(), "plan.db")

	return []migrationTarget{
		{
			name: "mysql", typ: config.DBMySQL, cfg: mysqlConfig,
			exec:  func(statement string) { execMySQL(t, mysqlConfig, statement) },
			rows:  rows("mysql", mysqlDSN(mysqlConfig)),
			quote: backtick, text: "varchar(64)",
			tail: ") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;",
		},
		{
			name: "postgres", typ: config.DBPostgres, cfg: postgresConfig,
			exec:  func(statement string) { execPostgres(t, postgresConfig, statement) },
			rows:  rows("postgres", postgresDSN(postgresConfig)),
			quote: func(name string) string { return `"` + name + `"` }, text: "varchar(64)",
			tail: ");",
		},
		{
			name: "sqlite", typ: config.DBSqlite, cfg: &dbmigrate.DatabaseConfig{Database: file},
			exec:  func(statement string) { execSQLite(t, file, statement) },
			rows:  rows("sqlite3", file),
			quote: backtick, text: "text",
			tail: ");",
		},
	}
}

// table spells the CREATE TABLE of a table holding an id and the columns
// given, each a short text that is not null, the way the schema dumper
// writes the dialect: one column to a line, every identifier quoted, and the
// id, which the primary key makes not null, not saying so.
func (m migrationTarget) table(name string, columns ...string) string {
	var b strings.Builder
	b.WriteString("CREATE TABLE " + m.quote(name) + " (\n  " + m.quote("id") + " " + m.text + ",\n")
	for _, column := range columns {
		b.WriteString("  " + m.quote(column) + " " + m.text + " NOT NULL,\n")
	}
	b.WriteString("  PRIMARY KEY (" + m.quote("id") + ")\n" + m.tail)
	return b.String()
}

// index spells the CREATE INDEX of an index over one column of a table.
func (m migrationTarget) index(unique bool, name, table, column string) string {
	kind := "INDEX"
	if unique {
		kind = "UNIQUE INDEX"
	}
	return "CREATE " + kind + " " + m.quote(name) + " ON " + m.quote(table) + " (" + m.quote(column) + ");"
}

// nullable returns schema with the column given, declared by table, left
// nullable.
func (m migrationTarget) nullable(schema, column string) string {
	return strings.Replace(schema, m.quote(column)+" "+m.text+" NOT NULL", m.quote(column)+" "+m.text, 1)
}

// insert runs the INSERT of one row into table, the columns and their text
// values in pairs.
func (m migrationTarget) insert(table string, pairs ...string) {
	columns, values := make([]string, 0, len(pairs)/2), make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		columns = append(columns, m.quote(pairs[i]))
		values = append(values, "'"+pairs[i+1]+"'")
	}
	m.exec("INSERT INTO " + m.quote(table) + " (" + strings.Join(columns, ", ") + ") VALUES (" + strings.Join(values, ", ") + ")")
}

// plan plans the migration of the target towards schema, applying nothing.
func (m migrationTarget) plan(t *testing.T, schema ...string) dbmigrate.Plan {
	t.Helper()
	plan, err := dbmigrate.Migrate([]string{strings.Join(schema, "\n")}, m.typ, m.cfg, &dbmigrate.MigrateOption{DryRun: true})
	require.NoError(t, err)
	return plan
}

// migrate migrates the target to schema.
func (m migrationTarget) migrate(t *testing.T, schema ...string) {
	t.Helper()
	_, err := dbmigrate.Migrate([]string{strings.Join(schema, "\n")}, m.typ, m.cfg, nil)
	require.NoError(t, err)
}

// TestMigrateLeavesTheTablesNoModelDeclares pins, on each dialect, that a
// plan stays inside the tables of the models: a table of the database no
// model declares is reported, never dropped, and a change the models make to
// their own table is planned and applied beside it, its rows untouched.
func TestMigrateLeavesTheTablesNoModelDeclares(t *testing.T) {
	for _, target := range migrationTargets(t) {
		t.Run(target.name, func(t *testing.T) {
			target.migrate(t, target.table("samples", "title"))
			target.exec(strings.TrimSuffix(target.table("archives", "note"), ";"))
			target.insert("archives", "id", "a", "note", "kept")

			plan := target.plan(t, target.table("samples", "title"))
			require.False(t, plan.Changed(), "%v", plan.Statements)
			require.Equal(t, []string{"archives"}, plan.UndeclaredTables)
			require.Empty(t, plan.Renames)

			plan = target.plan(t, target.nullable(target.table("samples", "title", "remark"), "remark"))
			require.Len(t, plan.Statements, 1, "%v", plan.Statements)
			require.Contains(t, plan.Statements[0], "remark")
			require.Equal(t, []string{"archives"}, plan.UndeclaredTables)
			require.NoError(t, dbmigrate.Apply(plan, target.typ, target.cfg, nil))
			require.Equal(t, []string{"a,kept"}, target.rows("SELECT "+target.quote("id")+", "+target.quote("note")+" FROM "+target.quote("archives")))
		})
	}
}

// TestMigrateRefusesToDropAColumnUnlessTheDataLossIsAccepted pins, on each
// dialect, what a plan makes of a column the models do not declare: the
// plan drops it and says so, and neither Apply nor a run that is no dry run
// applies the plan until the data loss is accepted. Refused, the column and
// its values stand.
func TestMigrateRefusesToDropAColumnUnlessTheDataLossIsAccepted(t *testing.T) {
	for _, target := range migrationTargets(t) {
		t.Run(target.name, func(t *testing.T) {
			without := target.table("samples", "title")
			target.migrate(t, target.table("samples", "title", "note"))
			target.insert("samples", "id", "a", "title", "first", "note", "kept")
			note := "SELECT " + target.quote("note") + " FROM " + target.quote("samples")

			plan := target.plan(t, without)
			require.True(t, plan.Changed())
			require.Equal(t, []string{"samples.note"}, plan.DroppedColumns)
			require.Empty(t, plan.Renames, "nothing is added for the column to be the rename of")

			err := dbmigrate.Apply(plan, target.typ, target.cfg, nil)
			require.ErrorContains(t, err, "samples.note")
			require.ErrorContains(t, err, "data loss")
			_, err = dbmigrate.Migrate([]string{without}, target.typ, target.cfg, nil)
			require.ErrorContains(t, err, "samples.note")
			require.Equal(t, []string{"kept"}, target.rows(note))

			require.NoError(t, dbmigrate.Apply(plan, target.typ, target.cfg, &dbmigrate.ApplyOption{AcceptDataLoss: true}))
			after := target.plan(t, without)
			require.False(t, after.Changed(), "%v", after.Statements)
			require.Empty(t, after.DroppedColumns)
			require.Equal(t, []string{"a,first"}, target.rows("SELECT * FROM "+target.quote("samples")))
		})
	}
}

// TestMigrateReportsAColumnUnderANewNameAsARename pins, on each dialect, the
// plan of a model field renamed: its statements still drop the old column and
// add the new one, losing the values, and the plan reports the rename beside
// them with the statement that renames. Carried out, the rename keeps the
// values and takes the index over the column along, and the migration planned
// again holds only the rename of that index, which MySQL and PostgreSQL carry
// out as a rename and SQLite, renaming no index, as a drop and a create.
func TestMigrateReportsAColumnUnderANewNameAsARename(t *testing.T) {
	for _, target := range migrationTargets(t) {
		t.Run(target.name, func(t *testing.T) {
			target.migrate(t, target.table("samples", "title"), target.index(false, "idx_samples_title", "samples", "title"))
			target.insert("samples", "id", "a", "title", "first")
			renamed := []string{target.table("samples", "subject"), target.index(false, "idx_samples_subject", "samples", "subject")}

			plan := target.plan(t, renamed...)
			require.Equal(t, []string{"samples.title"}, plan.DroppedColumns)
			require.Len(t, plan.Renames, 1)
			rename := plan.Renames[0]
			require.Equal(t, "column samples.title to subject", rename.String())
			require.Equal(t, []string{"ALTER TABLE " + target.quote("samples") + " RENAME COLUMN " + target.quote("title") + " TO " + target.quote("subject")}, rename.Statements)

			require.NoError(t, dbmigrate.ApplyRenames(plan.Renames, target.typ, target.cfg))
			require.Equal(t, []string{"a,first"}, target.rows("SELECT "+target.quote("id")+", "+target.quote("subject")+" FROM "+target.quote("samples")))

			plan = target.plan(t, renamed...)
			require.Empty(t, plan.Renames)
			require.Empty(t, plan.DroppedColumns)
			if target.typ == config.DBSqlite {
				require.Len(t, plan.Statements, 2, "%v", plan.Statements)
			} else {
				require.Len(t, plan.Statements, 1, "%v", plan.Statements)
				require.Contains(t, plan.Statements[0], "RENAME")
				require.Contains(t, plan.Statements[0], "idx_samples_subject")
			}
			require.NoError(t, dbmigrate.Apply(plan, target.typ, target.cfg, nil))
			require.False(t, target.plan(t, renamed...).Changed())
		})
	}
}

// TestMigrateReportsATableUnderANewNameAsARename pins, on each dialect, the
// plan of a model whose table name changed: its statements create the new
// table empty and leave the old one, which no model declares, and the plan
// reports the rename beside them. Carried out, the rename keeps the rows, and
// the migration planned again holds what the renamed table still lacks, the
// column the model added and the index under its new name.
func TestMigrateReportsATableUnderANewNameAsARename(t *testing.T) {
	for _, target := range migrationTargets(t) {
		t.Run(target.name, func(t *testing.T) {
			target.migrate(t, target.table("samples", "title"), target.index(false, "idx_samples_title", "samples", "title"))
			target.insert("samples", "id", "a", "title", "first")
			moved := []string{
				target.nullable(target.table("records", "title", "note"), "note"),
				target.index(false, "idx_records_title", "records", "title"),
			}

			plan := target.plan(t, moved...)
			require.Equal(t, []string{"samples"}, plan.UndeclaredTables)
			require.Empty(t, plan.DroppedColumns)
			require.Len(t, plan.Renames, 1)
			require.Equal(t, "table samples to records", plan.Renames[0].String())
			require.Len(t, plan.Renames[0].Statements, 1)
			require.Contains(t, plan.Renames[0].Statements[0], target.quote("samples"))
			require.Contains(t, plan.Renames[0].Statements[0], target.quote("records"))

			require.NoError(t, dbmigrate.ApplyRenames(plan.Renames, target.typ, target.cfg))
			require.Equal(t, []string{"a,first"}, target.rows("SELECT * FROM "+target.quote("records")))

			plan = target.plan(t, moved...)
			require.Empty(t, plan.Renames)
			require.Empty(t, plan.UndeclaredTables)
			require.Contains(t, plan.Statements[0], "note")
			if target.typ == config.DBSqlite {
				require.Len(t, plan.Statements, 3, "%v", plan.Statements)
			} else {
				require.Len(t, plan.Statements, 2, "%v", plan.Statements)
				require.Contains(t, plan.Statements[1], "RENAME")
			}
			require.NoError(t, dbmigrate.Apply(plan, target.typ, target.cfg, nil))
			require.False(t, target.plan(t, moved...).Changed())
		})
	}
}

// TestMigrateReportsNoRenameItCannotTell pins, on each dialect, the pairs a
// plan reports no rename for: a table created with a column fewer than the
// table no model declares, which renaming would not keep whole; a column
// dropped beside one added under another definition; and a column dropped
// beside two added that it could equally be.
func TestMigrateReportsNoRenameItCannotTell(t *testing.T) {
	for _, target := range migrationTargets(t) {
		t.Run(target.name, func(t *testing.T) {
			target.migrate(t, target.table("samples", "title", "note"))

			require.Empty(t, target.plan(t, target.table("records", "title")).Renames, "the created table lacks a column of the other")

			nullable := target.nullable(target.table("samples", "subject", "note"), "subject")
			require.Empty(t, target.plan(t, nullable).Renames, "the added column is nullable, the dropped one is not")

			plan := target.plan(t, target.table("samples", "subject", "headline", "note"))
			require.Empty(t, plan.Renames, "title could be either of the two added columns")
			require.Equal(t, []string{"samples.title"}, plan.DroppedColumns)
		})
	}
}

// TestApplyReportsTheStatementThatFailed pins, on each dialect, what a plan
// failing midway leaves and says: the statements start in order, each
// reported as it starts, and the error names the one that failed and what is
// left of the one before it. MySQL commits each statement on its own, so the
// column the first statement added stays; PostgreSQL and SQLite roll the
// plan back, and the column is not there.
func TestApplyReportsTheStatementThatFailed(t *testing.T) {
	for _, target := range migrationTargets(t) {
		t.Run(target.name, func(t *testing.T) {
			target.migrate(t, target.table("samples", "title"))
			target.insert("samples", "id", "a", "title", "same")
			target.insert("samples", "id", "b", "title", "same")

			// Two rows hold the same title, so the unique index cannot be built.
			plan := target.plan(t, target.nullable(target.table("samples", "title", "note"), "note"), target.index(true, "uniq_samples_title", "samples", "title"))
			require.Len(t, plan.Statements, 2, "%v", plan.Statements)

			started := make([]int, 0)
			err := dbmigrate.Apply(plan, target.typ, target.cfg, &dbmigrate.ApplyOption{
				Progress: func(index int, statement string) {
					require.Equal(t, plan.Statements[index], statement)
					started = append(started, index)
				},
			})
			require.Equal(t, []int{0, 1}, started)
			require.ErrorContains(t, err, "statement 2 of 2 failed")

			after := target.plan(t, target.table("samples", "title"))
			if target.typ == config.DBMySQL {
				require.ErrorContains(t, err, "the statements before it stay applied")
				require.Equal(t, []string{"samples.note"}, after.DroppedColumns, "the column of the first statement stays")
			} else {
				require.ErrorContains(t, err, "nothing of the plan is applied")
				require.False(t, after.Changed(), "%v", after.Statements)
			}
		})
	}
}
