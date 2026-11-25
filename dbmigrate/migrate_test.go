package dbmigrate_test

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	gstmysql "github.com/hydroan/gst/database/mysql"
	"github.com/hydroan/gst/dbmigrate"
	"github.com/hydroan/gst/internal/model"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/testutil/testcontainer"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

// runTests prepares both databases the migration tests target. They need two at
// once and no server at all, which is what SetupDatabase is for; os.Exit in
// TestMain would skip the deferred releases, hence the wrapper.
func runTests(m *testing.M) int {
	releaseMySQL, _, err := testcontainer.SetupDatabase(config.DBMySQL)
	if err != nil {
		panic(err)
	}
	defer func() { _ = releaseMySQL() }()

	releasePostgres, _, err := testcontainer.SetupDatabase(config.DBPostgres)
	if err != nil {
		panic(err)
	}
	defer func() { _ = releasePostgres() }()

	return m.Run()
}

// TestSQLiteTarget pins the migration target of a sqlite configuration, the
// examples of SQLiteTarget's comment among them: the file the path names,
// parameters and all, and none for a configuration selecting the in-memory
// database.
func TestSQLiteTarget(t *testing.T) {
	for _, path := range []string{"./data.db", "/var/lib/app/data.db?_busy_timeout=1000"} {
		target, ok := dbmigrate.SQLiteTarget(config.Sqlite{Path: path})
		require.True(t, ok, path)
		require.Equal(t, &dbmigrate.DatabaseConfig{Database: path}, target)
	}

	for _, cfg := range []config.Sqlite{
		{IsMemory: true, Path: "./data.db"},
		{},
		{Path: "file::memory:?cache=shared"},
	} {
		target, ok := dbmigrate.SQLiteTarget(cfg)
		require.False(t, ok, "%+v", cfg)
		require.Nil(t, target, "%+v", cfg)
	}
}

func TestMigrate(t *testing.T) {
	t.Run("mysql", func(t *testing.T) {
		dumper, err := dbmigrate.NewSchemaDumper()
		require.NoError(t, err)
		schema, err := dumper.Dump(config.DBMySQL, User{}, Box{}, Sample{}, DefaultedRecord{})
		require.NoError(t, err)

		database := fmt.Sprintf("gst_dbmigrate_test_%d", time.Now().UnixNano())
		createMySQLDatabase(t, mysqlDatabaseConfig(), database)
		t.Cleanup(func() {
			dropMySQLDatabase(t, mysqlDatabaseConfig(), database)
		})
		databaseConfig := mysqlDatabaseConfig()
		databaseConfig.Database = database
		// The sslmode of a PostgreSQL target is nothing a MySQL target reads.
		databaseConfig.SSLMode = "disable"

		plan, err := dbmigrate.Migrate([]string{schema}, config.DBMySQL, databaseConfig,
			&dbmigrate.MigrateOptions{
				DryRun: true,
			})
		require.NoError(t, err)
		require.True(t, plan.Changed())

		plan, err = dbmigrate.Migrate([]string{schema}, config.DBMySQL, databaseConfig,
			&dbmigrate.MigrateOptions{})
		require.NoError(t, err)
		require.True(t, plan.Changed())

		// A converged schema plans nothing on a re-run, custom indexes included.
		plan, err = dbmigrate.Migrate([]string{schema}, config.DBMySQL, databaseConfig,
			&dbmigrate.MigrateOptions{
				DryRun: true,
			})
		require.NoError(t, err)
		require.False(t, plan.Changed())
	})

	t.Run("postgres", func(t *testing.T) {
		dumper, err := dbmigrate.NewSchemaDumper()
		require.NoError(t, err)
		schema, err := dumper.Dump(config.DBPostgres, User{}, Box{}, Sample{}, DefaultedRecord{})
		require.NoError(t, err)

		database := fmt.Sprintf("gst_dbmigrate_test_%d", time.Now().UnixNano())
		adminConfig := postgresDatabaseConfig(os.Getenv(config.POSTGRES_DATABASE))
		createPostgresDatabase(t, adminConfig, database)
		t.Cleanup(func() {
			dropPostgresDatabase(t, adminConfig, database)
		})
		databaseConfig := postgresDatabaseConfig(database)

		plan, err := dbmigrate.Migrate(
			[]string{schema}, config.DBPostgres,
			databaseConfig,
			&dbmigrate.MigrateOptions{
				DryRun: true,
			},
		)
		require.NoError(t, err)
		require.True(t, plan.Changed())

		plan, err = dbmigrate.Migrate(
			[]string{schema}, config.DBPostgres,
			databaseConfig,
			&dbmigrate.MigrateOptions{},
		)
		require.NoError(t, err)
		require.True(t, plan.Changed())

		plan, err = dbmigrate.Migrate(
			[]string{schema}, config.DBPostgres,
			databaseConfig,
			&dbmigrate.MigrateOptions{
				DryRun: true,
			},
		)
		require.NoError(t, err)
		require.False(t, plan.Changed())
	})

	t.Run("sqlite", func(t *testing.T) {
		dumper, err := dbmigrate.NewSchemaDumper()
		require.NoError(t, err)
		schema, err := dumper.Dump(config.DBSqlite, User{}, Box{}, Sample{}, DefaultedRecord{})
		require.NoError(t, err)

		database := filepath.Join(t.TempDir(), "test.db")
		plan, err := dbmigrate.Migrate([]string{schema}, config.DBSqlite,
			&dbmigrate.DatabaseConfig{
				Database: database,
			},
			&dbmigrate.MigrateOptions{
				DryRun: true,
			})
		require.NoError(t, err)
		require.True(t, plan.Changed())

		plan, err = dbmigrate.Migrate([]string{schema}, config.DBSqlite,
			&dbmigrate.DatabaseConfig{
				Database: database,
			},
			&dbmigrate.MigrateOptions{})
		require.NoError(t, err)
		require.True(t, plan.Changed())

		plan, err = dbmigrate.Migrate([]string{schema}, config.DBSqlite,
			&dbmigrate.DatabaseConfig{
				Database: database,
			},
			&dbmigrate.MigrateOptions{
				DryRun: true,
			})
		require.NoError(t, err)
		require.False(t, plan.Changed())
	})
}

// TestMigratePlansAMissingSqliteFileWithoutCreatingIt pins that a sqlite
// database file that does not exist yet is planned against as the empty
// database it would be, whichever way the path spells it — plainly, with the
// driver's parameters, or as a file URI, relative or absolute, whose escapes
// name a space: a dry run creates nothing and reports that applying the plan
// creates the file, and applying it does. Planned again, the file needs
// nothing.
func TestMigratePlansAMissingSqliteFileWithoutCreatingIt(t *testing.T) {
	dumper, err := dbmigrate.NewSchemaDumper()
	require.NoError(t, err)
	schema, err := dumper.Dump(config.DBSqlite, User{})
	require.NoError(t, err)

	dir := filepath.Join(t.TempDir(), "with space")
	require.NoError(t, os.Mkdir(dir, 0o755))
	t.Chdir(dir)
	absolute := filepath.Join(dir, "absolute uri.db")

	for _, tc := range []struct {
		name string
		file string
		dsn  string
	}{
		{name: "plain_path", file: "plain.db", dsn: "plain.db"},
		{name: "path_with_parameters", file: "parameters.db", dsn: "parameters.db?_busy_timeout=1000"},
		{name: "relative_file_uri", file: "relative uri.db", dsn: "file:relative%20uri.db?cache=private"},
		{name: "absolute_file_uri", file: absolute, dsn: "file:" + (&url.URL{Path: absolute}).EscapedPath() + "?cache=private"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &dbmigrate.DatabaseConfig{Database: tc.dsn}

			plan, err := dbmigrate.Migrate([]string{schema}, config.DBSqlite, cfg, &dbmigrate.MigrateOptions{DryRun: true})
			require.NoError(t, err)
			require.True(t, plan.Changed())
			require.True(t, plan.CreatesDatabase)
			require.NoFileExists(t, tc.file, "a dry run creates nothing")

			require.NoError(t, dbmigrate.Apply(plan, config.DBSqlite, cfg, nil))
			require.FileExists(t, tc.file)

			plan, err = dbmigrate.Migrate([]string{schema}, config.DBSqlite, cfg, &dbmigrate.MigrateOptions{DryRun: true})
			require.NoError(t, err)
			require.False(t, plan.Changed())
			require.False(t, plan.CreatesDatabase)
		})
	}
}

// TestMigrateRefusesASqliteFileItCannotCreate pins that a sqlite database
// file that cannot come into being is refused while planning instead of
// promised: the driver creates a missing file but not the directory it goes
// in, and none where a regular file stands in for that directory. The refusal
// names the cause, and nothing is created on the way.
func TestMigrateRefusesASqliteFileItCannotCreate(t *testing.T) {
	dumper, err := dbmigrate.NewSchemaDumper()
	require.NoError(t, err)
	schema, err := dumper.Dump(config.DBSqlite, User{})
	require.NoError(t, err)
	root := t.TempDir()
	plan := func(file string) error {
		_, err := dbmigrate.Migrate([]string{schema}, config.DBSqlite, &dbmigrate.DatabaseConfig{Database: file}, &dbmigrate.MigrateOptions{DryRun: true})
		return err
	}

	t.Run("missing_directory", func(t *testing.T) {
		dir := filepath.Join(root, "missing")
		file := filepath.Join(dir, "data.db")

		require.ErrorContains(t, plan(file), "the sqlite database file "+file+" cannot be created: its directory "+dir+" does not exist")
		require.NoDirExists(t, dir)
	})

	t.Run("regular_file_in_place_of_the_directory", func(t *testing.T) {
		notes := filepath.Join(root, "notes.txt")
		require.NoError(t, os.WriteFile(notes, nil, 0o644))
		file := filepath.Join(notes, "data.db")

		err := plan(file)
		require.ErrorContains(t, err, "failed to inspect the sqlite database file "+file)
		require.ErrorIs(t, err, syscall.ENOTDIR)
	})
}

// TestMigrateDropsRemovedIndex pins the planner's drop path for a secondary
// index that disappears from the desired schema: it is planned as a drop,
// which loses no data and so is no data loss to accept, and applying the plan
// drops it. The fixture index comes from a struct tag because that is the
// shape a base-struct tag removal produces on every table at once, and tag
// indexes flow through the same desired-DDL rendering as everything else the
// dumper emits.
func TestMigrateDropsRemovedIndex(t *testing.T) {
	dumper, err := dbmigrate.NewSchemaDumper()
	require.NoError(t, err)
	defer dumper.Close()

	withIndex, err := dumper.Dump(config.DBMySQL, &TagIndexedWidget{})
	require.NoError(t, err)
	require.Contains(t, withIndex, "`idx_widgets_tag`")
	withoutIndex, err := dumper.Dump(config.DBMySQL, &PlainWidget{})
	require.NoError(t, err)
	require.NotContains(t, withoutIndex, "idx_widgets_tag")

	database := fmt.Sprintf("gst_dbmigrate_dropindex_%d", time.Now().UnixNano())
	createMySQLDatabase(t, mysqlDatabaseConfig(), database)
	t.Cleanup(func() {
		dropMySQLDatabase(t, mysqlDatabaseConfig(), database)
	})
	databaseConfig := mysqlDatabaseConfig()
	databaseConfig.Database = database

	plan, err := dbmigrate.Migrate([]string{withIndex}, config.DBMySQL, databaseConfig, &dbmigrate.MigrateOptions{})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Equal(t, 1, mysqlIndexCount(t, databaseConfig, "widgets", "idx_widgets_tag"))

	// The removal is planned, with no column dropped beside it...
	plan, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBMySQL, databaseConfig, &dbmigrate.MigrateOptions{DryRun: true})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Empty(t, plan.DroppedColumns)
	require.Equal(t, 1, mysqlIndexCount(t, databaseConfig, "widgets", "idx_widgets_tag"))

	// ...and applying it drops the index for good.
	plan, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBMySQL, databaseConfig, &dbmigrate.MigrateOptions{})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Equal(t, 0, mysqlIndexCount(t, databaseConfig, "widgets", "idx_widgets_tag"))

	// The converged schema plans nothing on a re-run.
	plan, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBMySQL, databaseConfig, &dbmigrate.MigrateOptions{DryRun: true})
	require.NoError(t, err)
	require.False(t, plan.Changed())
}

// TestMigrateDropsRemovedIndexOnPostgres pins the same drop path on
// postgres, where the shape differs from mysql on both sides: the dumper
// renders a tag index as a standalone CREATE INDEX statement, and the plan
// drops it with postgres DDL.
func TestMigrateDropsRemovedIndexOnPostgres(t *testing.T) {
	dumper, err := dbmigrate.NewSchemaDumper()
	require.NoError(t, err)
	defer dumper.Close()

	withIndex, err := dumper.Dump(config.DBPostgres, &TagIndexedWidget{})
	require.NoError(t, err)
	require.Contains(t, withIndex, "idx_widgets_tag")
	withoutIndex, err := dumper.Dump(config.DBPostgres, &PlainWidget{})
	require.NoError(t, err)
	require.NotContains(t, withoutIndex, "idx_widgets_tag")

	database := fmt.Sprintf("gst_dbmigrate_dropindex_pg_%d", time.Now().UnixNano())
	adminConfig := postgresDatabaseConfig(os.Getenv(config.POSTGRES_DATABASE))
	createPostgresDatabase(t, adminConfig, database)
	t.Cleanup(func() {
		dropPostgresDatabase(t, adminConfig, database)
	})
	databaseConfig := postgresDatabaseConfig(database)

	plan, err := dbmigrate.Migrate([]string{withIndex}, config.DBPostgres, databaseConfig, &dbmigrate.MigrateOptions{})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Equal(t, 1, postgresIndexCount(t, databaseConfig, "widgets", "idx_widgets_tag"))

	// The removal is planned, with no column dropped beside it...
	plan, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBPostgres, databaseConfig, &dbmigrate.MigrateOptions{DryRun: true})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Empty(t, plan.DroppedColumns)
	require.Equal(t, 1, postgresIndexCount(t, databaseConfig, "widgets", "idx_widgets_tag"))

	// ...and applying it drops the index for good.
	plan, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBPostgres, databaseConfig, &dbmigrate.MigrateOptions{})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Equal(t, 0, postgresIndexCount(t, databaseConfig, "widgets", "idx_widgets_tag"))

	// The converged schema plans nothing on a re-run.
	plan, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBPostgres, databaseConfig, &dbmigrate.MigrateOptions{DryRun: true})
	require.NoError(t, err)
	require.False(t, plan.Changed())
}

// TestApplyRunsThePlanItWasGiven pins what a reviewer's approval covers: the
// statements they were shown. The plan is computed once, against the schema
// the database had then, and Apply runs it as it stands — it does not plan
// again against whatever the database has by the time the answer comes, which
// is a different plan for a database that moved in between.
func TestApplyRunsThePlanItWasGiven(t *testing.T) {
	database := fmt.Sprintf("gst_dbmigrate_apply_%d", time.Now().UnixNano())
	createMySQLDatabase(t, mysqlDatabaseConfig(), database)
	t.Cleanup(func() { dropMySQLDatabase(t, mysqlDatabaseConfig(), database) })
	databaseConfig := mysqlDatabaseConfig()
	databaseConfig.Database = database

	schema := "CREATE TABLE `samples` (\n" +
		"  `id` varchar(36) NOT NULL,\n" +
		"  `code` varchar(64) NOT NULL,\n" +
		"  PRIMARY KEY (`id`)\n" +
		") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;"

	plan, err := dbmigrate.Migrate([]string{schema}, config.DBMySQL, databaseConfig,
		&dbmigrate.MigrateOptions{DryRun: true})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Contains(t, strings.Join(plan.Statements, "\n"), "CREATE TABLE", "the plan carries the statements it showed")

	require.NoError(t, dbmigrate.Apply(plan, config.DBMySQL, databaseConfig, nil))

	// The schema now matches, so nothing is planned any more.
	after, err := dbmigrate.Migrate([]string{schema}, config.DBMySQL, databaseConfig,
		&dbmigrate.MigrateOptions{DryRun: true})
	require.NoError(t, err)
	require.False(t, after.Changed())

	// The plan already applied is still the same statements: running it again
	// fails on the table it would create, instead of quietly planning nothing.
	// That failure is the proof the plan is not recomputed.
	require.Error(t, dbmigrate.Apply(plan, config.DBMySQL, databaseConfig, nil))
}

// TestMigrateReachesAMySQLTargetUnderTheTLSKeysOfTheApplication pins that a
// migration connects to a MySQL target the way the application does, under
// the TLS keys of its mysql section: a server that takes encrypted
// connections alone refuses it while the section leaves TLS off, and is
// planned against and migrated once the section turns it on.
func TestMigrateReachesAMySQLTargetUnderTheTLSKeysOfTheApplication(t *testing.T) {
	plain, terminate, err := testcontainer.SetupStandaloneMySQL("sample", "root", "sample-pass9")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, terminate()) })

	secure := plain
	secure.TLSEnabled = true
	admin, err := gstmysql.New(secure)
	require.NoError(t, err)
	require.NoError(t, admin.Exec("SET GLOBAL require_secure_transport = ON").Error)
	pool, err := admin.DB()
	require.NoError(t, err)
	require.NoError(t, pool.Close())

	target := &dbmigrate.DatabaseConfig{
		Host:     plain.Host,
		Port:     int(plain.Port),
		Database: plain.Database,
		Username: plain.Username,
		Password: plain.Password,
	}
	schema := "CREATE TABLE `records` (`id` varchar(36) NOT NULL, PRIMARY KEY (`id`));"

	section := config.App.MySQL
	t.Cleanup(func() { config.App.MySQL = section })

	config.App.MySQL = plain
	_, err = dbmigrate.Migrate([]string{schema}, config.DBMySQL, target, &dbmigrate.MigrateOptions{DryRun: true})
	require.ErrorContains(t, err, "Error 3159")

	config.App.MySQL = secure
	plan, err := dbmigrate.Migrate([]string{schema}, config.DBMySQL, target, &dbmigrate.MigrateOptions{DryRun: true})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.NoError(t, dbmigrate.Apply(plan, config.DBMySQL, target, nil))

	plan, err = dbmigrate.Migrate([]string{schema}, config.DBMySQL, target, &dbmigrate.MigrateOptions{DryRun: true})
	require.NoError(t, err)
	require.False(t, plan.Changed(), "the table the plan created is there")
}

// newDatabaseConfig reads back the connection the test container was prepared on.
func newDatabaseConfig(hostKey, portKey, userKey, passwordKey, database string) *dbmigrate.DatabaseConfig {
	port, err := strconv.Atoi(os.Getenv(portKey))
	if err != nil {
		panic(err)
	}
	return &dbmigrate.DatabaseConfig{
		Host:     os.Getenv(hostKey),
		Port:     port,
		Username: os.Getenv(userKey),
		Password: os.Getenv(passwordKey),
		Database: database,
	}
}

func mysqlDatabaseConfig() *dbmigrate.DatabaseConfig {
	return newDatabaseConfig(config.MYSQL_HOST, config.MYSQL_PORT, config.MYSQL_USERNAME, config.MYSQL_PASSWORD,
		os.Getenv(config.MYSQL_DATABASE))
}

func createMySQLDatabase(t *testing.T, cfg *dbmigrate.DatabaseConfig, database string) {
	t.Helper()
	execMySQL(t, cfg, "CREATE DATABASE "+database)
}

func dropMySQLDatabase(t *testing.T, cfg *dbmigrate.DatabaseConfig, database string) {
	t.Helper()

	db, err := sql.Open("mysql", mysqlDSN(cfg))
	require.NoError(t, err)
	defer db.Close()

	_, _ = db.Exec("DROP DATABASE IF EXISTS " + database)
}

// execMySQL runs one statement on the configured MySQL database. The driver
// is registered by the sqldef mysql package that dbmigrate itself imports.
func execMySQL(t *testing.T, cfg *dbmigrate.DatabaseConfig, statement string) {
	t.Helper()

	db, err := sql.Open("mysql", mysqlDSN(cfg))
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(statement)
	require.NoError(t, err)
}

func mysqlDSN(cfg *dbmigrate.DatabaseConfig) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database)
}

// mysqlIndexCount reports whether the named index exists on one table, as the
// number of matching index names information_schema holds: 1 when present, 0
// when dropped.
func mysqlIndexCount(t *testing.T, cfg *dbmigrate.DatabaseConfig, table, index string) int {
	t.Helper()

	db, err := sql.Open("mysql", mysqlDSN(cfg))
	require.NoError(t, err)
	defer db.Close()

	var count int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(DISTINCT index_name) FROM information_schema.statistics WHERE table_schema = ? AND table_name = ? AND index_name = ?",
		cfg.Database, table, index,
	).Scan(&count))
	return count
}

// postgresIndexCount reports whether the named index exists on one table, as
// the number of matching index names pg_indexes holds: 1 when present, 0
// when dropped.
func postgresIndexCount(t *testing.T, cfg *dbmigrate.DatabaseConfig, table, index string) int {
	t.Helper()

	db, err := sql.Open("postgres", postgresDSN(cfg))
	require.NoError(t, err)
	defer db.Close()

	var count int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM pg_indexes WHERE schemaname = 'public' AND tablename = $1 AND indexname = $2",
		table, index,
	).Scan(&count))
	return count
}

// TagIndexedWidget and PlainWidget share one table: the second is the first
// with its struct-tag index removed, which is the change
// TestMigrateDropsRemovedIndex drives through the planner.
type TagIndexedWidget struct {
	Tag string `json:"tag" gorm:"size:191;index"`

	model.Base
}

func (*TagIndexedWidget) TableName() string { return "widgets" }

type PlainWidget struct {
	Tag string `json:"tag" gorm:"size:191"`

	model.Base
}

func (*PlainWidget) TableName() string { return "widgets" }

func postgresDatabaseConfig(database string) *dbmigrate.DatabaseConfig {
	cfg := newDatabaseConfig(config.POSTGRES_HOST, config.POSTGRES_PORT, config.POSTGRES_USERNAME, config.POSTGRES_PASSWORD, database)
	cfg.SSLMode = "disable"
	return cfg
}

func createPostgresDatabase(t *testing.T, cfg *dbmigrate.DatabaseConfig, database string) {
	t.Helper()

	db, err := sql.Open("postgres", postgresDSN(cfg))
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec("CREATE DATABASE " + database)
	require.NoError(t, err)
}

func dropPostgresDatabase(t *testing.T, cfg *dbmigrate.DatabaseConfig, database string) {
	t.Helper()

	db, err := sql.Open("postgres", postgresDSN(cfg))
	require.NoError(t, err)
	defer db.Close()

	_, _ = db.Exec("DROP DATABASE IF EXISTS " + database)
}

// execPostgres runs one statement on the configured PostgreSQL database. The
// driver is registered by the sqldef postgres package that dbmigrate itself
// imports.
func execPostgres(t *testing.T, cfg *dbmigrate.DatabaseConfig, statement string) {
	t.Helper()

	db, err := sql.Open("postgres", postgresDSN(cfg))
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(statement)
	require.NoError(t, err)
}

func postgresDSN(cfg *dbmigrate.DatabaseConfig) string {
	dsn := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.Username, cfg.Password),
		Host:   fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Path:   "/" + cfg.Database,
	}

	options := url.Values{}
	if cfg.SSLMode != "" {
		options.Set("sslmode", cfg.SSLMode)
	}
	dsn.RawQuery = options.Encode()

	return dsn.String()
}

// TestMigrateHandlesUniqueIndexesOverTheLiveRows pins, on each dialect, what
// a migration makes of a unique index over the live rows alone (see
// modelschema.IndexPlan.LiveOnly), spelled as the index renderer writes it.
// A database carrying the index over every row under the same name is
// planned a drop and a create, the plan gg migrate asks for, and once it is
// applied nothing is left to plan. And the index is renamed the way an index
// over every row is, alone and once its table is renamed: by one rename
// statement where the dialect renames indexes, and on SQLite, which renames
// none, by a drop and a create.
func TestMigrateHandlesUniqueIndexesOverTheLiveRows(t *testing.T) {
	type dialect struct {
		typ       config.DBType
		dialector gorm.Dialector
		table     func(name string) string
		target    func(t *testing.T, name string) *dbmigrate.DatabaseConfig
	}
	dialects := []dialect{
		{
			typ:       config.DBMySQL,
			dialector: mysql.New(mysql.Config{}),
			table: func(name string) string {
				return "CREATE TABLE `" + name + "` (\n" +
					"  `id` char(36) NOT NULL,\n" +
					"  `code` varchar(64) NOT NULL,\n" +
					"  `deleted_at` datetime(3) NULL,\n" +
					"  PRIMARY KEY (`id`)\n" +
					") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;"
			},
			target: func(t *testing.T, name string) *dbmigrate.DatabaseConfig {
				t.Helper()
				createMySQLDatabase(t, mysqlDatabaseConfig(), name)
				t.Cleanup(func() { dropMySQLDatabase(t, mysqlDatabaseConfig(), name) })
				cfg := mysqlDatabaseConfig()
				cfg.Database = name
				return cfg
			},
		},
		{
			typ:       config.DBPostgres,
			dialector: postgres.New(postgres.Config{}),
			table: func(name string) string {
				return `CREATE TABLE "` + name + `" (` + "\n" +
					`  "id" char(36) NOT NULL,` + "\n" +
					`  "code" varchar(64) NOT NULL,` + "\n" +
					`  "deleted_at" timestamptz,` + "\n" +
					`  PRIMARY KEY ("id")` + "\n" +
					");"
			},
			target: func(t *testing.T, name string) *dbmigrate.DatabaseConfig {
				t.Helper()
				admin := postgresDatabaseConfig(os.Getenv(config.POSTGRES_DATABASE))
				createPostgresDatabase(t, admin, name)
				t.Cleanup(func() { dropPostgresDatabase(t, admin, name) })
				return postgresDatabaseConfig(name)
			},
		},
		{
			typ:       config.DBSqlite,
			dialector: sqlite.New(sqlite.Config{}),
			table: func(name string) string {
				return "CREATE TABLE `" + name + "` (\n" +
					"  `id` text NOT NULL,\n" +
					"  `code` text NOT NULL,\n" +
					"  `deleted_at` datetime,\n" +
					"  PRIMARY KEY (`id`)\n" +
					");"
			},
			target: func(t *testing.T, name string) *dbmigrate.DatabaseConfig {
				t.Helper()
				return &dbmigrate.DatabaseConfig{Database: filepath.Join(t.TempDir(), name+".db")}
			},
		},
	}
	for _, d := range dialects {
		t.Run(string(d.typ), func(t *testing.T) {
			stamp := time.Now().UnixNano()
			schemaOf := func(table, index string, liveOnly bool) []string {
				plan := modelschema.IndexPlan{Name: index, Table: table, Columns: []string{"code"}, Unique: true, LiveOnly: liveOnly}
				return []string{d.table(table), plan.CreateSQL(d.dialector)}
			}
			plan := func(cfg *dbmigrate.DatabaseConfig, desired []string) dbmigrate.Plan {
				t.Helper()
				planned, err := dbmigrate.Migrate(desired, d.typ, cfg, &dbmigrate.MigrateOptions{DryRun: true})
				require.NoError(t, err)
				return planned
			}
			create := func(name string, current []string) *dbmigrate.DatabaseConfig {
				t.Helper()
				cfg := d.target(t, fmt.Sprintf("gst_dbmigrate_live_%s_%d", name, stamp))
				_, err := dbmigrate.Migrate(current, d.typ, cfg, &dbmigrate.MigrateOptions{})
				require.NoError(t, err)
				return cfg
			}

			// The index over every row, replaced under its own name.
			cfg := create("swap", schemaOf("items", "uniq_items_code", false))
			desired := schemaOf("items", "uniq_items_code", true)
			swap := plan(cfg, desired)
			require.Len(t, swap.Statements, 2, "%v", swap.Statements)
			require.Contains(t, swap.Statements[0], "DROP INDEX")
			require.Contains(t, swap.Statements[1], "CREATE UNIQUE INDEX")
			require.Empty(t, swap.Renames, "one name, two definitions: a rebuild, not a rename")
			require.NoError(t, dbmigrate.Apply(swap, d.typ, cfg, nil))
			require.False(t, plan(cfg, desired).Changed())

			// requireIndexRenamed holds a plan to the rename of one index and
			// nothing else: one statement renaming it, or on SQLite its drop
			// and its create.
			requireIndexRenamed := func(planned dbmigrate.Plan, from, to string) {
				t.Helper()
				require.Empty(t, planned.Renames)
				require.Empty(t, planned.DroppedColumns)
				if d.typ == config.DBSqlite {
					require.Len(t, planned.Statements, 2, "%v", planned.Statements)
					statements := strings.Join(planned.Statements, "\n")
					require.Contains(t, statements, "DROP INDEX")
					require.Contains(t, statements, "CREATE UNIQUE INDEX")
					return
				}
				require.Len(t, planned.Statements, 1, "%v", planned.Statements)
				require.Contains(t, planned.Statements[0], "RENAME")
				require.Contains(t, planned.Statements[0], from)
				require.Contains(t, planned.Statements[0], to)
			}

			// The index renamed alone.
			cfg = create("index", schemaOf("items", "legacy_items_code", true))
			desired = schemaOf("items", "uniq_items_code", true)
			renamed := plan(cfg, desired)
			requireIndexRenamed(renamed, "legacy_items_code", "uniq_items_code")
			require.NoError(t, dbmigrate.Apply(renamed, d.typ, cfg, nil))
			require.False(t, plan(cfg, desired).Changed())

			// The index renamed along with its table: the table first, as the
			// rename the plan reports, then the index in the plan made of the
			// renamed table.
			cfg = create("table", schemaOf("old_items", "uniq_old_items_code", true))
			desired = schemaOf("new_items", "uniq_new_items_code", true)
			renamed = plan(cfg, desired)
			require.Len(t, renamed.Renames, 1)
			require.Equal(t, "table old_items to new_items", renamed.Renames[0].String())
			require.NoError(t, dbmigrate.ApplyRenames(renamed.Renames, d.typ, cfg))
			renamed = plan(cfg, desired)
			requireIndexRenamed(renamed, "uniq_old_items_code", "uniq_new_items_code")
			require.NoError(t, dbmigrate.Apply(renamed, d.typ, cfg, nil))
			require.False(t, plan(cfg, desired).Changed())
		})
	}
}

// TestMigrateRefusesASQLiteColumnChangeItCannotPlan pins that a change to a
// column's definition on a sqlite database file is refused rather than
// planned as nothing: SQLite's ALTER TABLE cannot change a column's type,
// nullability, default or check, and the migration must say so, naming the
// table and the columns, instead of answering "no changes".
func TestMigrateRefusesASQLiteColumnChangeItCannotPlan(t *testing.T) {
	t.Run("hand-written schema", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "drift.db")
		databaseConfig := &dbmigrate.DatabaseConfig{Database: file}

		before := "CREATE TABLE `samples` (\n" +
			"  `id` text NOT NULL,\n" +
			"  `amount` integer NOT NULL,\n" +
			"  `note` text,\n" +
			"  PRIMARY KEY (`id`)\n" +
			");"
		after := "CREATE TABLE `samples` (\n" +
			"  `id` text NOT NULL,\n" +
			"  `amount` text NOT NULL,\n" +
			"  `note` text NOT NULL,\n" +
			"  `extra` text,\n" +
			"  PRIMARY KEY (`id`)\n" +
			");"

		plan, err := dbmigrate.Migrate([]string{before}, config.DBSqlite, databaseConfig, &dbmigrate.MigrateOptions{})
		require.NoError(t, err)
		require.True(t, plan.Changed())

		// A changed type and a changed nullability beside an added column:
		// the addition could be planned, the other two cannot, and the
		// migration refuses the whole rather than apply the half it can
		// express.
		_, err = dbmigrate.Migrate([]string{after}, config.DBSqlite, databaseConfig, &dbmigrate.MigrateOptions{DryRun: true})
		require.Error(t, err)
		require.ErrorContains(t, err, "samples")
		require.ErrorContains(t, err, "amount")
		require.ErrorContains(t, err, "note")
		require.ErrorContains(t, err, "rebuild")

		// The same schema again plans nothing and refuses nothing.
		plan, err = dbmigrate.Migrate([]string{before}, config.DBSqlite, databaseConfig, &dbmigrate.MigrateOptions{DryRun: true})
		require.NoError(t, err)
		require.False(t, plan.Changed())
	})

	t.Run("schema the models dump", func(t *testing.T) {
		dumper, err := dbmigrate.NewSchemaDumper()
		require.NoError(t, err)
		before, err := dumper.Dump(config.DBSqlite, SampleWithIntegerAmount{})
		require.NoError(t, err)
		after, err := dumper.Dump(config.DBSqlite, SampleWithTextAmount{})
		require.NoError(t, err)

		file := filepath.Join(t.TempDir(), "drift.db")
		databaseConfig := &dbmigrate.DatabaseConfig{Database: file}
		plan, err := dbmigrate.Migrate([]string{before}, config.DBSqlite, databaseConfig, &dbmigrate.MigrateOptions{})
		require.NoError(t, err)
		require.True(t, plan.Changed())

		// The model's amount column changed from integer to text.
		_, err = dbmigrate.Migrate([]string{after}, config.DBSqlite, databaseConfig, &dbmigrate.MigrateOptions{DryRun: true})
		require.Error(t, err)
		require.ErrorContains(t, err, "amount_samples")
		require.ErrorContains(t, err, "amount")
		require.ErrorContains(t, err, "rebuild")

		plan, err = dbmigrate.Migrate([]string{before}, config.DBSqlite, databaseConfig, &dbmigrate.MigrateOptions{DryRun: true})
		require.NoError(t, err)
		require.False(t, plan.Changed())
	})
}

// SampleWithIntegerAmount and SampleWithTextAmount are two revisions of one
// model: the same table, whose amount column changes type between them.
type SampleWithIntegerAmount struct {
	Amount int64 `json:"amount"`

	model.Base
}

func (*SampleWithIntegerAmount) TableName() string { return "amount_samples" }

type SampleWithTextAmount struct {
	Amount string `json:"amount" gorm:"size:191"`

	model.Base
}

func (*SampleWithTextAmount) TableName() string { return "amount_samples" }

// execSQLite runs one statement on the sqlite database file.
func execSQLite(t *testing.T, file, statement string) {
	t.Helper()
	db, err := sql.Open("sqlite3", file)
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(statement)
	require.NoError(t, err)
}
