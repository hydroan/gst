package controller

import (
	"context"
	"encoding/json"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
	modellogmgmt "github.com/hydroan/gst/internal/modulemodel/logmgmt"
	"github.com/hydroan/gst/internal/util"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
)

// createManyFlow runs the batch create flow on the items of req: it refuses
// a batch carrying no item with 400 (see missingItemsMsg), clears the
// creator, the updater and the read-only fields every item carries (see
// clearServerFields), runs the batch create hooks around the write, which
// names the caller the request carries as creator and updater, and records
// the operation. The items are req's own, filled by the write.
func (a *action[M, REQ, RSP]) createManyFlow(ctx context.Context, newServiceContext serviceContextFunc, req *batch[M]) error {
	log := logger.Controller.WithContext(ctx, apiaction.CreateMany)
	svc := a.service()
	val := a.newModel()
	if len(req.Items) == 0 {
		return failWith(ctx, log, "batch create without items", nil, badRequest(missingItemsMsg))
	}
	a.clearServerFields(req.Items...)

	// 1.Perform business logic processing before batch create resource.
	if err := a.traceServiceHook(ctx, apiaction.CreateManyBefore, svc, newServiceContext, func(sc *gst.ServiceContext) error {
		return svc.CreateManyBefore(sc, req.Items...)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// 2.Batch create resource in database. Pure INSERT with one transaction
	// around the batch: any unique-key collision renders 409 and rolls the
	// whole batch back.
	if err := database.Database[M](ctx).Create(req.Items...); err != nil {
		return failDatabase(ctx, log, err)
	}
	// 3.Perform business logic processing after batch create resource
	if err := a.traceServiceHook(ctx, apiaction.CreateManyAfter, svc, newServiceContext, func(sc *gst.ServiceContext) error {
		return svc.CreateManyAfter(sc, req.Items...)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// 4.record operation log to database.
	// Record, Request, and Response carry the same serialized payload on
	// this action, so one marshal feeds all three columns.
	if err := audit.RecordOperation(ctx, val, gst.OperationCreateMany,
		func() *modellogmgmt.OperationLog {
			record, _ := json.Marshal(req)
			entry := operationLog(ctx, a.name)
			entry.Record = util.BytesToString(record)
			entry.Request = util.BytesToString(record)
			entry.Response = util.BytesToString(record)
			return entry
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}
	return nil
}
