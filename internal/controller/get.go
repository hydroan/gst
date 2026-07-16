package controller

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/requestctx"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/serviceregistry"
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
	return func(c *gin.Context) {
		ctrlSpanCtx, span := startControllerSpan[M](c, consts.PHASE_GET)
		defer span.End()

		meta := requestctx.FromGin(c)
		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_GET)
		svc := serviceregistry.Resolve[M, REQ, RSP](consts.PHASE_GET)

		if !modelregistry.AreTypesEqual[M, REQ, RSP]() {
			var err error
			var req REQ
			var rsp RSP

			reqTyp := reflect.TypeFor[REQ]()
			switch reqTyp.Kind() {
			case reflect.Struct:
				req = reflect.New(reqTyp).Elem().Interface().(REQ) //nolint:errcheck
			case reflect.Pointer:
				for reqTyp.Kind() == reflect.Pointer {
					reqTyp = reqTyp.Elem()
				}
				req = reflect.New(reqTyp).Interface().(REQ) //nolint:errcheck
			}

			var serviceCtx *types.ServiceContext
			if rsp, err = traceServiceOperation[M, RSP](ctrlSpanCtx, consts.PHASE_GET, func(spanCtx context.Context) (RSP, error) {
				serviceCtx = types.NewServiceContext(c, spanCtx, consts.PHASE_GET)
				return svc.Get(serviceCtx, req)
			}); err != nil {
				log.Error(err)
				handleServiceError(c, serviceCtx, err)
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
			param = meta.Param(util.Deref(cfg[0]).ParamName)
		}
		if len(param) == 0 {
			log.Error(CodeNotFoundRouteParam)
			JSON(c, CodeNotFoundRouteParam)
			gstotel.RecordError(span, errors.New(CodeNotFoundRouteParam.Msg()))
			return
		}
		index, _ := c.GetQuery(consts.QUERY_INDEX)
		selects, _ := c.GetQuery(consts.QUERY_SELECT)

		// The underlying type of interface types.Model must be pointer to structure, such as *model.User.
		// 'typ' is the structure type, such as: model.User.
		// 'm' is the structure value, such as: &model.User{ID: myid, Name: myname}.
		typ := reflect.TypeOf(*new(M)).Elem()
		m := reflect.New(typ).Interface().(M) //nolint:errcheck
		m.SetID(param)                        // `GetBefore` hook need id.

		var err error
		noCache := true // default disable cache.
		if noCacheStr, ok := c.GetQuery(consts.QUERY_NO_CACHE); ok {
			var parsed bool
			if parsed, err = strconv.ParseBool(noCacheStr); err == nil {
				noCache = parsed
			}
		}
		expands := parseExpandQuery(c, m)
		log.Infoz("", zap.Object(typ.Name(), m))

		// 1.Perform business logic processing before get resource.
		var serviceCtxBefore *types.ServiceContext
		if err = traceServiceHook[M](ctrlSpanCtx, consts.PHASE_GET_BEFORE, func(spanCtx context.Context) error {
			serviceCtxBefore = types.NewServiceContext(c, spanCtx, consts.PHASE_GET_BEFORE)
			return svc.GetBefore(serviceCtxBefore, m)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, serviceCtxBefore, err)
			gstotel.RecordError(span, err)
			return
		}
		// 2.Get resource from database.
		if err = handler(requestContext(c)).
			WithIndex(index).
			WithSelect(strings.Split(selects, ",")...).
			WithExpand(expands).
			WithCache(!noCache).
			Get(m, param); err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		// 3.Perform business logic processing after get resource.
		var serviceCtxAfter *types.ServiceContext
		if err = traceServiceHook[M](ctrlSpanCtx, consts.PHASE_GET_AFTER, func(spanCtx context.Context) error {
			serviceCtxAfter = types.NewServiceContext(c, spanCtx, consts.PHASE_GET_AFTER)
			return svc.GetAfter(serviceCtxAfter, m)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, serviceCtxAfter, err)
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
			Model:     typ.Name(),
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
