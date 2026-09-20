package auditmanager

import (
	"context"
	"slices"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/database"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/types"
	prommetrics "github.com/hydroan/gst/metrics"
)

// writeTimeout bounds the statement that writes one entry. The entry is
// written where the operation happened, so this bound is what keeps a
// database that stopped answering from holding the response back, while
// staying wide enough that a loaded one still records the operation.
const writeTimeout = 5 * time.Second

// AuditManager writes the operation log: one entry per operation the
// configuration does not exclude, written by the handler that performed it,
// before the request answers. It holds the audit configuration and nothing
// else — no buffer, no goroutine — so a record that was asked for either
// reaches the table or is reported as failed.
type AuditManager struct {
	config *config.Audit
}

// New creates an audit manager that reads auditConfig on every call, so a
// configuration reloaded in place takes effect without rebuilding it.
func New(auditConfig *config.Audit) *AuditManager {
	return &AuditManager{config: auditConfig}
}

// RecordOperation records a single operation audit log, leaving out what the
// configuration excludes.
//
// The entry is produced by build rather than handed in, because building one
// is the expensive part: it serializes the record, reads the request and
// allocates the entry. Auditing is disabled by default, and a call whose
// argument is already built pays that price on every write request only to
// have it discarded here. build runs once the entry is known to be wanted, so
// a disabled audit costs this call and nothing else.
//
// op is taken separately for the same reason: it decides whether the operation
// is excluded, so it has to be known before build runs. It is stamped onto the
// entry here, which keeps the caller from naming the operation twice.
//
// A nil build is an error rather than a silent skip: an audit entry that was
// asked for and never written is a gap in a security record.
//
// The write runs on a context derived from ctx, which keeps what the request
// carries — the trace the statement is annotated with, the identity every log
// line takes — and leaves its cancellation behind: the operation the entry
// records has already happened, and a client that went away must not take the
// record of it with it. writeTimeout bounds the write instead. A failure is
// counted and returned to the caller, which logs it beside the request.
func (am *AuditManager) RecordOperation(ctx context.Context, m types.Model, op consts.OP,
	build func() *modellogmgmt.OperationLog,
) error {
	// A missing build is the caller failing to say what to record, not a
	// request to record nothing. It is refused before the configuration is
	// consulted, so that turning the audit on is not what first surfaces it.
	if build == nil {
		return errors.New("audit: RecordOperation was given no build function")
	}

	// Skip if audit is disabled
	if !am.config.Enabled {
		return nil
	}

	// Skip if the operation is excluded.
	if slices.Contains(am.config.ExcludeOperations, op) {
		return nil
	}

	operationLog := build()
	operationLog.OP = op
	// Record the table name; every model declares it explicitly.
	operationLog.Table = m.TableName()

	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), writeTimeout)
	defer cancel()
	if err := database.Database[*modellogmgmt.OperationLog](writeCtx).Create(operationLog); err != nil {
		// Counted as well as returned: a record that never reached the table
		// is a gap in a security record, and the count is what an alert can
		// watch. The metric is nil in a process that never initialized them.
		if prommetrics.AuditWriteFailuresTotal != nil {
			prommetrics.AuditWriteFailuresTotal.Inc()
		}
		return errors.Wrap(err, "failed to write audit log")
	}
	return nil
}
