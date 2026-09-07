package dbruntime

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

// TestConfigurePoolAppliesTheConfiguredLimits pins that a pool handed to
// ConfigurePool runs under the [database] limits: the open-connection cap is
// what the pool reports, and the idle cap is what it enforces when
// connections come back. The lifetime and idle-time limits are set by the
// same call; database/sql exposes neither for inspection.
func TestConfigurePoolAppliesTheConfiguredLimits(t *testing.T) {
	original := config.App.Database
	config.App.Database.MaxIdleConns = 1
	config.App.Database.MaxOpenConns = 3
	config.App.Database.ConnMaxLifetime = time.Hour
	config.App.Database.ConnMaxIdleTime = time.Minute
	t.Cleanup(func() { config.App.Database = original })

	pool, err := newSQLiteDB(t).DB()
	require.NoError(t, err)
	ConfigurePool(pool)

	require.Equal(t, 3, pool.Stats().MaxOpenConnections)

	// Hold three connections at once and return them: one may stay idle,
	// the other two are closed on return.
	ctx := context.Background()
	conns := make([]*sql.Conn, 0, 3)
	for range 3 {
		conn, err := pool.Conn(ctx)
		require.NoError(t, err)
		conns = append(conns, conn)
	}
	for _, conn := range conns {
		require.NoError(t, conn.Close())
	}
	stats := pool.Stats()
	require.Equal(t, 1, stats.Idle, "one idle connection is kept")
	require.EqualValues(t, 2, stats.MaxIdleClosed, "the connections beyond the idle cap are closed on return")
}
