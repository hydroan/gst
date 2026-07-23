package dbruntime

import (
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

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
