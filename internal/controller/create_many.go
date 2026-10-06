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

// CreateManyHandler returns a Gin handler that creates multiple resources.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// batch[M], runs the batch create flow (see createManyFlow), and
// returns the request data.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its CreateMany method runs on the bound payload.
func CreateManyHandler[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), consts.CreateMany, consts.CreateManyBefore, consts.CreateManyAfter)
	if !a.typesEqual {
		return a.serviceHandler()
	}
	return func(c *gin.Context) {
		// The span context lives on in the request context the span start
		// rebinds, which requestContext reads for the flow.
		_, span := a.startControllerSpan(c)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), consts.CreateMany)

		var req batch[M]
		if reqErr := bindJSONRequest(c, &req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
			log.Errorz("bind request body failed", zap.Error(reqErr))
			response.Error(c, invalidArgument(reqErr))
			gstotel.RecordError(span, reqErr)
			return
		}
		normalizeBatch(&req)

		if err := a.createManyFlow(requestContext(c), ginServiceContext(c), &req); err != nil {
			response.Error(c, err)
			return
		}
		response.JSON(c, req)
	}
}

// CreateManyCall returns the batch create call of M on route, the
// counterpart of the handler CreateManyHandler returns for the generated
// handler of a CreateMany rpc: given the route parameters and the items the
// request message decoded into, it validates the batch the way the handler
// validates a bound body, runs the batch create flow (see createManyFlow)
// and answers with the items created, or with the status the failure maps
// to (see call).
func CreateManyCall[M types.Model](route string) func(ctx context.Context, params map[string]string, items []M) ([]M, error) {
	a := newAction[M, M, M](route, consts.CreateMany, consts.CreateManyBefore, consts.CreateManyAfter)
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
		if err := a.createManyFlow(c.ctx, c.serviceContext, &req); err != nil {
			return nil, c.fail(err)
		}
		return answer(c, req.Items)
	}
}

// createManyFlow runs the batch create flow on the items of req: it takes the
// creator and updater of every item from the identity the request carries,
// runs the batch create hooks around the write, and records the operation.
// The items are req's own, filled by the write.
func (a *action[M, REQ, RSP]) createManyFlow(ctx context.Context, newServiceContext serviceContextFunc, req *batch[M]) error {
	log := logger.Controller.WithContext(ctx, consts.CreateMany)
	svc := a.service()
	val := a.newModel()
	username := requestctx.FromContext(ctx).Username()
	for _, m := range req.Items {
		m.SetCreatedBy(username)
		m.SetUpdatedBy(username)
	}

	// 1.Perform business logic processing before batch create resource.
	if err := a.traceServiceHook(ctx, consts.CreateManyBefore, svc, newServiceContext, func(sc *types.ServiceContext) error {
		return svc.CreateManyBefore(sc, req.Items...)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// 2.Batch create resource in database. Pure INSERT with one transaction
	// around the batch: any unique-key collision renders 409 and rolls the
	// whole batch back. A batch without items writes nothing.
	if err := database.Database[M](ctx).WithExpand(val.Expands()).Create(req.Items...); err != nil {
		return failDatabase(ctx, log, err)
	}
	// 3.Perform business logic processing after batch create resource
	if err := a.traceServiceHook(ctx, consts.CreateManyAfter, svc, newServiceContext, func(sc *types.ServiceContext) error {
		return svc.CreateManyAfter(sc, req.Items...)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// 4.record operation log to database.
	// Record, Request, and Response carry the same serialized payload on
	// this action, so one marshal feeds all three columns.
	if err := audit.RecordOperation(ctx, val, consts.OP_CREATE_MANY,
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
