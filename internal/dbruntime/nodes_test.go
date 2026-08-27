package dbruntime

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAttachNodesRoundTrip(t *testing.T) {
	handle := new(gorm.DB)
	require.Nil(t, NodesFor(handle), "a plain handle carries no nodes")

	nodes := []DBNode{{Role: RolePrimary}, {Role: RoleReplica}}
	AttachNodes(handle, nodes)
	require.Equal(t, nodes, NodesFor(handle))

	AttachNodes(handle, nil)
	require.Nil(t, NodesFor(handle), "nil detaches")
}

func TestReplicaPoolMetricNames(t *testing.T) {
	names := ReplicaPoolMetricNames("default", []DBNode{
		{Role: RolePrimary},
		{Role: RoleReplica},
		{Role: RoleReplica},
	})
	require.Equal(t, []string{"default", "default_replica_0", "default_replica_1"}, names)
}
