package database

import (
	"database/sql"

	"github.com/hydroan/gst/internal/dbruntime"
)

// Node roles reported in NodeStats: the writable primary, and the read
// replicas attached through the mysql.replicas / postgres.replicas
// configuration.
const (
	RolePrimary = dbruntime.RolePrimary
	RoleReplica = dbruntime.RoleReplica
)

// NodeStats is the connection pool snapshot of one database node.
type NodeStats struct {
	// Role names the node's place in the topology, RolePrimary for the
	// writable node.
	Role string
	// DBStats is the standard library pool snapshot: open, in-use and idle
	// connections, wait counts and durations.
	DBStats sql.DBStats
}

// Stats reports the connection pool snapshot of every node of the default
// database, for callers that surface pool state themselves: a custom health
// endpoint, a debug page, a startup print. The Prometheus collector the
// framework registers at initialization reads the same source; this is the
// programmatic view of it.
//
// With read replicas configured it reports one snapshot per node, the
// primary first in registration order; without them, the single primary.
//
// It returns nil when the database is not initialized: a snapshot of nothing
// is empty rather than an error, unlike Health, which is asked for an
// authoritative answer and panics there like Database[M].
func Stats() []NodeStats {
	db := DB()
	if db == nil {
		return nil
	}
	if nodes := dbruntime.NodesFor(db); len(nodes) > 0 {
		stats := make([]NodeStats, 0, len(nodes))
		for _, node := range nodes {
			stats = append(stats, NodeStats{Role: node.Role, DBStats: node.DB.Stats()})
		}
		return stats
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil
	}
	return []NodeStats{{Role: RolePrimary, DBStats: sqlDB.Stats()}}
}
