package controller

import (
	"context"
	"encoding/json"
	"io"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/database"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// UpdateManyFactory returns a Gin handler that replaces multiple resources.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// requestData[M], runs the batch update flow (see updateManyFlow), and
// returns the request data.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its UpdateMany method runs on the bound payload.
func UpdateManyFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), consts.PHASE_UPDATE_MANY, consts.PHASE_UPDATE_MANY_BEFORE, consts.PHASE_UPDATE_MANY_AFTER)
	if !a.typesEqual {
		return a.serviceHandler()
	}
	return func(c *gin.Context) {
		// The span context lives on in the request context the span start
		// rebinds, which requestContext reads for the flow.
		_, span := a.startControllerSpan(c)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_UPDATE_MANY)

		var req requestData[M]
		if reqErr := bindJSONRequest(c, &req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
			log.Errorz("bind request body failed", zap.Error(reqErr))
			JSON(c, CodeInvalidParam.WithErr(reqErr))
			gstotel.RecordError(span, reqErr)
			return
		}
		normalizeBatchRequest(&req)

		if err := a.updateManyFlow(requestContext(c), ginServiceContext(c), &req); err != nil {
			JSON(c, failureCoder(err))
			return
		}
		JSON(c, CodeSuccess, req)
	}
}

// UpdateManyCall returns the batch update call of M on route, the
// counterpart of the handler UpdateManyFactory returns for the generated
// handler of an UpdateMany rpc: given the route parameters and the items the
// request message decoded into, it validates the batch the way the handler
// validates a bound body, runs the batch update flow (see updateManyFlow)
// and answers with the items as stored, or with the status the failure maps
// to (see call).
func UpdateManyCall[M types.Model](route string) func(ctx context.Context, params map[string]string, items []M) ([]M, error) {
	a := newAction[M, M, M](route, consts.PHASE_UPDATE_MANY, consts.PHASE_UPDATE_MANY_BEFORE, consts.PHASE_UPDATE_MANY_AFTER)
	return func(ctx context.Context, params map[string]string, items []M) ([]M, error) {
		c := a.beginCall(ctx, params, nil)
		defer c.end()
		req := requestData[M]{Items: items}
		normalizeBatchRequest(&req)
		if err := validateRequest(&req); err != nil {
			return nil, c.invalidMessage(err)
		}
		if err := a.updateManyFlow(c.ctx, c.serviceContext, &req); err != nil {
			return nil, c.fail(err)
		}
		return answer(c, req.Items)
	}
}

// updateManyFlow runs the batch update flow on the items of req: it runs the
// batch update hooks around the write and records the operation. The items
// are req's own, as the write and the hooks left them.
func (a *action[M, REQ, RSP]) updateManyFlow(ctx context.Context, newServiceContext serviceContextFunc, req *requestData[M]) error {
	log := logger.Controller.WithContext(ctx, consts.PHASE_UPDATE_MANY)
	svc := a.service()

	// 1.Perform business logic processing before batch update resource.
	if err := a.traceServiceHook(ctx, consts.PHASE_UPDATE_MANY_BEFORE, svc, func(spanCtx context.Context) error {
		return svc.UpdateManyBefore(newServiceContext(spanCtx, consts.PHASE_UPDATE_MANY_BEFORE), req.Items...)
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
	if err := a.traceServiceHook(ctx, consts.PHASE_UPDATE_MANY_AFTER, svc, func(spanCtx context.Context) error {
		return svc.UpdateManyAfter(newServiceContext(spanCtx, consts.PHASE_UPDATE_MANY_AFTER), req.Items...)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// 4.record operation log to database.
	// Record, Request, and Response carry the same serialized payload on
	// this action, so one marshal feeds all three columns.
	if err := am.RecordOperation(ctx, a.newModel(), consts.OP_UPDATE_MANY,
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
