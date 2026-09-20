package sqlite

import (
	"context"
	"database/sql"
	"strings"
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

// TestMemoryDatabaseIsSharedAcrossHandles proves a process has one in-memory
// database: a second handle reads what the first wrote.
func TestMemoryDatabaseIsSharedAcrossHandles(t *testing.T) {
	withPoolLifetime(t, 0)
	first, _ := openMemoryDatabase(t)
	createSampleTable(t, first, "shared_samples")

	second, _ := openMemoryDatabase(t)
	requireSampleRows(t, second, "shared_samples", 1)
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

// TestBuildDSNKeepsWhatThePathAsked pins the two ways a configured path used
// to be read wrong: a path naming memory opened as a file, which lives as
// long as one connection and vanishes with it, and a path carrying
// parameters of its own gaining a second question mark, which sqlite reads as
// part of a value — the tuning after it is silently lost.
func TestBuildDSNKeepsWhatThePathAsked(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  config.Sqlite
		want string
	}{
		{
			name: "MemoryPathWithoutTheFlag",
			cfg:  config.Sqlite{Path: ":memory:"},
			want: memoryDSN,
		},
		{
			name: "MemoryFileURIWithoutTheFlag",
			cfg:  config.Sqlite{Path: "file::memory:?cache=shared"},
			want: memoryDSN,
		},
		{
			name: "FlagWithoutThePath",
			cfg:  config.Sqlite{IsMemory: true, Path: "/tmp/ignored.db"},
			want: memoryDSN,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, buildDSN(tc.cfg))
		})
	}

	t.Run("PathWithParametersKeepsOneQuestionMark", func(t *testing.T) {
		dsn := buildDSN(config.Sqlite{Path: "/tmp/app.db?_txlock=immediate"})
		require.Equal(t, 1, strings.Count(dsn, "?"), "a second question mark makes the parameters after it part of a value")
		require.Contains(t, dsn, "_txlock=immediate", "what the path asked for survives")
		require.Contains(t, dsn, "_journal_mode=WAL", "and the framework's own tuning is appended to it")
	})

	t.Run("PlainPathTakesTheParameters", func(t *testing.T) {
		dsn := buildDSN(config.Sqlite{Path: "/tmp/app.db"})
		require.True(t, strings.HasPrefix(dsn, "/tmp/app.db?"))
		require.Contains(t, dsn, "_busy_timeout=5000")
	})
}
