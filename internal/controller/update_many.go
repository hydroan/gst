package controller

import (
	"context"
	"encoding/json"
	"io"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/consts"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// UpdateManyHandler returns a Gin handler that replaces multiple resources.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// batch[M], runs the batch update flow (see updateManyFlow), and
// returns the request data.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its UpdateMany method runs on the bound payload.
func UpdateManyHandler[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), consts.UpdateMany, consts.UpdateManyBefore, consts.UpdateManyAfter)
	if !a.typesEqual {
		return a.serviceHandler()
	}
	return func(c *gin.Context) {
		// The span context lives on in the request context the span start
		// rebinds, which requestContext reads for the flow.
		_, span := a.startControllerSpan(c)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), consts.UpdateMany)

		var req batch[M]
		if reqErr := bindJSONRequest(c, &req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
			log.Errorz("bind request body failed", zap.Error(reqErr))
			response.Error(c, invalidArgument(reqErr))
			gstotel.RecordError(span, reqErr)
			return
		}
		normalizeBatch(&req)

		if err := a.updateManyFlow(requestContext(c), ginServiceContext(c), &req); err != nil {
			response.Error(c, err)
			return
		}
		response.JSON(c, req)
	}
}

// UpdateManyCall returns the batch update call of M on route, the
// counterpart of the handler UpdateManyHandler returns for the generated
// handler of an UpdateMany rpc: given the route parameters and the items the
// request message decoded into, it validates the batch the way the handler
// validates a bound body, runs the batch update flow (see updateManyFlow)
// and answers with the items as stored, or with the status the failure maps
// to (see call).
func UpdateManyCall[M types.Model](route string) func(ctx context.Context, params map[string]string, items []M) ([]M, error) {
	a := newAction[M, M, M](route, consts.UpdateMany, consts.UpdateManyBefore, consts.UpdateManyAfter)
	return func(ctx context.Context, params map[string]string, items []M) ([]M, error) {
		c, err := a.beginCall(ctx, params, nil)
		defer c.end()
		if err != nil {
			return nil, c.invalid(err)
		}
		req := batch[M]{Items: items}
		normalizeBatch(&req)
		if err := validateRequest(&req); err != nil {
			return nil, c.invalidMessage(err)
		}
		if err := a.updateManyFlow(c.ctx, c.serviceContext, &req); err != nil {
			return nil, c.fail(err)
		}
		return answer(c, req.Items)
	}
}

// updateManyFlow runs the batch update flow on the items of req: it refuses
// a batch naming one record twice with 400 (see repeatedID), stamps the
// caller on every item as its updater, runs the batch update hooks around
// the write and records the operation. The items are req's own, as the
// write and the hooks left them, with the creation audit as stored.
func (a *action[M, REQ, RSP]) updateManyFlow(ctx context.Context, newServiceContext serviceContextFunc, req *batch[M]) error {
	log := logger.Controller.WithContext(ctx, consts.UpdateMany)
	svc := a.service()

	if err := req.repeatedID(); err != nil {
		return failWith(ctx, log, "batch update naming a record twice", err, invalidArgument(err))
	}
	// The caller is who updates the records, whatever the items carry.
	username := requestctx.FromContext(ctx).Username()
	for _, m := range req.Items {
		m.SetUpdatedBy(username)
	}
	// 1.Perform business logic processing before batch update resource.
	if err := a.traceServiceHook(ctx, consts.UpdateManyBefore, svc, newServiceContext, func(sc *types.ServiceContext) error {
		return svc.UpdateManyBefore(sc, req.Items...)
	}); err != nil {
		return failService(ctx, log, err)
	}
	// 2.Batch update resource in database. Pure UPDATE with one transaction
	// around the batch: an item without an id fails the whole request, and
	// an item without a live row renders 404 and rolls the batch back, so
	// the batch endpoint can never insert rows. A batch without items writes
	// nothing.
	if err := database.Database[M](ctx).Update(req.Items...); err != nil {
		return failDatabase(ctx, log, err)
	}
	// 3.Perform business logic processing after batch update resource.
	if err := a.traceServiceHook(ctx, consts.UpdateManyAfter, svc, newServiceContext, func(sc *types.ServiceContext) error {
		return svc.UpdateManyAfter(sc, req.Items...)
	}); err != nil {
		return failService(ctx, log, err)
	}
	// Backfill the creation audit columns from the persisted rows, read in
	// one statement from the primary: Update never writes
	// created_at/created_by, so each item holds whatever the client sent.
	// Only these two fields are copied, so the values the hooks set survive.
	// On a reload failure the items stay as they are: the update itself
	// already committed.
	if stored, reloadErr := a.recordsByID(database.Database[M](ctx).WithReplica(false), req.itemIDs()); reloadErr != nil {
		log.Warnz("reload audit columns failed", zap.Error(reloadErr))
	} else {
		for _, m := range req.Items {
			if record, ok := stored[m.GetID()]; ok {
				m.SetCreatedAt(record.GetCreatedAt())
				m.SetCreatedBy(record.GetCreatedBy())
			}
		}
	}

	// 4.record operation log to database.
	// Record, Request, and Response carry the same serialized payload on
	// this action, so one marshal feeds all three columns.
	if err := audit.RecordOperation(ctx, a.newModel(), consts.OP_UPDATE_MANY,
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
