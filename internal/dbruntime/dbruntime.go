package dbruntime

import (
	"time"

	"github.com/hydroan/gst/internal/modelregistry"
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
// after this call. Wait is what blocks until every queued model has its table.
func InitDatabase(db *gorm.DB) error {
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
