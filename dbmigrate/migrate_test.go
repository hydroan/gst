package dbmigrate_test

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/dbmigrate"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

// runTests prepares both databases the migration tests target. They need two at
// once and no server at all, which is what SetupDatabase is for; os.Exit in
// TestMain would skip the deferred releases, hence the wrapper.
func runTests(m *testing.M) int {
	releaseMySQL, err := testutil.SetupDatabase(config.DBMySQL)
	if err != nil {
		panic(err)
	}
	defer func() { _ = releaseMySQL() }()

	releasePostgres, err := testutil.SetupDatabase(config.DBPostgres)
	if err != nil {
		panic(err)
	}
	defer func() { _ = releasePostgres() }()

	return m.Run()
}

// databaseConfig reads back the connection the test container was prepared on.
func databaseConfig(hostKey, portKey, userKey, passwordKey, database string) *dbmigrate.DatabaseConfig {
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
	return databaseConfig(config.MYSQL_HOST, config.MYSQL_PORT, config.MYSQL_USERNAME, config.MYSQL_PASSWORD,
		os.Getenv(config.MYSQL_DATABASE))
}

func TestMigrate(t *testing.T) {
	t.Run("mysql", func(t *testing.T) {
		dumper, err := dbmigrate.NewSchemaDumper()
		require.NoError(t, err)
		schema, err := dumper.Dump(config.DBMySQL, User{}, Group{})
		require.NoError(t, err)

		migrated, _, err := dbmigrate.Migrate([]string{schema}, config.DBMySQL,
			mysqlDatabaseConfig(),
			&dbmigrate.MigrateOption{
				DryRun: true,
			})
		require.NoError(t, err)
		require.True(t, migrated)
	})

	t.Run("postgres", func(t *testing.T) {
		dumper, err := dbmigrate.NewSchemaDumper()
		require.NoError(t, err)
		schema, err := dumper.Dump(config.DBPostgres, User{}, Group{})
		require.NoError(t, err)

		database := fmt.Sprintf("gst_dbmigrate_test_%d", time.Now().UnixNano())
		adminConfig := postgresDatabaseConfig("test")
		createPostgresDatabase(t, adminConfig, database)
		t.Cleanup(func() {
			dropPostgresDatabase(t, adminConfig, database)
		})
		databaseConfig := postgresDatabaseConfig(database)

		migrated, _, err := dbmigrate.Migrate(
			[]string{schema}, config.DBPostgres,
			databaseConfig,
			&dbmigrate.MigrateOption{
				DryRun: true,
			},
		)
		require.NoError(t, err)
		require.True(t, migrated)

		migrated, _, err = dbmigrate.Migrate(
			[]string{schema}, config.DBPostgres,
			databaseConfig,
			&dbmigrate.MigrateOption{},
		)
		require.NoError(t, err)
		require.True(t, migrated)

		migrated, _, err = dbmigrate.Migrate(
			[]string{schema}, config.DBPostgres,
			databaseConfig,
			&dbmigrate.MigrateOption{
				DryRun: true,
			},
		)
		require.NoError(t, err)
		require.False(t, migrated)
	})

	t.Run("sqlite", func(t *testing.T) {
		dumper, err := dbmigrate.NewSchemaDumper()
		require.NoError(t, err)
		schema, err := dumper.Dump(config.DBSqlite, User{}, Group{})
		require.NoError(t, err)

		database := filepath.Join(t.TempDir(), "test.db")
		migrated, _, err := dbmigrate.Migrate([]string{schema}, config.DBSqlite,
			&dbmigrate.DatabaseConfig{
				Database: database,
			},
			&dbmigrate.MigrateOption{
				DryRun: true,
			})
		require.NoError(t, err)
		require.True(t, migrated)

		migrated, _, err = dbmigrate.Migrate([]string{schema}, config.DBSqlite,
			&dbmigrate.DatabaseConfig{
				Database: database,
			},
			&dbmigrate.MigrateOption{})
		require.NoError(t, err)
		require.True(t, migrated)

		migrated, _, err = dbmigrate.Migrate([]string{schema}, config.DBSqlite,
			&dbmigrate.DatabaseConfig{
				Database: database,
			},
			&dbmigrate.MigrateOption{
				DryRun: true,
			})
		require.NoError(t, err)
		require.False(t, migrated)
	})
}

func postgresDatabaseConfig(database string) *dbmigrate.DatabaseConfig {
	cfg := databaseConfig(config.POSTGRES_HOST, config.POSTGRES_PORT, config.POSTGRES_USERNAME, config.POSTGRES_PASSWORD, database)
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
