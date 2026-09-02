package dbruntime

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/testutil/testcontainer"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	mysqlOnce    sync.Once
	releaseMySQL func() error
	errMySQL     error

	postgresOnce    sync.Once
	releasePostgres func() error
	errPostgres     error
)

// TestMain releases the mysql and postgres containers newMySQLDB and
// newPostgresDB prepare. Only the tests covering server-dialect index
// behavior ask for one, so both start lazily and this is the only place that
// knows whether there is anything to release.
func TestMain(m *testing.M) {
	code := m.Run()
	for _, release := range []func() error{releaseMySQL, releasePostgres} {
		if release != nil {
			if err := release(); err != nil {
				fmt.Fprintf(os.Stderr, "failed to release the test database: %v\n", err)
			}
		}
	}
	os.Exit(code)
}

// newSQLiteDB opens an isolated in-memory sqlite database. The connection
// pool is capped at one so every session sees the same in-memory schema.
func newSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// newMySQLDB opens a mysql container of its own. The package otherwise runs on
// sqlite, so the container is prepared once for the whole package rather than
// per test.
func newMySQLDB(t *testing.T) *gorm.DB {
	t.Helper()

	mysqlOnce.Do(func() {
		releaseMySQL, _, errMySQL = testcontainer.SetupDatabase(config.DBMySQL)
	})
	require.NoError(t, errMySQL)

	port, err := strconv.Atoi(os.Getenv(config.MYSQL_PORT))
	require.NoError(t, err)
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		os.Getenv(config.MYSQL_USERNAME), os.Getenv(config.MYSQL_PASSWORD),
		os.Getenv(config.MYSQL_HOST), port, os.Getenv(config.MYSQL_DATABASE))

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Discard, TranslateError: true})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Ping())
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// newPostgresDB opens a postgres container of its own, prepared once for the
// whole package like the mysql one.
func newPostgresDB(t *testing.T) *gorm.DB {
	t.Helper()

	postgresOnce.Do(func() {
		releasePostgres, _, errPostgres = testcontainer.SetupDatabase(config.DBPostgres)
	})
	require.NoError(t, errPostgres)

	port, err := strconv.Atoi(os.Getenv(config.POSTGRES_PORT))
	require.NoError(t, err)
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		os.Getenv(config.POSTGRES_HOST), port, os.Getenv(config.POSTGRES_USERNAME),
		os.Getenv(config.POSTGRES_PASSWORD), os.Getenv(config.POSTGRES_DATABASE))

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard, TranslateError: true})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Ping())
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}
