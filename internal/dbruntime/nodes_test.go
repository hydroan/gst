package dbruntime_test

import (
	"testing"

	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAttachNodesRoundTrip(t *testing.T) {
	handle := new(gorm.DB)
	require.Nil(t, dbruntime.NodesFor(handle), "a plain handle carries no nodes")

	nodes := []dbruntime.DBNode{{Role: dbruntime.RolePrimary}, {Role: dbruntime.RoleReplica}}
	dbruntime.AttachNodes(handle, nodes)
	require.Equal(t, nodes, dbruntime.NodesFor(handle))

	dbruntime.AttachNodes(handle, nil)
	require.Nil(t, dbruntime.NodesFor(handle), "nil detaches")
}

func TestParseReplicaEndpoint(t *testing.T) {
	t.Run("host_and_port_are_split", func(t *testing.T) {
		host, port, err := dbruntime.ParseReplicaEndpoint("10.0.0.1:3306")
		require.NoError(t, err)
		require.Equal(t, "10.0.0.1", host)
		require.EqualValues(t, 3306, port)
	})

	t.Run("a_bracketed_address_keeps_its_host", func(t *testing.T) {
		// An IPv6 replica is written the way every other address is, so the
		// brackets belong to the notation rather than to the host.
		host, port, err := dbruntime.ParseReplicaEndpoint("[::1]:5432")
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
			_, _, err := dbruntime.ParseReplicaEndpoint(endpoint)

			require.Error(t, err)
			// The message names the offending entry, which is what turns a
			// typo in a replica list into something findable.
			require.ErrorContains(t, err, "want host:port")
			require.ErrorContains(t, err, endpoint)
		})
	}
}
