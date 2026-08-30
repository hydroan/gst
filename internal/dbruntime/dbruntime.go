package dbruntime

import (
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/modelregistry"
	prommetrics "github.com/hydroan/gst/metrics"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// DB holds the framework-managed default GORM database handle.
//
// The database runtime updates it during initialization, and public packages
// expose read-only accessors for application code.
var DB *gorm.DB

// NowUTC produces every framework-managed timestamp: it is the gorm.Config
// NowFunc shared by the dialect packages (the updated_at refresh on Update,
// the deleted_at stamp on soft delete) and the explicit source
// database.Create/Upsert stamp rows with. Two decisions live here:
//
//   - UTC, the framework's one time base. The gorm default time.Now() carries
//     the server's local zone: drivers with a UTC wire location still store
//     the right instant, but the in-memory model then serializes with a local
//     offset instead of the UTC form read rows carry, and sqlite would even
//     persist that local offset into the row text.
//   - Millisecond truncation, matching the millisecond storage precision the
//     dialects share (MySQL datetime(3), ClickHouse DateTime64(3)). Without
//     it the in-memory value keeps nanoseconds the row cannot hold, so the
//     timestamp a write hands back differs from what a later read returns —
//     and MySQL rounds half up, which can even shift the stored millisecond.
func NowUTC() time.Time { return time.Now().UTC().Truncate(time.Millisecond) }

// InitDatabase publishes db as the framework's default handle and starts
// preparing the table of every registered model.
//
// Preparation runs on a goroutine of its own that drains the registration
// queue as it fills, so models may register at any stage: before, during or
// after this call. Wait is what blocks until the queue is empty.
func InitDatabase(db *gorm.DB) error {
	// A mistyped comment mode must fail the boot, not silently strip the
	// annotations an operator relies on; see config.SQLCommentMode.
	if err := config.App.Database.SQLComment.Validate(); err != nil {
		return err
	}
	if tablePreparationStarted.CompareAndSwap(0, 1) {
		go func() {
			for m := range modelregistry.TableChan {
				prepareTable(db, m)
			}
		}()
	}

	// From here on every framework read and write reaches the database
	// through this handle.
	DB = db

	registerPoolMetrics(db)
	return nil
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
