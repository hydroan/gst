package controller

import (
	"context"
	"encoding/json"

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
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// DeleteHandler returns a Gin handler that deletes one resource.
//
// When M, REQ, and RSP are the same type, the handler reads the resource id
// from the configured route parameter (batch deletion uses the DeleteMany
// action instead), runs the delete flow (see deleteFlow), and returns a
// success response.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its Delete method runs on the bound payload.
func DeleteHandler[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), consts.Delete, consts.DeleteBefore, consts.DeleteAfter)
	if !a.typesEqual {
		return a.serviceHandler()
	}
	return func(c *gin.Context) {
		// The span context lives on in the request context the span start
		// rebinds, which requestContext reads for the flow.
		_, span := a.startControllerSpan(c)
		defer span.End()

		reqMeta := requestctx.FromGin(c)
		log := logger.Controller.WithContext(c.Request.Context(), consts.Delete)

		// The resource id comes from the configured route parameter only.
		var id string
		if len(cfg) > 0 {
			id = reqMeta.Param(util.Deref(cfg[0]).ParamName)
		}
		if len(id) == 0 {
			log.Errorz(missingRouteParamMsg)
			response.Error(c, badRequest(missingRouteParamMsg))
			gstotel.RecordError(span, errors.New(missingRouteParamMsg))
			return
		}

		if err := a.deleteFlow(requestContext(c), ginServiceContext(c), id); err != nil {
			response.Error(c, err)
			return
		}
		response.JSON(c)
	}
}

// DeleteCall returns the delete call of M on route, the counterpart of the
// handler DeleteHandler returns for the generated handler of a Delete rpc:
// given the route parameters and the id the request message names the
// record by, it runs the delete flow (see deleteFlow) and answers nothing,
// or the status the failure maps to (see call).
func DeleteCall[M types.Model](route string) func(ctx context.Context, params map[string]string, id string) error {
	a := newAction[M, M, M](route, consts.Delete, consts.DeleteBefore, consts.DeleteAfter)
	return func(ctx context.Context, params map[string]string, id string) error {
		c, err := a.beginCall(ctx, params, nil)
		defer c.end()
		if err != nil {
			return c.invalid(err)
		}
		if id == "" {
			return c.missingID()
		}
		if err := a.deleteFlow(c.ctx, c.serviceContext, id); err != nil {
			return c.fail(err)
		}
		return c.finish()
	}
}

// deleteFlow runs the delete flow on the record id names: it runs the delete
// hooks around the write, keeps a copy of the record for the operation log,
// and records the operation. id must not be empty (see setID); an id the
// model rejects answers 404 (see notFound). Whether the row is purged is the
// model's decision (its Purge method), never the request's.
func (a *action[M, REQ, RSP]) deleteFlow(ctx context.Context, newServiceContext serviceContextFunc, id string) error {
	log := logger.Controller.WithContext(ctx, consts.Delete)
	svc := a.service()

	// 'm' is a fresh model instance, such as: &model.User{ID: myid, Name: myname}.
	m := a.newModel()
	if !setID(m, id) {
		// An id the model rejects cannot match any row; answer 404 instead
		// of passing an unset id to the database layer.
		log.Errorz("route id rejected by model", zap.String("id", id))
		return notFound(nil)
	}

	// 1.Perform business logic processing before delete resource.
	if err := a.traceServiceHook(ctx, consts.DeleteBefore, svc, newServiceContext, func(sc *types.ServiceContext) error {
		return svc.DeleteBefore(sc, m)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// find out the record and keep a copy for the operation log.
	copied := a.newModel()
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
	if err := a.traceServiceHook(ctx, consts.DeleteAfter, svc, newServiceContext, func(sc *types.ServiceContext) error {
		return svc.DeleteAfter(sc, m)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// 4.record operation log to database.
	if err := audit.RecordOperation(ctx, a.newModel(), consts.OP_DELETE,
		func() *modellogmgmt.OperationLog {
			record, _ := json.Marshal(copied)
			entry := operationLog(ctx, a.name)
			entry.RecordID = m.GetID()
			entry.Record = util.BytesToString(record)
			return entry
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}
	return nil
}
