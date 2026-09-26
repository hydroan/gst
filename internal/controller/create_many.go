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
	"github.com/hydroan/gst/internal/requestctx"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// requestData is the body of a batch request: the items to create, update or
// patch, or the ids to delete.
//
// TODO: decide whether a batch update or patch may skip the items whose
// record does not exist and apply the rest, instead of failing whole. A
// switch for it would be a member of this body beside items, and the answer
// would have to list the items it skipped.
//
// TODO: decide whether a batch delete may accept an empty id instead of
// refusing the request, switched on by a member of this body beside ids.
type requestData[M types.Model] struct {
	// IDs is the id list that should be batch delete.
	IDs []string `json:"ids,omitempty"`
	// Items is the resource list that should be batch create/update/partial
	// update. Each item is validated against its binding tags the way the
	// body of a single-resource request is: the validator only descends into
	// a slice told to dive.
	Items []M `json:"items,omitempty" binding:"dive"`
}

// CreateManyFactory returns a Gin handler that creates multiple resources.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// requestData[M], runs the batch create flow (see createManyFlow), and
// returns the request data.
//
// When REQ or RSP differs from M, the handler binds the JSON body into REQ and
// delegates the operation to the phase service's CreateMany method.
func CreateManyFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	meta := newFactoryMeta[M, REQ, RSP](routeFromConfig(cfg...), consts.PHASE_CREATE_MANY, consts.PHASE_CREATE_MANY_BEFORE, consts.PHASE_CREATE_MANY_AFTER)
	return func(c *gin.Context) {
		ctrlSpanCtx, span := meta.startControllerSpan(c)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_CREATE_MANY)

		if !meta.typesEqual {
			var err error
			var rsp RSP
			req := meta.newRequest()
			svc := meta.service()

			if reqErr := bindJSONRequest(c, &req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
				log.Errorz("bind request body failed", zap.Error(reqErr))
				JSON(c, CodeInvalidParam.WithErr(reqErr))
				gstotel.RecordError(span, reqErr)
				return
			}
			meta.normalizeRequest(&req)
			if rsp, err = meta.traceServiceOperation(ctrlSpanCtx, consts.PHASE_CREATE_MANY, func(spanCtx context.Context) (RSP, error) {
				return svc.CreateMany(types.NewServiceContext(c, spanCtx, consts.PHASE_CREATE_MANY), req)
			}); err != nil {
				log.Errorz("service operation failed", zap.Error(err))
				handleServiceError(c, err)
				gstotel.RecordError(span, err)
				return
			}
			// Check if response is already written (e.g., SSE streaming)
			if !c.Writer.Written() {
				JSON(c, CodeSuccess, rsp)
			}
			return
		}

		var req requestData[M]
		if reqErr := bindJSONRequest(c, &req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
			log.Errorz("bind request body failed", zap.Error(reqErr))
			JSON(c, CodeInvalidParam.WithErr(reqErr))
			gstotel.RecordError(span, reqErr)
			return
		}
		normalizeBatchRequest(&req)

		if err := meta.createManyFlow(requestContext(c), ginServiceContext(c), &req); err != nil {
			JSON(c, failureCoder(err))
			return
		}
		JSON(c, CodeSuccess, req)
	}
}

// CreateManyCall returns the batch create call of M on route, the
// counterpart of the handler CreateManyFactory returns for the generated
// handler of a CreateMany rpc: given the route parameters and the items the
// request message decoded into, it validates the batch the way the handler
// validates a bound body, runs the batch create flow (see createManyFlow)
// and answers with the items created, or with the status the failure maps
// to (see call).
func CreateManyCall[M types.Model](route string) func(ctx context.Context, params map[string]string, items []M) ([]M, error) {
	meta := newFactoryMeta[M, M, M](route, consts.PHASE_CREATE_MANY, consts.PHASE_CREATE_MANY_BEFORE, consts.PHASE_CREATE_MANY_AFTER)
	return func(ctx context.Context, params map[string]string, items []M) ([]M, error) {
		c := meta.beginCall(ctx, params, nil)
		defer c.end()
		req := requestData[M]{Items: items}
		normalizeBatchRequest(&req)
		if err := validateRequest(&req); err != nil {
			return nil, c.invalidMessage(err)
		}
		if err := meta.createManyFlow(c.ctx, c.serviceContext, &req); err != nil {
			return nil, c.fail(err)
		}
		return answer(c, req.Items)
	}
}

// createManyFlow runs the batch create flow on the items of req: it takes the
// creator and updater of every item from the identity the request carries,
// runs the batch create hooks around the write, and records the operation.
// The items are req's own, filled by the write.
func (meta *factoryMeta[M, REQ, RSP]) createManyFlow(ctx context.Context, newServiceContext serviceContextFunc, req *requestData[M]) error {
	log := logger.Controller.WithContext(ctx, consts.PHASE_CREATE_MANY)
	svc := meta.service()
	val := meta.newModel()
	username := requestctx.FromContext(ctx).Username()
	for _, m := range req.Items {
		m.SetCreatedBy(username)
		m.SetUpdatedBy(username)
	}

	// 1.Perform business logic processing before batch create resource.
	if err := meta.traceServiceHook(ctx, consts.PHASE_CREATE_MANY_BEFORE, svc, func(spanCtx context.Context) error {
		return svc.CreateManyBefore(newServiceContext(spanCtx, consts.PHASE_CREATE_MANY_BEFORE), req.Items...)
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
	if err := meta.traceServiceHook(ctx, consts.PHASE_CREATE_MANY_AFTER, svc, func(spanCtx context.Context) error {
		return svc.CreateManyAfter(newServiceContext(spanCtx, consts.PHASE_CREATE_MANY_AFTER), req.Items...)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// 4.record operation log to database.
	// Record, Request, and Response carry the same serialized payload on
	// this action, so one marshal feeds all three columns.
	if err := am.RecordOperation(ctx, val, consts.OP_CREATE_MANY,
		func() *modellogmgmt.OperationLog {
			record, _ := json.Marshal(req)
			entry := operationLog(ctx, meta.name)
			entry.Record = util.BytesToString(record)
			entry.Request = util.BytesToString(record)
			entry.Response = util.BytesToString(record)
			return entry
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}
	return nil
}
