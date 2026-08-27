package database

import (
	"database/sql"
)

// RolePrimary names the writable node of a database in NodeStats. Today the
// default database is that single node; read replicas will report their own
// role once read/write splitting lands.
const RolePrimary = "primary"

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
// It returns nil when the database is not initialized: a snapshot of nothing
// is empty rather than an error, unlike Health, which is asked for an
// authoritative answer and panics there like Database[M].
func Stats() []NodeStats {
	db := DB()
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil
	}
	return []NodeStats{{Role: RolePrimary, DBStats: sqlDB.Stats()}}
}
