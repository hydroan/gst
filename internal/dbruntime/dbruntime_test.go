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

func (*plainRecord) TableName() string { return "plain_records" }

func TestEnsureTableCreatesTableWhenAutoMigrateEnabled(t *testing.T) {
	db := newSQLiteDB(t)
	withAutoMigrate(t, true)

	require.NoError(t, ensureTable(db, &plainRecord{}))
	require.True(t, db.Migrator().HasTable("plain_records"))
}

func TestEnsureTableFailsFastWhenDisabledAndTableMissing(t *testing.T) {
	db := newSQLiteDB(t)
	withAutoMigrate(t, false)
	withSqlite(t, false)

	err := ensureTable(db, &plainRecord{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "gg migrate")
	require.False(t, db.Migrator().HasTable("plain_records"))
}

func TestEnsureTableMigratesInMemorySqliteWhenDisabled(t *testing.T) {
	db := newSQLiteDB(t)
	withAutoMigrate(t, false)
	withSqlite(t, true)

	require.NoError(t, ensureTable(db, &plainRecord{}))
	require.True(t, db.Migrator().HasTable("plain_records"))
}

func TestEnsureTablePassesWhenDisabledAndTableExists(t *testing.T) {
	db := newSQLiteDB(t)
	withAutoMigrate(t, true)
	require.NoError(t, ensureTable(db, &plainRecord{}))

	withAutoMigrate(t, false)
	require.NoError(t, ensureTable(db, &plainRecord{}))
}

// undeclaredRecord omits an explicit table name; table preparation must
// reject it instead of falling back to gorm's naming strategy.
type undeclaredRecord struct {
	Name string

	modelregistry.Base
}

func TestEnsureTableRejectsUndeclaredTableName(t *testing.T) {
	db := newSQLiteDB(t)
	withAutoMigrate(t, true)

	err := ensureTable(db, &undeclaredRecord{})
	require.ErrorContains(t, err, "must declare an explicit table name")
	require.False(t, db.Migrator().HasTable("undeclared_records"))
}

// withAutoMigrate overrides the auto-migrate option and restores it on cleanup.
func withAutoMigrate(t *testing.T, enabled bool) {
	t.Helper()
	old := config.App.Database.AutoMigrate
	config.App.Database.AutoMigrate = enabled
	t.Cleanup(func() { config.App.Database.AutoMigrate = old })
}

// withSqlite selects sqlite as the database type and marks whether it is the
// in-memory variant, restoring both options on cleanup.
func withSqlite(t *testing.T, inMemory bool) {
	t.Helper()
	oldType, oldIsMemory := config.App.Database.Type, config.App.Sqlite.IsMemory
	config.App.Database.Type, config.App.Sqlite.IsMemory = config.DBSqlite, inMemory
	t.Cleanup(func() {
		config.App.Database.Type, config.App.Sqlite.IsMemory = oldType, oldIsMemory
	})
}
