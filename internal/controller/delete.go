package controller

import (
	"context"
	"encoding/json"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/logger"
	modellogmgmt "github.com/hydroan/gst/internal/modulemodel/logmgmt"
	"github.com/hydroan/gst/internal/tracing"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// deleteFlow runs the delete flow on the record id names: it runs the delete
// hooks around the write, reads the record first when the operation log
// records deletes, and records the operation. id must not be empty (see
// setID); an id the model rejects answers 404 (see notFound), and so does a
// record that is missing or soft deleted: existence is enforced by the
// database layer, whose ErrRecordNotFound renders 404, or by the read the
// operation log needs when that runs first. Whether the row is purged is the
// model's decision (its Purge method), never the request's.
func (a *action[M, REQ, RSP]) deleteFlow(ctx context.Context, newServiceContext serviceContextFunc, id string) error {
	log := logger.Controller.WithContext(ctx, apiaction.Delete)
	svc := a.service()

	m := a.newModel()
	if !setID(m, id) {
		// An id the model rejects cannot match any row; answer 404 instead
		// of passing an unset id to the database layer.
		return reportFailure(log, trace.SpanFromContext(ctx), "route id rejected by model", nil, notFound(nil), zap.String("id", id))
	}

	// 1.Perform business logic processing before delete resource.
	if err := a.traceServiceHook(ctx, apiaction.DeleteBefore, svc, newServiceContext, func(sc *gst.ServiceContext) error {
		return svc.DeleteBefore(sc, m)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// The record as it was, for the operation log: read before the delete,
	// since afterwards there is nothing to read; only when the log records
	// deletes, so a disabled audit costs no query; and from the primary the
	// delete runs on, since a replica may not have caught up on the record.
	var copied M
	if audit.Enabled(gst.OperationDelete) {
		copied = a.newModel()
		copied.SetID(m.GetID())
		if err := database.Database[M](ctx).WithReplica(false).WithExpand(copied.Expands()).Get(copied, m.GetID()); err != nil {
			// The read has answered existence: a record that is gone is
			// the 404 the delete below would answer.
			if errors.Is(err, database.ErrRecordNotFound) {
				return notFound(err)
			}
			log.Errorz("database operation failed", zap.Error(err))
			tracing.RecordError(trace.SpanFromContext(ctx), err)
		}
	}

	// 2.Delete resource in database. The database layer answers existence:
	// ErrRecordNotFound renders 404.
	if err := database.Database[M](ctx).Delete(m); err != nil {
		return failDatabase(ctx, log, err)
	}
	// 3.Perform business logic processing after delete resource.
	if err := a.traceServiceHook(ctx, apiaction.DeleteAfter, svc, newServiceContext, func(sc *gst.ServiceContext) error {
		return svc.DeleteAfter(sc, m)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// 4.record operation log to database.
	if err := audit.RecordOperation(ctx, a.newModel(), gst.OperationDelete,
		func() *modellogmgmt.OperationLog {
			record, _ := json.Marshal(copied)
			entry := operationLog(ctx, a.name)
			entry.RecordID = m.GetID()
			entry.Record = bytesToString(record)
			return entry
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}
	return nil
}
