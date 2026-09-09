package auditmanager

import (
	"context"
	"slices"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/ds/queue/circularbuffer"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

// AuditManager manages audit logging based on configuration.
// It provides a centralized way to handle operation logging across all Factory functions,
// replacing the previous direct enqueuing of OperationLog records.
// The manager supports configurable filtering, field exclusion, and data truncation.
type AuditManager struct {
	config *config.Audit
	cb     *circularbuffer.CircularBuffer[*modellogmgmt.OperationLog]
}

// New creates a new audit manager instance.
// This replaces the previous direct usage of circular buffer for operation logging.
func New(auditConfig *config.Audit, cb *circularbuffer.CircularBuffer[*modellogmgmt.OperationLog]) *AuditManager {
	return &AuditManager{
		config: auditConfig,
		cb:     cb,
	}
}

// RecordOperation records a single operation audit log, with configurable
// filtering and support for both synchronous and asynchronous writing.
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

	if am.config.AsyncWrite {
		// Use existing circular buffer for async writing
		am.cb.Enqueue(operationLog)
		return nil
	}

	// Synchronous writing
	if err := database.Database[*modellogmgmt.OperationLog](ctx).Create(operationLog); err != nil {
		return errors.Wrap(err, "failed to write audit log")
	}
	return nil
}

// Consume operation log.
func (am *AuditManager) Consume() {
	operationLogs := make([]*modellogmgmt.OperationLog, 0, config.App.Server.CircularBuffer.SizeOperationLog)
	ticker := time.NewTicker(5 * time.Second)

	for range ticker.C {
		operationLogs = operationLogs[:0]
		for !am.cb.IsEmpty() {
			ol, _ := am.cb.Dequeue()
			operationLogs = append(operationLogs, ol)
		}
		if len(operationLogs) > 0 {
			if err := database.Database[*modellogmgmt.OperationLog](context.Background()).WithBatchSize(1000).Create(operationLogs...); err != nil {
				zap.S().Error(err)
			}
		}
	}
}
