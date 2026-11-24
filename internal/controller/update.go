package controller

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
	modellogmgmt "github.com/hydroan/gst/internal/modulemodel/logmgmt"
	"github.com/hydroan/gst/internal/util"
	"github.com/hydroan/gst/logger"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// updateFlow runs the update flow, replacing the record id names with req: it
// clears the creator, the updater and the read-only fields req carries (see
// clearServerFields), runs the update hooks around the write, which names
// the caller the request carries as the updater, and records the operation.
// The write is the full row, or, for a model declaring read-only columns,
// every column but those (see writeFieldsOf): a read-only column keeps what
// is stored, whatever the request carried and whatever a hook set. Existence
// is enforced by the database layer instead of a pre-read: a missing or
// soft-deleted record surfaces as database.ErrRecordNotFound and renders
// 404, and a unique-key collision renders 409. The UpdateBefore service hook
// therefore runs before existence is known. After a successful write, and
// before the hook after it, the flow reads the record back from the primary
// and backfills the columns the update left alone, the creation audit and
// the read-only ones among them (see backfillUnwrittenColumns), keeping the
// rest of req intact so hook-populated fields survive; req is the
// replacement answered. The id req carries is replaced by id, which must not
// be empty (see setID): SetID keeps an id already set, so the one the body
// carries is cleared first, or the replacement would land on the record it
// names; an id the model rejects answers 404 (see notFound) without touching
// the database.
func (a *action[M, REQ, RSP]) updateFlow(ctx context.Context, newServiceContext serviceContextFunc, id string, req M) error {
	log := logger.Controller.WithContext(ctx, apiaction.Update)
	svc := a.service()

	m := a.newModel()
	if !setID(m, id) {
		// An id the model rejects cannot match any row; answer 404 without
		// touching the database.
		return reportFailure(log, trace.SpanFromContext(ctx), "route id rejected by model", nil, notFound(nil), zap.String("id", id))
	}
	req.ClearID()
	req.SetID(id)
	a.clearServerFields(req)

	// 1.Perform business logic processing before update resource.
	if err := a.traceServiceHook(ctx, apiaction.UpdateBefore, svc, newServiceContext, func(sc *gst.ServiceContext) error {
		return svc.UpdateBefore(sc, req)
	}); err != nil {
		return failService(ctx, log, err)
	}
	// 2.Update resource in database. The database layer answers existence:
	// ErrRecordNotFound renders 404, ErrDuplicatedKey renders 409. WithSelect
	// given no column narrows nothing: the write is the full row.
	if err := database.Database[M](ctx).WithSelect(a.writable...).Update(req); err != nil {
		return failDatabase(ctx, log, err)
	}
	// The columns the update left alone hold whatever the client sent or a
	// hook set; they are read back as stored, from the primary, before the
	// hook after the write sees them (see backfillUnwrittenColumns). On a
	// reload failure keep req as is: the update itself already committed.
	reloaded := a.newModel()
	if reloadErr := database.Database[M](ctx).WithReplica(false).Get(reloaded, id); reloadErr != nil {
		log.Warnz("reload the columns the update leaves alone failed", zap.Error(reloadErr))
	} else {
		backfillUnwrittenColumns(a.unwritten, reflect.ValueOf(req).Elem(), reflect.ValueOf(reloaded).Elem())
	}
	// 3.Perform business logic processing after update resource.
	if err := a.traceServiceHook(ctx, apiaction.UpdateAfter, svc, newServiceContext, func(sc *gst.ServiceContext) error {
		return svc.UpdateAfter(sc, req)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// 4.record operation log to database.
	// Record, Request, and Response carry the same serialized payload on
	// this action, so one marshal feeds all three columns.
	if err := audit.RecordOperation(ctx, req, gst.OperationUpdate,
		func() *modellogmgmt.OperationLog {
			record, _ := json.Marshal(req)
			entry := operationLog(ctx, a.name)
			entry.RecordID = req.GetID()
			entry.Record = util.BytesToString(record)
			entry.Request = util.BytesToString(record)
			entry.Response = util.BytesToString(record)
			return entry
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}
	return nil
}

// backfillUnwrittenColumns copies onto dst, from stored, the columns an
// update leaves alone, unwritten naming them by index path (see
// writeFieldsOf): those gorm writes on insert only or never, the creation
// audit among them, and the ones the model declares read-only. dst holds
// what the request carried or a hook set for them, which the database did
// not take, and an update answers what is stored. The columns an update
// writes keep dst's values, so what a hook set on them stands, and so do the
// fields no column stores.
func backfillUnwrittenColumns(unwritten [][]int, dst, stored reflect.Value) {
	for _, index := range unwritten {
		value, err := stored.FieldByIndexErr(index)
		if err != nil {
			continue
		}
		if target := fieldByIndexAllocating(dst, index); target.IsValid() && target.CanSet() {
			target.Set(value)
		}
	}
}
