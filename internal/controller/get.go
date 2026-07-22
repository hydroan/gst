package controller

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/requestctx"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// Get handles a single-resource get request with the default factory settings.
func Get[M types.Model, REQ types.Request, RSP types.Response](c *gin.Context) {
	GetFactory[M, REQ, RSP]()(c)
}

// GetFactory returns a Gin handler that retrieves one resource.
//
// When M, REQ, and RSP are the same type, the handler reads the configured route
// parameter as the resource id, applies expansion, depth, selection, cache, and
// database index query options, runs get hooks, loads the model through the
// configured database handler, records an operation log, and returns the model.
//
// When REQ or RSP differs from M, the handler delegates the operation to the
// phase service's Get method with a zero-value REQ. Get handles an HTTP GET
// request whose body carries no semantics, so nothing is bound into REQ;
// custom services read parameters from ServiceContext.Query() and
// ServiceContext.Param().
func GetFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	handler, _ := extractConfig(cfg...)
	meta := newFactoryMeta[M, REQ, RSP](routeFromConfig(cfg...), consts.PHASE_GET, consts.PHASE_GET_BEFORE, consts.PHASE_GET_AFTER)
	return func(c *gin.Context) {
		ctrlSpanCtx, span := meta.startControllerSpan(c)
		defer span.End()

		reqMeta := requestctx.FromGin(c)
		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_GET)
		svc := meta.service()

		if !meta.typesEqual {
			var err error
			var rsp RSP
			req := meta.newRequest()

			var serviceCtx *types.ServiceContext
			if rsp, err = meta.traceServiceOperation(ctrlSpanCtx, consts.PHASE_GET, func(spanCtx context.Context) (RSP, error) {
				serviceCtx = types.NewServiceContext(c, spanCtx, consts.PHASE_GET)
				return svc.Get(serviceCtx, req)
			}); err != nil {
				log.Error(err)
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

		var param string
		if len(cfg) > 0 {
			param = reqMeta.Param(util.Deref(cfg[0]).ParamName)
		}
		if len(param) == 0 {
			log.Error(CodeNotFoundRouteParam)
			JSON(c, CodeNotFoundRouteParam)
			gstotel.RecordError(span, errors.New(CodeNotFoundRouteParam.Msg()))
			return
		}
		index, _ := c.GetQuery(consts.QUERY_INDEX)
		selects, _ := c.GetQuery(consts.QUERY_SELECT)

		// 'm' is a fresh model instance, such as: &model.User{ID: myid, Name: myname}.
		m := meta.newModel()
		// `GetBefore` hook need id.
		if !setRouteID(m, param) {
			// An id the model rejects cannot match any row; answer 404 before
			// the raw value reaches SQL, where implicit string-to-integer
			// coercion could match an unintended row.
			log.Errorz("route id rejected by model", zap.String("id", param))
			JSON(c, CodeNotFound)
			return
		}

		var err error
		noCache := true // default disable cache.
		if noCacheStr, ok := c.GetQuery(consts.QUERY_NO_CACHE); ok {
			var parsed bool
			if parsed, err = strconv.ParseBool(noCacheStr); err == nil {
				noCache = parsed
			}
		}
		expands := parseExpandQuery(c, m)
		log.Infoz("", zap.Object(meta.name, m))

		// 1.Perform business logic processing before get resource.
		var serviceCtxBefore *types.ServiceContext
		if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_GET_BEFORE, func(spanCtx context.Context) error {
			serviceCtxBefore = types.NewServiceContext(c, spanCtx, consts.PHASE_GET_BEFORE)
			return svc.GetBefore(serviceCtxBefore, m)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}
		// 2.Get resource from database.
		if err = handler(requestContext(c)).
			WithIndex(index).
			WithSelect(strings.Split(selects, ",")...).
			WithExpand(expands).
			WithCache(!noCache).
			Get(m, m.GetID()); err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		// 3.Perform business logic processing after get resource.
		var serviceCtxAfter *types.ServiceContext
		if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_GET_AFTER, func(spanCtx context.Context) error {
			serviceCtxAfter = types.NewServiceContext(c, spanCtx, consts.PHASE_GET_AFTER)
			return svc.GetAfter(serviceCtxAfter, m)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}
		// It will returns a empty types.Model if found nothing from database,
		// we should response status code "CodeNotFound".
		if len(m.GetID()) == 0 || m.GetCreatedAt().Equal(time.Time{}) {
			log.Error(CodeNotFound)
			JSON(c, CodeNotFound)
			gstotel.RecordError(span, errors.New(CodeNotFound.Msg()))
			return
		}

		// 4.record operation log to database.
		// cb.Enqueue(&modellogmgmt.OperationLog{
		// 	OP:        consts.OP_GET,
		// 	Model:     typ.Name(),
		// 	Table:     tableName,
		// 	IP:        c.ClientIP(),
		// 	User:      c.GetString(consts.CTX_USERNAME),
		// 	TraceID: c.GetString(consts.TRACE_ID),
		// 	URI:       c.Request.RequestURI,
		// 	Method:    c.Request.Method,
		// 	UserAgent: c.Request.UserAgent(),
		// })
		if err = am.RecordOperation(requestContext(c), m, &modellogmgmt.OperationLog{
			OP:        consts.OP_GET,
			Model:     meta.name,
			IP:        c.ClientIP(),
			User:      c.GetString(consts.CTX_USERNAME),
			TraceID:   c.GetString(consts.TRACE_ID),
			URI:       c.Request.RequestURI,
			Method:    c.Request.Method,
			UserAgent: c.Request.UserAgent(),
		}); err != nil {
			log.Warn(err)
		}

		JSON(c, CodeSuccess, m)
	}
}
