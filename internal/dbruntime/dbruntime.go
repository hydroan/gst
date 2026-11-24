// Package dbruntime holds the database runtime below the public database
// package: what that package, the dialect packages and the framework's other
// users of the database share, and a project never sees.
//
// It keeps the default handle and prepares the table of every registered
// model as the process starts (see InitDatabase and Wait); it holds the steps
// every dialect takes on a handle it opens, the pool limits, tracing, the
// translation of the errors a client's data causes and the replicas behind a
// handle (see ConfigurePool, InstallTracing, InstallErrorTranslation and
// AttachResolver); it carries a transaction on a context (see WithTx and
// Handle) and serializes what the processes of a deployment starting together
// must run one at a time (see Serialized); and it defines the framework's
// time base (see UTCMillisecond) and the statements renaming a table or an
// index in each dialect (see RenameTableSQL and RenameIndexSQL).
package dbruntime

import (
	"time"

	"github.com/hydroan/gst/internal/model"
	"gorm.io/gorm"
)

// DB holds the framework-managed default GORM database handle.
//
// The database runtime updates it during initialization, and public packages
// expose read-only accessors for application code.
var DB *gorm.DB

// UTCMillisecond brings t onto the framework's time base, UTC truncated to
// the millisecond. Two decisions live here:
//
//   - UTC, the framework's one time base. A time produced by time.Now
//     carries the server's local zone: drivers with a UTC wire location
//     still store the right instant, but the in-memory model then
//     serializes with a local offset instead of the UTC form read rows
//     carry.
//   - Millisecond truncation, matching the millisecond storage precision the
//     dialects share (MySQL datetime(3), ClickHouse DateTime64(3)). Without
//     it the in-memory value keeps nanoseconds the row cannot hold, so the
//     time a write hands back differs from what a later read returns — and
//     MySQL rounds half up, which can even shift the stored millisecond.
//
// Every time a model carries goes through it on its way into a row, so a
// project produces its times with time.Now and reads back what it wrote.
func UTCMillisecond(t time.Time) time.Time { return t.UTC().Truncate(time.Millisecond) }

// NowUTC produces every framework-managed timestamp: it is the gorm.Config
// NowFunc shared by the dialect packages (the updated_at refresh on Update,
// the deleted_at stamp on soft delete) and the explicit source
// database.Create/Upsert stamp rows with, the current time on the time base
// (see UTCMillisecond).
func NowUTC() time.Time { return UTCMillisecond(time.Now()) }

// InitDatabase publishes db as the framework's default handle and starts
// preparing the table of every registered model.
//
// Preparation runs on a goroutine of its own that drains the registration
// queue as it fills, so models may register at any stage: before, during or
// after this call. Wait is what blocks until every queued model has its table.
func InitDatabase(db *gorm.DB) error {
	if tablePreparationStarted.CompareAndSwap(0, 1) {
		go func() {
			for m := range model.TableChan {
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
