package controller

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/apiaction"
	modellogmgmt "github.com/hydroan/gst/internal/modulemodel/logmgmt"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/util"
	"github.com/hydroan/gst/logger"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// patchFlow runs the patch flow on the record id names: it loads the record,
// copies the fields of req present in the request (fields) into it, clears
// the updater it was stored with, runs the patch hooks around the write,
// which names the caller the request carries as the updater, records the
// operation, and returns the patched record. id must not be empty (see
// setID); an id the model rejects, and one naming no record, both answer 404
// (see notFound).
//
// The write is the whole record loaded, not only the fields the request
// carried. Concurrent patches of one record therefore resolve as last writer
// wins: a later patch puts back the fields an earlier one changed, even when
// the two touched different fields, and both answer success. That is the
// default contract of every framework update, not a defect; a model that
// needs the stale write refused instead declares model.Version.
func (a *action[M, REQ, RSP]) patchFlow(ctx context.Context, newServiceContext serviceContextFunc, id string, req M, fields patchFieldSet) (M, error) {
	var zero M
	log := logger.Controller.WithContext(ctx, apiaction.Patch)
	svc := a.service()

	data := make([]M, 0)
	m := a.newModel()
	if !setID(m, id) {
		// An id the model rejects cannot match any row; answer 404 without
		// relying on the empty-query safety net below.
		return zero, reportFailure(log, trace.SpanFromContext(ctx), "route id rejected by model", nil, notFound(nil), zap.String("id", id))
	}

	// Make sure the record already exists. The read is pinned to the
	// primary because what it reads is written straight back: the patch
	// merges onto this row, so a replica still catching up would have the
	// fields the request does not touch written back stale.
	if err := database.Database[M](ctx).WithReplica(false).WithLimit(1).WithQuery(m).List(&data); err != nil {
		return zero, failDatabase(ctx, log, err)
	}
	if len(data) != 1 {
		return zero, reportFailure(log, trace.SpanFromContext(ctx), "records matched by id is not exactly one", nil, notFound(nil), zap.Int("count", len(data)), zap.String("id", id))
	}
	newVal := reflect.ValueOf(req).Elem()
	oldVal := reflect.ValueOf(data[0]).Elem()
	applyPatch(log, a.typ, oldVal, newVal, fields)
	cur := oldVal.Addr().Interface().(M) //nolint:errcheck
	// The updater is the write's to name, the caller or, for an anonymous
	// request, none, as on an update (see clearServerFields): the one stored
	// before did not make this change. The creator stays as stored, the
	// record being answered whole.
	cur.SetUpdatedBy("")

	// 1.Perform business logic processing before partial update resource.
	if err := a.traceServiceHook(ctx, apiaction.PatchBefore, svc, newServiceContext, func(sc *types.ServiceContext) error {
		return svc.PatchBefore(sc, cur)
	}); err != nil {
		return zero, failService(ctx, log, err)
	}
	// 2.Partial update resource in database. The record was loaded above, so
	// ErrRecordNotFound only fires when it vanished in between; unique-key
	// collisions from the patched values render 409.
	if err := database.Database[M](ctx).Update(cur); err != nil {
		return zero, failDatabase(ctx, log, err)
	}
	// 3.Perform business logic processing after partial update resource.
	if err := a.traceServiceHook(ctx, apiaction.PatchAfter, svc, newServiceContext, func(sc *types.ServiceContext) error {
		return svc.PatchAfter(sc, cur)
	}); err != nil {
		return zero, failService(ctx, log, err)
	}

	// 4.record operation log to database.
	// Record and Request both carry the request payload, so one marshal
	// feeds both columns; Response carries the resulting row instead. The
	// entry names the patched record by its own id: the body need not carry
	// one, the route named the record.
	if err := audit.RecordOperation(ctx, req, types.OperationPatch,
		func() *modellogmgmt.OperationLog {
			record, _ := json.Marshal(req)
			respData, _ := json.Marshal(cur)
			entry := operationLog(ctx, a.name)
			entry.RecordID = cur.GetID()
			entry.Record = util.BytesToString(record)
			entry.Request = util.BytesToString(record)
			entry.Response = util.BytesToString(respData)
			return entry
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}
	return cur, nil
}
