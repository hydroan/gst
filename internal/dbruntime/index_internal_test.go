package dbruntime

import (
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
)

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

// conflictKindRecord and conflictKindTwin declare an index over the same
// column sequence of one table; ensuring the second model must fail instead
// of silently skipping the already-existing index.
type conflictKindRecord struct {
	Kind string

	modelregistry.Base
}

func (*conflictKindRecord) TableName() string { return "conflict_kind_records" }

func (*conflictKindRecord) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Kind", "CreatedAt"}}}
}

type conflictKindTwin struct {
	Kind string

	modelregistry.Base
}

func (*conflictKindTwin) TableName() string { return "conflict_kind_records" }

func (*conflictKindTwin) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Kind", "CreatedAt"}}}
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

func TestEnsureCustomIndexesRejectsCrossModelConflict(t *testing.T) {
	db := newSQLiteDB(t)
	first := &conflictKindRecord{}
	require.NoError(t, db.Table(first.TableName()).AutoMigrate(first))
	require.NoError(t, ensureCustomIndexes(db, first))
	// Re-ensuring the same model is no conflict: its plans replace the
	// recorded ones instead of colliding with them.
	require.NoError(t, ensureCustomIndexes(db, first))

	err := ensureCustomIndexes(db, &conflictKindTwin{})
	require.ErrorContains(t, err, `conflict on table "conflict_kind_records"`)
	require.ErrorContains(t, err, "conflictKindRecord")
	require.ErrorContains(t, err, "conflictKindTwin")
}
