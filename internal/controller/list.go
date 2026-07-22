package controller

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/modelregistry"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

// List handles a list request with the default factory settings.
func List[M types.Model, REQ types.Request, RSP types.Response](c *gin.Context) {
	ListFactory[M, REQ, RSP]()(c)
}

// ListFactory returns a Gin handler that lists resources.
//
// When M, REQ, and RSP are the same type, the handler decodes query parameters
// into M, applies service filters, runs list hooks, queries the configured
// database handler, records an operation log, and returns the items with a total
// count unless total counting is disabled or cursor pagination is used.
//
// The automatic listing branch supports model schema fields plus framework query
// parameters for pagination, cursor pagination, expansion, depth, ordering, and
// field operator filters; OR matching, selection, cache control, database index
// hints, and total-count suppression additionally require the model to embed
// model.UnsafeQuery.
//
// When REQ or RSP differs from M, the handler delegates the operation to the
// phase service's List method with a zero-value REQ. List handles an HTTP GET
// request whose body carries no semantics, so nothing is bound into REQ;
// custom services read query parameters from ServiceContext.Query().
func ListFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	handler, _ := extractConfig(cfg...)
	meta := newFactoryMeta[M, REQ, RSP](routeFromConfig(cfg...), consts.PHASE_LIST, consts.PHASE_LIST_BEFORE, consts.PHASE_LIST_AFTER)
	return func(c *gin.Context) {
		ctrlSpanCtx, span := meta.startControllerSpan(c)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_LIST)
		svc := meta.service()
		ctx := types.NewServiceContext(c, nil, consts.PHASE_LIST)

		if !meta.typesEqual {
			var err error
			var rsp RSP
			req := meta.newRequest()

			var serviceCtx *types.ServiceContext
			if rsp, err = meta.traceServiceOperation(ctrlSpanCtx, consts.PHASE_LIST, func(spanCtx context.Context) (RSP, error) {
				serviceCtx = types.NewServiceContext(c, spanCtx, consts.PHASE_LIST)
				return svc.List(serviceCtx, req)
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

		var page, size int
		if pageStr, ok := c.GetQuery(consts.QUERY_PAGE); ok {
			page, _ = strconv.Atoi(pageStr)
		}
		if sizeStr, ok := c.GetQuery(consts.QUERY_SIZE); ok {
			size, _ = strconv.Atoi(sizeStr)
		}
		index, _ := c.GetQuery(consts.QUERY_INDEX)
		selects, _ := c.GetQuery(consts.QUERY_SELECT)

		// 'm' is a fresh model instance, such as: &model.User{ID: myid, Name: myname}.
		m := meta.newModel()

		var err error
		if err = decodeListQuery(m, c.Request.URL.Query()); err != nil {
			log.Error(err)
			JSON(c, CodeInvalidParam.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		var fieldConditions []types.FieldCondition
		if fieldConditions, err = parseFieldConditionsQuery(m, c.Request.URL.Query()); err != nil {
			log.Error(err)
			JSON(c, CodeInvalidParam.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		log.Infoz(meta.name+": list query parameter", zap.Object(meta.fullName, m))
		present := presentQueryFields(c.Request.URL.Query())

		var or bool
		var cursorNext bool
		var noTotal bool // default enable total.
		cursorValue := c.Query(consts.QUERY_CURSOR_VALUE)
		cursorField := c.Query(consts.QUERY_CURSOR_FIELD)
		noCache := true // default disable cache.
		data := make([]M, 0)
		if noCacheStr, ok := c.GetQuery(consts.QUERY_NO_CACHE); ok {
			var parsed bool
			if parsed, err = strconv.ParseBool(noCacheStr); err == nil {
				noCache = parsed
			}
		}
		if orStr, ok := c.GetQuery(consts.QUERY_OR); ok {
			or, _ = strconv.ParseBool(orStr)
		}
		if cursorNextStr, ok := c.GetQuery(consts.QUERY_CURSOR_NEXT); ok {
			cursorNext, _ = strconv.ParseBool(cursorNextStr)
		}
		expands := parseExpandQuery(c, m)

		// 1.Perform business logic processing before list resources.
		var serviceCtxBefore *types.ServiceContext
		if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_LIST_BEFORE, func(spanCtx context.Context) error {
			serviceCtxBefore = types.NewServiceContext(c, spanCtx, consts.PHASE_LIST_BEFORE)
			return svc.ListBefore(serviceCtxBefore, &data)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}
		sortBy, _ := c.GetQuery(consts.QUERY_SORT_BY)
		// 2.List resources from database.
		sizeAdjustable := modelregistry.IsPaginatable(m) || modelregistry.IsCursorable(m)
		page, size = resolveListPagination(page, size, sizeAdjustable, len(cursorValue) > 0)
		if err = handler(requestContext(c)).
			WithPagination(page, size).
			WithIndex(index).
			WithSelect(strings.Split(selects, ",")...).
			WithQuery(svc.Filter(ctx, m), types.QueryConfig{
				AllowEmpty:      true,
				UseOr:           or,
				RawQuery:        svc.FilterRaw(ctx),
				PresentFields:   present,
				FieldConditions: fieldConditions,
			}).
			WithCursor(cursorValue, cursorNext, cursorField).
			WithExclude(m.Excludes()).
			WithExpand(expands, sortBy).
			WithOrder(sortBy).
			WithCache(!noCache).
			List(&data); err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		// 3.Perform business logic processing after list resources.
		var serviceCtxAfter *types.ServiceContext
		if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_LIST_AFTER, func(spanCtx context.Context) error {
			serviceCtxAfter = types.NewServiceContext(c, spanCtx, consts.PHASE_LIST_AFTER)
			return svc.ListAfter(serviceCtxAfter, &data)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}
		total := new(int)
		noTotalStr, _ := c.GetQuery(consts.QUERY_NO_TOTAL)
		noTotal, _ = strconv.ParseBool(noTotalStr)
		// NOTE: Total count is not provided when using cursor-based pagination.
		if !noTotal && len(cursorValue) == 0 {
			if err = handler(requestContext(c)).
				// WithPagination(page, size). // NOTE: WithPagination should not apply in Count method.
				// WithSelect(strings.Split(selects, ",")...). // NOTE: WithSelect should not apply in Count method.
				WithIndex(index).
				WithQuery(svc.Filter(ctx, m), types.QueryConfig{
					AllowEmpty:      true,
					UseOr:           or,
					RawQuery:        svc.FilterRaw(ctx),
					PresentFields:   present,
					FieldConditions: fieldConditions,
				}).
				WithExclude(m.Excludes()).
				WithCache(!noCache).
				Count(total); err != nil {
				log.Error(err)
				JSON(c, CodeFailure.WithErr(err))
				gstotel.RecordError(span, err)
				return
			}
		}

		// 4.record operation log to database.
		// cb.Enqueue(&modellogmgmt.OperationLog{
		// 	OP:        consts.OP_LIST,
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
			OP:        consts.OP_LIST,
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

		log.Infoz(fmt.Sprintf("%s: length: %d, total: %d", meta.name, len(data), *total), zap.Object(meta.name, m))
		if !noTotal {
			JSON(c, CodeSuccess, gin.H{
				"items": data,
				"total": *total,
			})
		} else {
			JSON(c, CodeSuccess, gin.H{
				"items": data,
			})
		}
	}
}
