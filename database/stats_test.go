package database_test

import (
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/stretchr/testify/require"
)

func TestStats(t *testing.T) {
	// The default database is initialized by TestMain without replicas, so
	// Stats reports its single primary node with a live pool snapshot.
	nodes := database.Stats()
	require.Len(t, nodes, 1, "a replica-free database is a single node")
	require.Equal(t, database.RolePrimary, nodes[0].Role)
	require.GreaterOrEqual(t, nodes[0].DBStats.OpenConnections, 0)
}

func TestStatsReportsEveryNode(t *testing.T) {
	// With nodes attached to the handle, Stats reports one snapshot per node
	// in registration order.
	sqlDB, err := database.DB().DB()
	require.NoError(t, err)
	dbruntime.AttachNodes(database.DB(), []dbruntime.DBNode{
		{Role: database.RolePrimary, DB: sqlDB},
		{Role: database.RoleReplica, DB: sqlDB},
	})
	t.Cleanup(func() { dbruntime.AttachNodes(database.DB(), nil) })

	nodes := database.Stats()
	require.Len(t, nodes, 2)
	require.Equal(t, database.RolePrimary, nodes[0].Role)
	require.Equal(t, database.RoleReplica, nodes[1].Role)
}
