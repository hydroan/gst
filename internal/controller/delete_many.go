package controller

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/apiaction"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/util"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// deleteManyFlow runs the batch delete flow on the ids of req: it converts
// them into model instances, which become the items of req, runs the batch
// delete hooks around the write, reads the records in one statement first
// when the operation log records batch deletes, and records the operation
// with the records as they were. The batch is all or nothing: a batch naming
// no id (see missingIDsMsg) and an empty id, or one of whitespace alone, are
// defective requests refused before anything is deleted; an id the model
// rejects names no record and answers 404 with nothing deleted, as does an
// id no stored record carries, which the database layer reports as
// ErrRecordNotFound. Whether the rows are purged is the model's decision
// (its Purge method), never the request's.
func (a *action[M, REQ, RSP]) deleteManyFlow(ctx context.Context, newServiceContext serviceContextFunc, req *batch[M]) error {
	log := logger.Controller.WithContext(ctx, apiaction.DeleteMany)
	svc := a.service()

	if len(req.IDs) == 0 {
		return failWith(ctx, log, "batch delete without ids", nil, badRequest(missingIDsMsg))
	}
	// 1.Perform business logic processing before batch delete resources.
	req.Items = make([]M, 0, len(req.IDs))
	for _, id := range req.IDs {
		// An empty id, or one of whitespace alone, names no record: a
		// defective request, refused before anything is deleted.
		// Setting an empty one on a UUID-keyed model would mint a fresh
		// id instead. Any other id is used as sent, never trimmed.
		if strings.TrimSpace(id) == "" {
			err := errors.Wrapf(database.ErrIDRequired, "delete many %s", a.name)
			return failWith(ctx, log, "batch delete with an empty id", err, databaseError(err))
		}
		m := a.newModel()
		if !setID(m, id) {
			// An id the model rejects names no record, and every id of the
			// batch must name one: answer 404 with nothing deleted.
			return reportFailure(log, trace.SpanFromContext(ctx), "batch delete with an id the model rejects", nil, notFound(nil), zap.String("id", id))
		}
		req.Items = append(req.Items, m)
	}
	if err := a.traceServiceHook(ctx, apiaction.DeleteManyBefore, svc, newServiceContext, func(sc *types.ServiceContext) error {
		return svc.DeleteManyBefore(sc, req.Items...)
	}); err != nil {
		return failService(ctx, log, err)
	}
	// The records as they were, for the operation log: read before the
	// delete, since afterwards there is nothing to read; in one statement
	// for the batch; only when the log records batch deletes, so a disabled
	// audit costs no query; and from the primary the delete runs on, since
	// a replica may not have caught up on the records.
	var deleted []M
	if audit.Enabled(types.OperationDeleteMany) {
		stored, err := a.recordsByID(database.Database[M](ctx).WithReplica(false).WithExpand(a.newModel().Expands()), req.itemIDs())
		if err != nil {
			log.Errorz("database operation failed", zap.Error(err))
			gstotel.RecordError(trace.SpanFromContext(ctx), err)
		}
		for _, m := range req.Items {
			if record, ok := stored[m.GetID()]; ok {
				deleted = append(deleted, record)
			}
		}
	}
	// 2.Batch delete resources in database. Every id must name a live record
	// (see database.Delete), so ErrRecordNotFound renders 404 with nothing
	// deleted.
	if err := database.Database[M](ctx).Delete(req.Items...); err != nil {
		return failDatabase(ctx, log, err)
	}
	// 3.Perform business logic processing after batch delete resources.
	if err := a.traceServiceHook(ctx, apiaction.DeleteManyAfter, svc, newServiceContext, func(sc *types.ServiceContext) error {
		return svc.DeleteManyAfter(sc, req.Items...)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// 4.record operation log to database.
	if err := audit.RecordOperation(ctx, a.newModel(), types.OperationDeleteMany,
		func() *modellogmgmt.OperationLog {
			record, _ := json.Marshal(batch[M]{IDs: req.IDs, Items: deleted})
			entry := operationLog(ctx, a.name)
			entry.Record = util.BytesToString(record)
			return entry
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}
	return nil
}
