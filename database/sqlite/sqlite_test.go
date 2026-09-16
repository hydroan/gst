package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// TestMemoryDatabaseOutlivesThePoolConnection proves the in-memory database
// lives as long as the process, whatever becomes of the one connection the
// pool holds: a table and its row are still there after the pool retired the
// connection for age and idle time, and after the pool discarded the
// connection a transaction held when its context ended.
func TestMemoryDatabaseOutlivesThePoolConnection(t *testing.T) {
	t.Run("retired for age and idle time", func(t *testing.T) {
		withPoolLifetime(t, time.Millisecond)
		db, pool := openMemoryDatabase(t)
		createSampleTable(t, db, "retired_samples")

		require.Eventually(t, func() bool {
			stats := pool.Stats()
			return stats.MaxLifetimeClosed+stats.MaxIdleTimeClosed > 0
		}, 5*time.Second, 10*time.Millisecond, "the pool must have retired its connection")
		requireSampleRows(t, db, "retired_samples", 1)
	})

	t.Run("discarded after a transaction whose context ended", func(t *testing.T) {
		withPoolLifetime(t, 0)
		db, pool := openMemoryDatabase(t)
		createSampleTable(t, db, "discarded_samples")

		ctx, cancel := context.WithCancel(context.Background())
		tx := db.WithContext(ctx).Begin()
		require.NoError(t, tx.Error)
		require.NoError(t, tx.Exec("INSERT INTO discarded_samples (id) VALUES (2)").Error)
		cancel()

		// database/sql rolls back a transaction whose context ends and closes
		// its connection, since this driver cannot reset a session.
		require.Eventually(t, func() bool {
			return pool.Stats().OpenConnections == 0
		}, 5*time.Second, 10*time.Millisecond, "the pool must have discarded the connection")
		requireSampleRows(t, db, "discarded_samples", 1)
	})
}

// withPoolLifetime sets the [database] lifetime and idle-time limits the pool
// runs under to d, silences the SQL log, and restores both afterwards.
func withPoolLifetime(t *testing.T, d time.Duration) {
	t.Helper()

	originalConfig, originalGorm := config.App, logger.Gorm
	config.App = new(config.Config)
	config.App.Database.ConnMaxLifetime = d
	config.App.Database.ConnMaxIdleTime = d
	logger.Gorm = gormlogger.Discard
	t.Cleanup(func() { config.App, logger.Gorm = originalConfig, originalGorm })
}

// openMemoryDatabase opens a handle on the in-memory database and closes its
// pool once the test ends.
func openMemoryDatabase(t *testing.T) (*gorm.DB, *sql.DB) {
	t.Helper()

	db, err := New(config.Sqlite{IsMemory: true})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	return db, pool
}

// createSampleTable creates table holding one row, and drops it once the test
// ends: the in-memory database outlives the handle the test opened.
func createSampleTable(t *testing.T, db *gorm.DB, table string) {
	t.Helper()

	require.NoError(t, db.Exec("CREATE TABLE "+table+" (id INTEGER PRIMARY KEY)").Error)
	t.Cleanup(func() { require.NoError(t, db.Exec("DROP TABLE IF EXISTS "+table).Error) })
	require.NoError(t, db.Exec("INSERT INTO "+table+" (id) VALUES (1)").Error)
}

// requireSampleRows asserts table holds want rows.
func requireSampleRows(t *testing.T, db *gorm.DB, table string, want int64) {
	t.Helper()

	var count int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM "+table).Scan(&count).Error)
	require.Equal(t, want, count)
}
