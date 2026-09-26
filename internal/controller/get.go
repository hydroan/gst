package controller

import (
	"context"
	"time"

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

// GetFactory returns a Gin handler that retrieves one resource.
//
// When M, REQ, and RSP are the same type, the handler reads the configured route
// parameter as the resource id, runs the get flow (see getFlow), and returns
// the model.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its Get method runs on a zero-value REQ, the GET request
// carrying no body; the service reads ServiceContext.Query() and Param().
func GetFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), consts.PHASE_GET, consts.PHASE_GET_BEFORE, consts.PHASE_GET_AFTER)
	if !a.typesEqual {
		return a.serviceHandler()
	}
	return func(c *gin.Context) {
		// The span context lives on in the request context the span start
		// rebinds, which requestContext reads for the flow.
		_, span := a.startControllerSpan(c)
		defer span.End()

		reqMeta := requestctx.FromGin(c)
		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_GET)

		var param string
		if len(cfg) > 0 {
			param = reqMeta.Param(util.Deref(cfg[0]).ParamName)
		}
		if len(param) == 0 {
			log.Errorz(missingRouteParamMsg)
			JSON(c, CodeInvalidParam.WithMsg(missingRouteParamMsg))
			gstotel.RecordError(span, errors.New(missingRouteParamMsg))
			return
		}

		m, err := a.getFlow(requestContext(c), ginServiceContext(c), param)
		if err != nil {
			JSON(c, failureCoder(err))
			return
		}
		JSON(c, CodeSuccess, m)
	}
}

// GetCall returns the get call of M on route, the counterpart of the handler
// GetFactory returns for the generated handler of a Get rpc: given the route
// parameters, the id the request message names the record by and its
// expansion query, it runs the get flow (see getFlow) and answers with the
// model, or with the status the failure maps to (see call); a message
// naming no record is refused the way a request missing its route parameter
// is.
func GetCall[M types.Model](route string) func(ctx context.Context, params map[string]string, id string, query Query) (M, error) {
	a := newAction[M, M, M](route, consts.PHASE_GET, consts.PHASE_GET_BEFORE, consts.PHASE_GET_AFTER)
	return func(ctx context.Context, params map[string]string, id string, query Query) (M, error) {
		var zero M
		c, err := a.beginQueryCall(ctx, params, query)
		defer c.end()
		if err != nil {
			return zero, c.invalid(err)
		}
		if id == "" {
			return zero, c.missingID()
		}
		m, err := a.getFlow(c.ctx, c.serviceContext, id)
		if err != nil {
			return zero, c.fail(err)
		}
		return answer(c, m)
	}
}

// getFlow runs the get flow on the record id names: it applies the expansion
// and depth query options the request carries, runs the get hooks around the
// read, records the operation, and returns the model. id must not be empty:
// a UUID-keyed model mints a fresh id for an empty one (see setRouteID). An
// id the model rejects, and a read that finds no stored record, both answer
// CodeNotFound.
func (a *action[M, REQ, RSP]) getFlow(ctx context.Context, newServiceContext serviceContextFunc, id string) (M, error) {
	var zero M
	log := logger.Controller.WithContext(ctx, consts.PHASE_GET)
	svc := a.service()

	// 'm' is a fresh model instance, such as: &model.User{ID: myid, Name: myname}.
	m := a.newModel()
	// `GetBefore` hook need id.
	if !setRouteID(m, id) {
		// An id the model rejects cannot match any row; answer 404 before
		// the raw value reaches SQL, where implicit string-to-integer
		// coercion could match an unintended row.
		log.Errorz("route id rejected by model", zap.String("id", id))
		return zero, &failure{coder: CodeNotFound}
	}
	expands := parseExpandQuery(requestctx.QueryValues(ctx), m)

	// 1.Perform business logic processing before get resource.
	if err := a.traceServiceHook(ctx, consts.PHASE_GET_BEFORE, svc, func(spanCtx context.Context) error {
		return svc.GetBefore(newServiceContext(spanCtx, consts.PHASE_GET_BEFORE), m)
	}); err != nil {
		return zero, failService(ctx, log, err)
	}
	// 2.Get resource from database. The database layer answers existence:
	// database.ErrRecordNotFound renders 404 instead of a generic failure.
	if err := database.Database[M](ctx).WithExpand(expands).Get(m, m.GetID()); err != nil {
		return zero, failDatabase(ctx, log, err)
	}
	// 3.Perform business logic processing after get resource.
	if err := a.traceServiceHook(ctx, consts.PHASE_GET_AFTER, svc, func(spanCtx context.Context) error {
		return svc.GetAfter(newServiceContext(spanCtx, consts.PHASE_GET_AFTER), m)
	}); err != nil {
		return zero, failService(ctx, log, err)
	}
	// A model without an id or creation time holds no stored record (a
	// missing row already failed above with ErrRecordNotFound), so answer
	// CodeNotFound instead of an empty resource.
	if len(m.GetID()) == 0 || m.GetCreatedAt().Equal(time.Time{}) {
		log.Errorz(CodeNotFound.String())
		err := errors.New(CodeNotFound.Msg())
		gstotel.RecordError(trace.SpanFromContext(ctx), err)
		return zero, &failure{coder: CodeNotFound, err: err}
	}

	// 4.record operation log to database.
	if err := am.RecordOperation(ctx, m, consts.OP_GET,
		func() *modellogmgmt.OperationLog {
			return operationLog(ctx, a.name)
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}
	return m, nil
}
