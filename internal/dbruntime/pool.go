package dbruntime

import (
	"database/sql"
	"fmt"

	"github.com/hydroan/gst/config"
	prommetrics "github.com/hydroan/gst/metrics"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ConfigurePool applies the framework's connection pool limits — the
// [database] max_idle_conns, max_open_conns, conn_max_lifetime, and
// conn_max_idle_time settings — to the pool of a freshly opened handle. It is
// the one step every dialect's New shares right after opening, so an
// application-held handle runs under the same pool contract as the default
// one; AttachResolver applies the same limits to every replica pool.
//
// Left at the database/sql defaults a pool keeps two idle connections and no
// lifetime: with more than two statements in flight, every connection beyond
// those two is closed on return and dialed again for the next statement, and
// a connection once opened is never retired.
//
// A dialect with a narrower contract tightens the result afterwards: sqlite
// pins its pool to a single connection, see sqlite.New.
func ConfigurePool(pool *sql.DB) {
	pool.SetMaxIdleConns(config.App.Database.MaxIdleConns)
	pool.SetMaxOpenConns(config.App.Database.MaxOpenConns)
	pool.SetConnMaxLifetime(config.App.Database.ConnMaxLifetime)
	pool.SetConnMaxIdleTime(config.App.Database.ConnMaxIdleTime)
}

// defaultPoolMetricName is the name the framework's default database is
// exposed under in the metrics registry. A plain handle registers its one pool
// under it; with replicas attached the primary keeps it and the replicas
// derive theirs from it.
const defaultPoolMetricName = "default"

// registerPoolMetrics exposes the default database's connection pools to the
// metrics registry: the single pool of a plain handle under the stable name
// "default", and with replicas attached, every node — the primary as
// "default" and replicas as "default_replica_N". Failures only log:
// observability must never block startup, and a deployment without a metrics
// endpoint simply leaves the collectors unserved.
func registerPoolMetrics(db *gorm.DB) {
	if nodes := NodesFor(db); len(nodes) > 0 {
		names := replicaPoolMetricNames(defaultPoolMetricName, nodes)
		for i, node := range nodes {
			if err := prommetrics.RegisterDBStats(node.DB, names[i]); err != nil {
				zap.S().Warnw("failed to register database pool metrics collector", "db_name", names[i], "error", err)
			}
		}
		return
	}
	sqlDB, err := db.DB()
	if err != nil {
		zap.S().Warnw("failed to reach sql.DB for pool metrics", "error", err)
		return
	}
	if err := prommetrics.RegisterDBStats(sqlDB, defaultPoolMetricName); err != nil {
		zap.S().Warnw("failed to register database pool metrics collector",
			"db_name", defaultPoolMetricName, "error", err)
	}
}

// replicaPoolMetricNames names the pool metric of each node for one handle:
// the primary keeps the base name, replicas append their index.
func replicaPoolMetricNames(base string, nodes []DBNode) []string {
	names := make([]string, 0, len(nodes))
	replicaIndex := 0
	for _, node := range nodes {
		if node.Role == RolePrimary {
			names = append(names, base)
			continue
		}
		names = append(names, fmt.Sprintf("%s_replica_%d", base, replicaIndex))
		replicaIndex++
	}
	return names
}
