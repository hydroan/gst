package dbruntime

import (
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// mysqlTestDSN points at the database the module test suites share.
const mysqlTestDSN = "test_module:test_module@tcp(127.0.0.1:3306)/test_module?charset=utf8mb4&parseTime=True&loc=Local"

// indexedRecord declares custom indexes covering embedded Base columns.
type indexedRecord struct {
	Code string `gorm:"index:idx_indexed_records_code"`
	Kind string

	modelregistry.Base
}

func (*indexedRecord) GetTableName() string { return "indexed_records" }

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

func (*renamedRecord) GetTableName() string { return "renamed_records" }

func (*renamedRecord) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Kind", "CreatedAt"}}}
}

// occupiedRecord reproduces a plan name occupied by a different definition.
type occupiedRecord struct {
	Code string
	Kind string

	modelregistry.Base
}

func (*occupiedRecord) GetTableName() string { return "occupied_records" }

func (*occupiedRecord) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Kind", "CreatedAt"}}}
}

// unnamedRecord leaves GetTableName at the Base default, relying on gorm's
// naming strategy for its table. Custom indexes must resolve the same table
// name the model is actually migrated into. The column sizes keep the indexed
// columns within the key length MySQL allows.
type unnamedRecord struct {
	Code string `gorm:"size:64"`
	Kind string `gorm:"size:64"`

	modelregistry.Base
}

func (*unnamedRecord) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Code", "Kind"}, Unique: true}}
}

// invalidRecord declares an index on a field that does not exist.
type invalidRecord struct {
	Name string

	modelregistry.Base
}

func (*invalidRecord) GetTableName() string { return "invalid_records" }

func (*invalidRecord) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Missing"}}}
}

func TestEnsureCustomIndexes(t *testing.T) {
	db := newSQLiteDB(t)
	m := &indexedRecord{}
	require.NoError(t, db.Table(m.GetTableName()).AutoMigrate(m))

	require.NoError(t, ensureCustomIndexes(db, m))
	// A second run must be idempotent.
	require.NoError(t, ensureCustomIndexes(db, m))

	indexes, err := db.Migrator().GetIndexes(m.GetTableName())
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
	require.NoError(t, db.Table(m.GetTableName()).AutoMigrate(m))
	require.NoError(t, db.Exec("CREATE INDEX legacy_records_kind ON renamed_records(kind, created_at)").Error)

	err := ensureCustomIndexes(db, m)
	require.ErrorContains(t, err, `already exists as "legacy_records_kind"`)
	require.ErrorContains(t, err, "RENAME INDEX legacy_records_kind TO idx_renamed_records_kind_created_at")
}

func TestEnsureCustomIndexesRejectsOccupiedName(t *testing.T) {
	db := newSQLiteDB(t)
	m := &occupiedRecord{}
	require.NoError(t, db.Table(m.GetTableName()).AutoMigrate(m))
	require.NoError(t, db.Exec("CREATE INDEX idx_occupied_records_kind_created_at ON occupied_records(code)").Error)

	err := ensureCustomIndexes(db, m)
	require.ErrorContains(t, err, "exists with a different definition")
}

func TestEnsureCustomIndexesWithoutDeclaredTableName(t *testing.T) {
	m := &unnamedRecord{}

	t.Run("creates and stays idempotent", func(t *testing.T) {
		db := newSQLiteDB(t)
		require.NoError(t, db.AutoMigrate(m))

		require.NoError(t, ensureCustomIndexes(db, m))
		// A second run must be idempotent.
		require.NoError(t, ensureCustomIndexes(db, m))

		indexes, err := db.Migrator().GetIndexes("unnamed_records")
		require.NoError(t, err)
		columnsByName := make(map[string][]string, len(indexes))
		for _, idx := range indexes {
			columnsByName[idx.Name()] = idx.Columns()
		}
		require.Equal(t, []string{"code", "kind"}, columnsByName["uniq_unnamed_records_code_kind"])
	})

	t.Run("rejects rename candidate", func(t *testing.T) {
		db := newSQLiteDB(t)
		require.NoError(t, db.AutoMigrate(m))
		require.NoError(t, db.Exec("CREATE UNIQUE INDEX legacy_unnamed_records_code_kind ON unnamed_records(code, kind)").Error)

		err := ensureCustomIndexes(db, m)
		require.ErrorContains(t, err, `already exists as "legacy_unnamed_records_code_kind"`)
	})
}

// TestEnsureCustomIndexesOnMySQL covers the same contract on MySQL, whose
// migrator reports index metadata through information_schema and rejects a
// duplicate name outright. It skips when no server is reachable.
func TestEnsureCustomIndexesOnMySQL(t *testing.T) {
	db := newMySQLDB(t)
	m := &unnamedRecord{}

	require.NoError(t, db.Migrator().DropTable(m))
	t.Cleanup(func() { _ = db.Migrator().DropTable(m) })
	require.NoError(t, db.AutoMigrate(m))

	require.NoError(t, ensureCustomIndexes(db, m))
	// A second run must be idempotent.
	require.NoError(t, ensureCustomIndexes(db, m))

	indexes, err := db.Migrator().GetIndexes("unnamed_records")
	require.NoError(t, err)
	columnsByName := make(map[string][]string, len(indexes))
	uniqueByName := make(map[string]bool, len(indexes))
	for _, idx := range indexes {
		columnsByName[idx.Name()] = idx.Columns()
		if unique, ok := idx.Unique(); ok {
			uniqueByName[idx.Name()] = unique
		}
	}
	require.Equal(t, []string{"code", "kind"}, columnsByName["uniq_unnamed_records_code_kind"])
	require.True(t, uniqueByName["uniq_unnamed_records_code_kind"])

	// The same definition living under a foreign name must be reported as a
	// rename candidate instead of being created a second time.
	require.NoError(t, db.Migrator().DropIndex(m, "uniq_unnamed_records_code_kind"))
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX legacy_unnamed_records_code_kind ON unnamed_records(code, kind)").Error)

	err = ensureCustomIndexes(db, m)
	require.ErrorContains(t, err, `already exists as "legacy_unnamed_records_code_kind"`)
}

func TestEnsureCustomIndexesRejectsInvalidDeclaration(t *testing.T) {
	db := newSQLiteDB(t)
	m := &invalidRecord{}
	require.NoError(t, db.Table(m.GetTableName()).AutoMigrate(m))

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

// newMySQLDB opens the shared module test database. The package otherwise
// runs on sqlite alone, so an unreachable server skips the test instead of
// failing it.
func newMySQLDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(mysql.Open(mysqlTestDSN), &gorm.Config{Logger: logger.Discard, TranslateError: true})
	if err != nil {
		t.Skipf("mysql is unavailable: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Skipf("mysql is unavailable: %v", err)
	}
	if err = sqlDB.Ping(); err != nil {
		t.Skipf("mysql is unavailable: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}
