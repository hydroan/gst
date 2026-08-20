package dbruntime

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/testcontainer"
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

// indexedRecord declares custom indexes covering embedded Base columns.
type indexedRecord struct {
	Code string `gorm:"index:idx_indexed_records_code"`
	Kind string

	modelregistry.Base
}

func (*indexedRecord) TableName() string { return "indexed_records" }

func (*indexedRecord) Indexes() []modelregistry.Index {
	return []modelregistry.Index{
		{Fields: []string{"Kind", "CreatedAt"}},
		{Fields: []string{"Code", "Kind"}, Unique: true},
	}
}

// renamedRecord reproduces a same-definition index living under a foreign name.
type renamedRecord struct {
	Kind string

	modelregistry.Base
}

func (*renamedRecord) TableName() string { return "renamed_records" }

func (*renamedRecord) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Kind", "CreatedAt"}}}
}

// occupiedRecord reproduces a plan name occupied by a different definition.
type occupiedRecord struct {
	Code string
	Kind string

	modelregistry.Base
}

func (*occupiedRecord) TableName() string { return "occupied_records" }

func (*occupiedRecord) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Kind", "CreatedAt"}}}
}

// unnamedRecord leaves TableName at the Base default; ensuring its indexes
// must be rejected, because no model may leave its table name undeclared.
type unnamedRecord struct {
	Code string `gorm:"size:64"`
	Kind string `gorm:"size:64"`

	modelregistry.Base
}

func (*unnamedRecord) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Code", "Kind"}, Unique: true}}
}

// sizedRecord declares its table name and caps the indexed column sizes so
// the unique key stays within the key length MySQL allows.
type sizedRecord struct {
	Code string `gorm:"size:64"`
	Kind string `gorm:"size:64"`

	modelregistry.Base
}

func (*sizedRecord) TableName() string { return "sized_records" }

func (*sizedRecord) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Code", "Kind"}, Unique: true}}
}

// invalidRecord declares an index on a field that does not exist.
type invalidRecord struct {
	Name string

	modelregistry.Base
}

func (*invalidRecord) TableName() string { return "invalid_records" }

func (*invalidRecord) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Missing"}}}
}

func TestEnsureCustomIndexes(t *testing.T) {
	db := newSQLiteDB(t)
	m := &indexedRecord{}
	require.NoError(t, db.Table(m.TableName()).AutoMigrate(m))

	require.NoError(t, ensureCustomIndexes(db, m))
	// A second run must be idempotent.
	require.NoError(t, ensureCustomIndexes(db, m))

	indexes, err := db.Migrator().GetIndexes(m.TableName())
	require.NoError(t, err)
	columnsByName := make(map[string][]string, len(indexes))
	uniqueByName := make(map[string]bool, len(indexes))
	for _, idx := range indexes {
		columnsByName[idx.Name()] = idx.Columns()
		if unique, ok := idx.Unique(); ok {
			uniqueByName[idx.Name()] = unique
		}
	}
	require.Equal(t, []string{"kind", "created_at"}, columnsByName["idx_indexed_records_kind_created_at"])
	require.Equal(t, []string{"code", "kind"}, columnsByName["uniq_indexed_records_code_kind"])
	require.True(t, uniqueByName["uniq_indexed_records_code_kind"])
}

func TestEnsureCustomIndexesRejectsRenameCandidate(t *testing.T) {
	db := newSQLiteDB(t)
	m := &renamedRecord{}
	require.NoError(t, db.Table(m.TableName()).AutoMigrate(m))
	require.NoError(t, db.Exec("CREATE INDEX legacy_records_kind ON renamed_records(kind, created_at)").Error)

	err := ensureCustomIndexes(db, m)
	require.ErrorContains(t, err, `already exists as "legacy_records_kind"`)
	require.ErrorContains(t, err, "RENAME INDEX legacy_records_kind TO idx_renamed_records_kind_created_at")
}

func TestEnsureCustomIndexesRejectsOccupiedName(t *testing.T) {
	db := newSQLiteDB(t)
	m := &occupiedRecord{}
	require.NoError(t, db.Table(m.TableName()).AutoMigrate(m))
	require.NoError(t, db.Exec("CREATE INDEX idx_occupied_records_kind_created_at ON occupied_records(code)").Error)

	err := ensureCustomIndexes(db, m)
	require.ErrorContains(t, err, "exists with a different definition")
}

func TestEnsureCustomIndexesRejectsUndeclaredTableName(t *testing.T) {
	db := newSQLiteDB(t)

	err := ensureCustomIndexes(db, &unnamedRecord{})
	require.ErrorContains(t, err, "must declare an explicit table name")
}

// TestEnsureCustomIndexesOnMySQL covers the same contract on MySQL, whose
// migrator reports index metadata through information_schema and rejects a
// duplicate name outright. It skips when no server is reachable.
func TestEnsureCustomIndexesOnMySQL(t *testing.T) {
	db := newMySQLDB(t)
	m := &sizedRecord{}

	require.NoError(t, db.Migrator().DropTable(m))
	t.Cleanup(func() { _ = db.Migrator().DropTable(m) })
	require.NoError(t, db.AutoMigrate(m))

	require.NoError(t, ensureCustomIndexes(db, m))
	// A second run must be idempotent.
	require.NoError(t, ensureCustomIndexes(db, m))

	indexes, err := db.Migrator().GetIndexes("sized_records")
	require.NoError(t, err)
	columnsByName := make(map[string][]string, len(indexes))
	uniqueByName := make(map[string]bool, len(indexes))
	for _, idx := range indexes {
		columnsByName[idx.Name()] = idx.Columns()
		if unique, ok := idx.Unique(); ok {
			uniqueByName[idx.Name()] = unique
		}
	}
	require.Equal(t, []string{"code", "kind"}, columnsByName["uniq_sized_records_code_kind"])
	require.True(t, uniqueByName["uniq_sized_records_code_kind"])

	// The same definition living under a foreign name must be reported as a
	// rename candidate instead of being created a second time.
	require.NoError(t, db.Migrator().DropIndex(m, "uniq_sized_records_code_kind"))
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX legacy_sized_records_code_kind ON sized_records(code, kind)").Error)

	err = ensureCustomIndexes(db, m)
	require.ErrorContains(t, err, `already exists as "legacy_sized_records_code_kind"`)
}

// TestEnsureCustomIndexesOnPostgres covers the same contract on PostgreSQL,
// whose migrator reads index metadata from the system catalogs.
func TestEnsureCustomIndexesOnPostgres(t *testing.T) {
	db := newPostgresDB(t)
	m := &sizedRecord{}

	require.NoError(t, db.Migrator().DropTable(m))
	t.Cleanup(func() { _ = db.Migrator().DropTable(m) })
	require.NoError(t, db.AutoMigrate(m))

	require.NoError(t, ensureCustomIndexes(db, m))
	// A second run must be idempotent.
	require.NoError(t, ensureCustomIndexes(db, m))

	indexes, err := db.Migrator().GetIndexes("sized_records")
	require.NoError(t, err)
	columnsByName := make(map[string][]string, len(indexes))
	uniqueByName := make(map[string]bool, len(indexes))
	for _, idx := range indexes {
		columnsByName[idx.Name()] = idx.Columns()
		if unique, ok := idx.Unique(); ok {
			uniqueByName[idx.Name()] = unique
		}
	}
	require.Equal(t, []string{"code", "kind"}, columnsByName["uniq_sized_records_code_kind"])
	require.True(t, uniqueByName["uniq_sized_records_code_kind"])

	// The same definition living under a foreign name must be reported as a
	// rename candidate instead of being created a second time. The index is
	// dropped through raw SQL because the gorm postgres migrator's DropIndex
	// renders CURRENT_SCHEMA() into a position DROP INDEX does not accept.
	require.NoError(t, db.Exec("DROP INDEX uniq_sized_records_code_kind").Error)
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX legacy_sized_records_code_kind ON sized_records(code, kind)").Error)

	err = ensureCustomIndexes(db, m)
	require.ErrorContains(t, err, `already exists as "legacy_sized_records_code_kind"`)
}

func TestEnsureCustomIndexesRejectsInvalidDeclaration(t *testing.T) {
	db := newSQLiteDB(t)
	m := &invalidRecord{}
	require.NoError(t, db.Table(m.TableName()).AutoMigrate(m))

	err := ensureCustomIndexes(db, m)
	require.ErrorContains(t, err, `unknown field "Missing"`)
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
		releaseMySQL, errMySQL = testcontainer.SetupDatabase(config.DBMySQL)
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
		releasePostgres, errPostgres = testcontainer.SetupDatabase(config.DBPostgres)
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
