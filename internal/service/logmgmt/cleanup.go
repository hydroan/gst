package servicelogmgmt

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/types"
)

// cleanupBatchSize bounds one deletion round. Each round runs in its own
// transaction, so lock time and undo growth stay flat however large the
// expired backlog is.
const cleanupBatchSize = 1000

// Cleanup deletes operation and login logs older than the configured
// retention (config key logmgmt.retention / LOGMGMT_RETENTION, 90 days by
// default). Errors surface to the cronjob runner instead of being swallowed:
// a log table that stops being trimmed must alarm, not rot silently.
//
// The add path registers it through module/logmgmt; projects using copied
// logmgmt source register it in their own cronjob setup, for example
// cronjob.Register(servicelogmgmt.Cleanup, "0 0 * * * *", "cleanup logs").
func Cleanup() error {
	cutoff := time.Now().UTC().Add(-config.App.Logmgmt.Retention)
	if err := cleanupExpired[*modellogmgmt.OperationLog](cutoff); err != nil {
		return errors.Wrap(err, "cleanup operation logs")
	}
	if err := cleanupExpired[*modellogmgmt.LoginLog](cutoff); err != nil {
		return errors.Wrap(err, "cleanup login logs")
	}
	return nil
}

// cleanupExpired removes expired rows batch by batch: every round loads at
// most one batch and deletes it in its own transaction, so neither memory nor
// a single long transaction grows with the backlog. Both log models purge on
// delete, so each round physically reclaims space.
func cleanupExpired[M types.Model](cutoff time.Time) error {
	expired := types.QueryOptions{Filters: []types.Filter{types.FilterLte("created_at", cutoff)}}
	for {
		batch := make([]M, 0, cleanupBatchSize)
		if err := database.Database[M](context.Background()).
			WithQuery(*new(M), expired).
			WithLimit(cleanupBatchSize).
			List(&batch); err != nil {
			return errors.Wrap(err, "list expired rows")
		}
		if len(batch) == 0 {
			return nil
		}
		if err := database.Database[M](context.Background()).Delete(batch...); err != nil {
			return errors.Wrap(err, "delete expired rows")
		}
		if len(batch) < cleanupBatchSize {
			return nil
		}
	}
}
