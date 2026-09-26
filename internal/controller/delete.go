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
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// DeleteFactory returns a Gin handler that deletes one resource.
//
// When M, REQ, and RSP are the same type, the handler reads the resource id
// from the configured route parameter (batch deletion uses the DeleteMany
// action instead), runs the delete flow (see deleteFlow), and returns a
// success response.
//
// When REQ or RSP differs from M, the handler binds the JSON body into REQ and
// delegates the operation to the phase service's Delete method.
func DeleteFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	meta := newFactoryMeta[M, REQ, RSP](routeFromConfig(cfg...), consts.PHASE_DELETE, consts.PHASE_DELETE_BEFORE, consts.PHASE_DELETE_AFTER)
	return func(c *gin.Context) {
		ctrlSpanCtx, span := meta.startControllerSpan(c)
		defer span.End()

		reqMeta := requestctx.FromGin(c)
		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_DELETE)

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
			if rsp, err = meta.traceServiceOperation(ctrlSpanCtx, consts.PHASE_DELETE, func(spanCtx context.Context) (RSP, error) {
				return svc.Delete(types.NewServiceContext(c, spanCtx, consts.PHASE_DELETE), req)
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

		// The resource id comes from the configured route parameter only.
		var id string
		if len(cfg) > 0 {
			id = reqMeta.Param(util.Deref(cfg[0]).ParamName)
		}
		if len(id) == 0 {
			log.Errorz(missingRouteParamMsg)
			JSON(c, CodeInvalidParam.WithMsg(missingRouteParamMsg))
			gstotel.RecordError(span, errors.New(missingRouteParamMsg))
			return
		}

		if err := meta.deleteFlow(requestContext(c), ginServiceContext(c), id); err != nil {
			JSON(c, failureCoder(err))
			return
		}
		JSON(c, CodeSuccess)
	}
}

// DeleteCall returns the delete call of M on route, the counterpart of the
// handler DeleteFactory returns for the generated handler of a Delete rpc:
// given the route parameters and the id the request message names the
// record by, it runs the delete flow (see deleteFlow) and answers nothing,
// or the status the failure maps to (see call).
func DeleteCall[M types.Model](route string) func(ctx context.Context, params map[string]string, id string) error {
	meta := newFactoryMeta[M, M, M](route, consts.PHASE_DELETE, consts.PHASE_DELETE_BEFORE, consts.PHASE_DELETE_AFTER)
	return func(ctx context.Context, params map[string]string, id string) error {
		c := meta.beginCall(ctx, params, nil)
		defer c.end()
		if id == "" {
			return c.missingID()
		}
		if err := meta.deleteFlow(c.ctx, c.serviceContext, id); err != nil {
			return c.fail(err)
		}
		return c.finish()
	}
}

// deleteFlow runs the delete flow on the record id names: it runs the delete
// hooks around the write, keeps a copy of the record for the operation log,
// and records the operation. id must not be empty (see setRouteID); an id the
// model rejects answers CodeNotFound. Whether the row is purged is the
// model's decision (its Purge method), never the request's.
func (meta *factoryMeta[M, REQ, RSP]) deleteFlow(ctx context.Context, newServiceContext serviceContextFunc, id string) error {
	log := logger.Controller.WithContext(ctx, consts.PHASE_DELETE)
	svc := meta.service()

	// 'm' is a fresh model instance, such as: &model.User{ID: myid, Name: myname}.
	m := meta.newModel()
	if !setRouteID(m, id) {
		// An id the model rejects cannot match any row; answer 404 instead
		// of passing an unset id to the database layer.
		log.Errorz("route id rejected by model", zap.String("id", id))
		return &failure{coder: CodeNotFound}
	}

	// 1.Perform business logic processing before delete resource.
	if err := meta.traceServiceHook(ctx, consts.PHASE_DELETE_BEFORE, svc, func(spanCtx context.Context) error {
		return svc.DeleteBefore(newServiceContext(spanCtx, consts.PHASE_DELETE_BEFORE), m)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// find out the record and keep a copy for the operation log.
	copied := meta.newModel()
	copied.SetID(m.GetID())
	if err := database.Database[M](ctx).WithExpand(copied.Expands()).Get(copied, m.GetID()); err != nil {
		log.Errorz("database operation failed", zap.Error(err))
		gstotel.RecordError(trace.SpanFromContext(ctx), err)
	}

	// 2.Delete resource in database.
	if err := database.Database[M](ctx).Delete(m); err != nil {
		return failDatabase(ctx, log, err)
	}
	// 3.Perform business logic processing after delete resource.
	if err := meta.traceServiceHook(ctx, consts.PHASE_DELETE_AFTER, svc, func(spanCtx context.Context) error {
		return svc.DeleteAfter(newServiceContext(spanCtx, consts.PHASE_DELETE_AFTER), m)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// 4.record operation log to database.
	if err := am.RecordOperation(ctx, meta.newModel(), consts.OP_DELETE,
		func() *modellogmgmt.OperationLog {
			record, _ := json.Marshal(copied)
			entry := operationLog(ctx, meta.name)
			entry.RecordID = m.GetID()
			entry.Record = util.BytesToString(record)
			return entry
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}
	return nil
}
