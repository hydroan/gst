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

func TestParseReplicaEndpoint(t *testing.T) {
	t.Run("host_and_port_are_split", func(t *testing.T) {
		host, port, err := ParseReplicaEndpoint("10.0.0.1:3306")
		require.NoError(t, err)
		require.Equal(t, "10.0.0.1", host)
		require.EqualValues(t, 3306, port)
	})

	t.Run("a_bracketed_address_keeps_its_host", func(t *testing.T) {
		// An IPv6 replica is written the way every other address is, so the
		// brackets belong to the notation rather than to the host.
		host, port, err := ParseReplicaEndpoint("[::1]:5432")
		require.NoError(t, err)
		require.Equal(t, "::1", host)
		require.EqualValues(t, 5432, port)
	})

	// A malformed entry fails initialization instead of being skipped, so each
	// of these has to report rather than return a zero endpoint: a replica that
	// silently never joined would read as "all reads on the primary".
	for name, endpoint := range map[string]string{
		"no_port":            "10.0.0.1",
		"empty_endpoint":     "",
		"non_numeric_port":   "10.0.0.1:primary",
		"port_above_the_max": "10.0.0.1:70000",
	} {
		t.Run("rejects_"+name, func(t *testing.T) {
			_, _, err := ParseReplicaEndpoint(endpoint)

			require.Error(t, err)
			// The message names the offending entry, which is what turns a
			// typo in a replica list into something findable.
			require.ErrorContains(t, err, "want host:port")
			require.ErrorContains(t, err, endpoint)
		})
	}
}

func TestReplicaPoolMetricNames(t *testing.T) {
	names := replicaPoolMetricNames("default", []DBNode{
		{Role: RolePrimary},
		{Role: RoleReplica},
		{Role: RoleReplica},
	})
	require.Equal(t, []string{"default", "default_replica_0", "default_replica_1"}, names)
}
