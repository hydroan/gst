package controller

import (
	"context"
	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/database"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/requestctx"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// CreateHandler returns a Gin handler that creates one resource.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// M, runs the create flow (see createFlow), and returns the created model.
// Creating a resource requires a body: an absent one is refused, and a client
// wanting a resource with all defaults states that intent with an explicit {}
// body.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its Create method runs on the bound payload, a multipart
// form left unbound for the service to read itself.
func CreateHandler[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), consts.Create, consts.CreateBefore, consts.CreateAfter)
	if !a.typesEqual {
		return a.serviceHandler()
	}
	return func(c *gin.Context) {
		// The span context lives on in the request context the span start
		// rebinds, which requestContext reads for the flow.
		_, span := a.startControllerSpan(c)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), consts.Create)

		req := a.newModel()
		if reqErr := bindJSONRequest(c, &req); reqErr != nil {
			// Creating a resource requires a body, so an absent one is refused
			// rather than answered as a success that wrote nothing.
			reqErr = requiredBodyError(reqErr)
			log.Errorz("bind request body failed", zap.Error(reqErr))
			JSON(c, CodeInvalidParam.WithErr(reqErr))
			gstotel.RecordError(span, reqErr)
			return
		}
		a.normalizeModel(&req)

		if err := a.createFlow(requestContext(c), ginServiceContext(c), req); err != nil {
			JSON(c, failureCoder(err))
			return
		}
		JSON(c, CodeSuccess, req)
	}
}

// CreateCall returns the create call of M on route, the counterpart of the
// handler CreateHandler returns for the generated handler of a Create rpc:
// given the route parameters and the model the request message decoded
// into, it validates the model the way the handler validates a bound body,
// runs the create flow (see createFlow) and answers with the model created,
// or with the status the failure maps to (see call).
func CreateCall[M types.Model](route string) func(ctx context.Context, params map[string]string, m M) (M, error) {
	a := newAction[M, M, M](route, consts.Create, consts.CreateBefore, consts.CreateAfter)
	return func(ctx context.Context, params map[string]string, m M) (M, error) {
		var zero M
		c := a.beginCall(ctx, params, nil)
		defer c.end()
		a.normalizeModel(&m)
		if err := validateRequest(m); err != nil {
			return zero, c.invalidMessage(err)
		}
		if err := a.createFlow(c.ctx, c.serviceContext, m); err != nil {
			return zero, c.fail(err)
		}
		return answer(c, m)
	}
}

// createFlow runs the create flow on req: it takes the creator and updater
// from the identity the request carries, runs the create hooks around the
// write, and records the operation. The created model is req itself, filled
// by the write.
func (a *action[M, REQ, RSP]) createFlow(ctx context.Context, newServiceContext serviceContextFunc, req M) error {
	log := logger.Controller.WithContext(ctx, consts.Create)
	svc := a.service()
	username := requestctx.FromContext(ctx).Username()
	req.SetCreatedBy(username)
	req.SetUpdatedBy(username)

	// 1.Perform business logic processing before create resource.
	if err := a.traceServiceHook(ctx, consts.CreateBefore, svc, func(spanCtx context.Context) error {
		return svc.CreateBefore(newServiceContext(spanCtx, consts.CreateBefore), req)
	}); err != nil {
		return failService(ctx, log, err)
	}
	// 2.Create resource in database. Create is a pure INSERT: a primary or
	// unique key collision (including one held by a soft-deleted row)
	// surfaces as ErrDuplicatedKey and renders 409.
	if err := database.Database[M](ctx).WithExpand(req.Expands()).Create(req); err != nil {
		return failDatabase(ctx, log, err)
	}
	// 3.Perform business logic processing after create resource
	if err := a.traceServiceHook(ctx, consts.CreateAfter, svc, func(spanCtx context.Context) error {
		return svc.CreateAfter(newServiceContext(spanCtx, consts.CreateAfter), req)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// 4.record operation log to database.
	// Record, Request, and Response carry the same serialized payload on
	// this action, so one marshal feeds all three columns.
	if err := audit.RecordOperation(ctx, req, consts.OP_CREATE,
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
