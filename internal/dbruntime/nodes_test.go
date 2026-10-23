package dbruntime_test

import (
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/dbruntime/dbnode"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAttachNodesRoundTrip(t *testing.T) {
	handle := new(gorm.DB)
	require.Nil(t, dbruntime.NodesFor(handle), "a plain handle carries no nodes")

	nodes := []dbruntime.Node{{Role: dbnode.RolePrimary}, {Role: dbnode.RoleReplica}}
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

// TestAttachResolverReturnsAHandleSafeToReuse pins that the handle
// AttachResolver returns, the one database.DB answers with once replicas are
// configured, can be chained on again and again the way a handle without
// replicas can: a condition one chain set is no condition of the next.
func TestAttachResolverReturnsAHandleSafeToReuse(t *testing.T) {
	type reusedRecord struct {
		ID   uint
		Name string
	}
	dir := t.TempDir()
	primary, err := gorm.Open(sqlite.Open(filepath.Join(dir, "primary.db")), &gorm.Config{})
	require.NoError(t, err)
	handle, err := dbruntime.AttachResolver(primary, []gorm.Dialector{sqlite.Open(filepath.Join(dir, "replica.db"))})
	require.NoError(t, err)
	t.Cleanup(func() {
		for _, node := range dbruntime.NodesFor(handle) {
			require.NoError(t, node.DB.Close())
		}
		dbruntime.AttachNodes(handle, nil)
	})
	require.NoError(t, handle.AutoMigrate(&reusedRecord{}))
	require.NoError(t, handle.Create(&[]reusedRecord{{Name: "first"}, {Name: "second"}}).Error)

	var named []reusedRecord
	require.NoError(t, handle.Where("name = ?", "first").Find(&named).Error)
	require.Len(t, named, 1)
	var all []reusedRecord
	require.NoError(t, handle.Find(&all).Error)
	require.Len(t, all, 2, "the condition of the chain before is no condition of this one")
}
