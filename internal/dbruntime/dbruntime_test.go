package dbruntime

import (
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
)

// plainRecord is a minimal model for table preparation tests.
type plainRecord struct {
	Name string

	modelregistry.Base
}

func (*plainRecord) GetTableName() string { return "plain_records" }

func TestEnsureTableCreatesTableWhenAutoMigrateEnabled(t *testing.T) {
	db := newSQLiteDB(t)
	withAutoMigrate(t, true)

	require.NoError(t, ensureTable(db, &plainRecord{}))
	require.True(t, db.Migrator().HasTable("plain_records"))
}

func TestEnsureTableFailsFastWhenDisabledAndTableMissing(t *testing.T) {
	db := newSQLiteDB(t)
	withAutoMigrate(t, false)

	err := ensureTable(db, &plainRecord{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "gg migrate")
	require.False(t, db.Migrator().HasTable("plain_records"))
}

func TestEnsureTablePassesWhenDisabledAndTableExists(t *testing.T) {
	db := newSQLiteDB(t)
	withAutoMigrate(t, true)
	require.NoError(t, ensureTable(db, &plainRecord{}))

	withAutoMigrate(t, false)
	require.NoError(t, ensureTable(db, &plainRecord{}))
}

// derivedRecord omits an explicit table name and relies on gorm's naming strategy.
type derivedRecord struct {
	Name string

	modelregistry.Base
}

func TestEnsureTableResolvesDerivedTableName(t *testing.T) {
	db := newSQLiteDB(t)
	withAutoMigrate(t, true)
	require.NoError(t, ensureTable(db, &derivedRecord{}))
	require.True(t, db.Migrator().HasTable("derived_records"))

	withAutoMigrate(t, false)
	require.NoError(t, ensureTable(db, &derivedRecord{}))
}

func TestEnsureTableReportsDerivedTableNameWhenMissing(t *testing.T) {
	db := newSQLiteDB(t)
	withAutoMigrate(t, false)

	err := ensureTable(db, &derivedRecord{})
	require.Error(t, err)
	require.Contains(t, err.Error(), `"derived_records"`)
}

// withAutoMigrate overrides the auto-migrate option and restores it on cleanup.
func withAutoMigrate(t *testing.T, enabled bool) {
	t.Helper()
	old := config.App.Database.AutoMigrate
	config.App.Database.AutoMigrate = enabled
	t.Cleanup(func() { config.App.Database.AutoMigrate = old })
}
