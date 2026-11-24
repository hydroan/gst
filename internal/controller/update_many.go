package controller

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
	modellogmgmt "github.com/hydroan/gst/internal/modulemodel/logmgmt"
	"github.com/hydroan/gst/internal/util"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
)

// updateManyFlow runs the batch update flow on the items of req: it refuses
// a batch carrying no item (see missingItemsMsg), a batch naming one record
// twice (see repeatedID) and an item naming no record (see itemWithoutID)
// with 400, clears the creator, the updater and the read-only fields every
// item carries (see clearServerFields), runs the batch update hooks around
// the write, which names the caller the request carries as the updater and,
// like an update of one record, leaves the read-only columns as stored (see
// updateFlow), and records the operation. The items are req's own, as the
// write and the hooks left them, with the columns an update leaves alone,
// the creation audit and the read-only ones among them, as stored (see
// backfillUnwrittenColumns).
func (a *action[M, REQ, RSP]) updateManyFlow(ctx context.Context, newServiceContext serviceContextFunc, req *batch[M]) error {
	log := logger.Controller.WithContext(ctx, apiaction.UpdateMany)
	svc := a.service()

	if len(req.Items) == 0 {
		return failWith(ctx, log, "batch update without items", nil, badRequest(missingItemsMsg))
	}
	if err := req.repeatedID(); err != nil {
		return failWith(ctx, log, "batch update naming a record twice", err, invalidArgument(err))
	}
	// An item without an id names no record: a defective request, refused
	// before anything is written with the sentence a batch patch refuses it
	// with (see itemWithoutID); the write would refuse it as well (see
	// database.ErrIDRequired), without naming the item.
	for i, m := range req.Items {
		if len(m.GetID()) == 0 {
			err := errors.Wrapf(database.ErrIDRequired, "update many %s item %d", a.name, i)
			return failWith(ctx, log, "batch update item without its id", err, itemWithoutID(i))
		}
	}
	a.clearServerFields(req.Items...)
	// 1.Perform business logic processing before batch update resource.
	if err := a.traceServiceHook(ctx, apiaction.UpdateManyBefore, svc, newServiceContext, func(sc *gst.ServiceContext) error {
		return svc.UpdateManyBefore(sc, req.Items...)
	}); err != nil {
		return failService(ctx, log, err)
	}
	// 2.Batch update resource in database. Pure UPDATE with one transaction
	// around the batch: an item without a live row renders 404 and rolls the
	// batch back, so the batch endpoint can never insert rows. WithSelect
	// given no column narrows nothing: the write is the full row.
	if err := database.Database[M](ctx).WithSelect(a.writable...).Update(req.Items...); err != nil {
		return failDatabase(ctx, log, err)
	}
	// The columns the update left alone hold whatever the client sent or a
	// hook set; they are read back as stored, in one statement from the
	// primary, before the hook after the write sees them (see
	// backfillUnwrittenColumns). On a reload failure the items stay as they
	// are: the update itself already committed.
	if stored, reloadErr := a.recordsByID(database.Database[M](ctx).WithReplica(false), req.itemIDs()); reloadErr != nil {
		log.Warnz("reload the columns the update leaves alone failed", zap.Error(reloadErr))
	} else {
		for _, m := range req.Items {
			if record, ok := stored[m.GetID()]; ok {
				backfillUnwrittenColumns(a.unwritten, reflect.ValueOf(m).Elem(), reflect.ValueOf(record).Elem())
			}
		}
	}
	// 3.Perform business logic processing after batch update resource.
	if err := a.traceServiceHook(ctx, apiaction.UpdateManyAfter, svc, newServiceContext, func(sc *gst.ServiceContext) error {
		return svc.UpdateManyAfter(sc, req.Items...)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// 4.record operation log to database.
	// Record, Request, and Response carry the same serialized payload on
	// this action, so one marshal feeds all three columns.
	if err := audit.RecordOperation(ctx, a.newModel(), gst.OperationUpdateMany,
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
