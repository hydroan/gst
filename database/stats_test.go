package database_test

import (
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/stretchr/testify/require"
)

func TestStats(t *testing.T) {
	// The default database is initialized by TestMain, so Stats reports its
	// single primary node with a live pool snapshot.
	nodes := database.Stats()
	require.Len(t, nodes, 1, "the default database is a single node today")
	require.Equal(t, database.RolePrimary, nodes[0].Role)
	require.GreaterOrEqual(t, nodes[0].DBStats.OpenConnections, 0)
}
