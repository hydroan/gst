package controller

import (
	"context"
	"encoding/json"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
	modellogmgmt "github.com/hydroan/gst/internal/modulemodel/logmgmt"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
)

// createFlow runs the create flow on req: it clears the creator, the updater
// and the read-only fields req carries (see clearServerFields), runs the
// create hooks around the write, which names the caller the request carries
// as creator and updater, and records the operation. The created model is
// req itself, filled by the write.
func (a *action[M, REQ, RSP]) createFlow(ctx context.Context, newServiceContext serviceContextFunc, req M) error {
	log := logger.Controller.WithContext(ctx, apiaction.Create)
	svc := a.service()
	a.clearServerFields(req)

	// 1.Perform business logic processing before create resource.
	if err := a.traceServiceHook(ctx, apiaction.CreateBefore, svc, newServiceContext, func(sc *gst.ServiceContext) error {
		return svc.CreateBefore(sc, req)
	}); err != nil {
		return failService(ctx, log, err)
	}
	// 2.Create resource in database. Create is a pure INSERT: a collision on
	// the primary key, a soft-deleted row's included, or on a unique key
	// surfaces as ErrDuplicatedKey and renders 409.
	if err := database.Database[M](ctx).Create(req); err != nil {
		return failDatabase(ctx, log, err)
	}
	// 3.Perform business logic processing after create resource
	if err := a.traceServiceHook(ctx, apiaction.CreateAfter, svc, newServiceContext, func(sc *gst.ServiceContext) error {
		return svc.CreateAfter(sc, req)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// 4.record operation log to database.
	// Record, Request, and Response carry the same serialized payload on
	// this action, so one marshal feeds all three columns.
	if err := audit.RecordOperation(ctx, req, gst.OperationCreate,
		func() *modellogmgmt.OperationLog {
			record, _ := json.Marshal(req)
			entry := operationLog(ctx, a.name)
			entry.RecordID = req.GetID()
			entry.Record = bytesToString(record)
			entry.Request = bytesToString(record)
			entry.Response = bytesToString(record)
			return entry
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}
	return nil
}
