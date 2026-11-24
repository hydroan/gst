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

// patchManyFlow runs the batch patch flow on the items of req: it loads the
// record each item names, copies the fields the item carried (fieldSets, one
// set per item in order; an item past its end carries none) into that
// record, clears the updater it was stored with, runs the batch patch hooks
// around the write, which names the caller the request carries as the
// updater, records the operation, and returns the batch with the patched
// records, as the hooks left them, in place of the items. The batch patches
// all of its items or none, as a batch update does: a batch carrying no item
// (see missingItemsMsg), a batch naming one record twice (see repeatedID)
// and an item without an id fail the whole batch with 400 before any record
// is read, and an item whose record does not exist fails it with 404 before
// anything is written.
//
// Each write is the whole record loaded, not only the fields its item
// carried, so concurrent patches of one record resolve as last writer wins
// exactly as in patchFlow: a later patch puts back the fields an earlier one
// changed, even different ones, and both answer success. A model that needs
// the stale write refused instead declares model.Version.
func (a *action[M, REQ, RSP]) patchManyFlow(ctx context.Context, newServiceContext serviceContextFunc, req *batch[M], fieldSets []patchFieldSet) (batch[M], error) {
	var zero batch[M]
	log := logger.Controller.WithContext(ctx, apiaction.PatchMany)
	svc := a.service()

	if len(req.Items) == 0 {
		return zero, failWith(ctx, log, "batch patch without items", nil, badRequest(missingItemsMsg))
	}
	if err := req.repeatedID(); err != nil {
		return zero, failWith(ctx, log, "batch patch naming a record twice", err, invalidArgument(err))
	}
	// An item without an id names no record: a defective request, refused
	// before any record is read with the sentence PatchItem refuses it with
	// for the generated handler (see itemWithoutID). Setting an empty id on
	// a UUID-keyed model would mint a fresh one instead.
	for i, m := range req.Items {
		if len(m.GetID()) == 0 {
			err := errors.Wrapf(database.ErrIDRequired, "patch many %s item %d", a.name, i)
			return zero, failWith(ctx, log, "batch patch item without its id", err, itemWithoutID(i))
		}
	}
	// One statement reads the records of the batch, pinned to the primary:
	// each row read here is merged with its patch and written straight back,
	// so a stale one would write the untouched fields back as they were on
	// the replica.
	stored, err := a.recordsByID(database.Database[M](ctx).WithReplica(false), req.itemIDs())
	if err != nil {
		return zero, failDatabase(ctx, log, err)
	}
	patched := make([]M, 0, len(req.Items))
	for i, m := range req.Items {
		current, ok := stored[m.GetID()]
		if !ok {
			err := errors.Wrapf(database.ErrRecordNotFound, "patch many %s id=%s", a.name, m.GetID())
			return zero, failWith(ctx, log, "partial update resource not found", err, databaseError(err))
		}
		fields := patchFieldSet{}
		if i < len(fieldSets) {
			fields = fieldSets[i]
		}
		applyPatch(log, a.typ, reflect.ValueOf(current).Elem(), reflect.ValueOf(m).Elem(), fields)
		// The updater is the write's to name, as in patchFlow.
		current.SetUpdatedBy("")
		patched = append(patched, current)
	}

	// 1.Perform business logic processing before batch patch resource.
	if err := a.traceServiceHook(ctx, apiaction.PatchManyBefore, svc, newServiceContext, func(sc *gst.ServiceContext) error {
		return svc.PatchManyBefore(sc, patched...)
	}); err != nil {
		return zero, failService(ctx, log, err)
	}
	// 2.Batch partial update resource in database. The rows were loaded
	// above, so ErrRecordNotFound only fires when one vanished in between;
	// unique-key collisions from the patched values render 409. Either way
	// the transaction rolls the whole batch back.
	if err := database.Database[M](ctx).Update(patched...); err != nil {
		return zero, failDatabase(ctx, log, err)
	}
	// 3.Perform business logic processing after batch patch resource.
	if err := a.traceServiceHook(ctx, apiaction.PatchManyAfter, svc, newServiceContext, func(sc *gst.ServiceContext) error {
		return svc.PatchManyAfter(sc, patched...)
	}); err != nil {
		return zero, failService(ctx, log, err)
	}

	// The response carries the patched records, whose fields the hooks
	// may have set, not the items of the request.
	rsp := *req
	rsp.Items = patched

	// 4.record operation log to database.
	// Record and Request both carry the request payload, so one marshal
	// feeds both columns; Response carries the patched records instead.
	if err := audit.RecordOperation(ctx, a.newModel(), gst.OperationPatchMany,
		func() *modellogmgmt.OperationLog {
			record, _ := json.Marshal(req)
			respData, _ := json.Marshal(rsp)
			entry := operationLog(ctx, a.name)
			entry.Record = util.BytesToString(record)
			entry.Request = util.BytesToString(record)
			entry.Response = util.BytesToString(respData)
			return entry
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}
	return rsp, nil
}
