package controller

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/urlquery"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
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
// count, which is omitted only when cursor pagination is used.
//
// The automatic listing branch supports model schema fields plus framework query
// parameters for pagination, cursor pagination, expansion, depth, ordering, and
// field operator filters.
//
// When REQ or RSP differs from M, the handler delegates the operation to the
// phase service's List method with a zero-value REQ. List handles an HTTP GET
// request whose body carries no semantics, so nothing is bound into REQ;
// custom services read query parameters from ServiceContext.Query().
func ListFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	meta := newFactoryMeta[M, REQ, RSP](routeFromConfig(cfg...), consts.PHASE_LIST, consts.PHASE_LIST_BEFORE, consts.PHASE_LIST_AFTER)
	return func(c *gin.Context) {
		ctrlSpanCtx, span := meta.startControllerSpan(c)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_LIST)
		svc := meta.service()

		if !meta.typesEqual {
			var err error
			var rsp RSP
			req := meta.newRequest()

			var serviceCtx *types.ServiceContext
			if rsp, err = meta.traceServiceOperation(ctrlSpanCtx, consts.PHASE_LIST, func(spanCtx context.Context) (RSP, error) {
				serviceCtx = types.NewServiceContext(c, spanCtx, consts.PHASE_LIST)
				return svc.List(serviceCtx, req)
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

		// Built only after the custom-typed branch has returned: that branch
		// runs the service with a context of its own, so building this one
		// earlier would extract and clone request metadata nothing reads.
		ctx := types.NewServiceContext(c, nil, consts.PHASE_LIST)

		// The URL query is parsed once and shared by every parser below;
		// url.URL.Query re-parses the raw query string on each call.
		query := c.Request.URL.Query()

		// 'm' is a fresh model instance, such as: &model.User{ID: myid, Name: myname}.
		m := meta.newModel()

		var err error
		if err = decodeListQuery(m, query); err != nil {
			log.Errorz("parse query parameter failed", zap.Error(err))
			JSON(c, CodeInvalidParam.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		var filters []types.Filter
		if filters, err = urlquery.Filters(query, m); err != nil {
			log.Errorz("parse query parameter failed", zap.Error(err))
			JSON(c, CodeInvalidParam.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		present := urlquery.PresentFields(query)

		var orders []types.Order
		if orders, err = urlquery.Orders(query, m); err != nil {
			log.Errorz("parse query parameter failed", zap.Error(err))
			JSON(c, CodeInvalidParam.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}

		var cursor types.Cursor
		if cursor, err = urlquery.Cursor(query, m); err != nil {
			log.Errorz("parse query parameter failed", zap.Error(err))
			JSON(c, CodeInvalidParam.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}

		if err = checkCursorOrderConflict(cursor, orders); err != nil {
			log.Errorz("parse query parameter failed", zap.Error(err))
			JSON(c, CodeInvalidParam.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}

		data := make([]M, 0)
		expands := parseExpandQuery(c, m)

		// 1.Perform business logic processing before list resources.
		var serviceCtxBefore *types.ServiceContext
		if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_LIST_BEFORE, func(spanCtx context.Context) error {
			serviceCtxBefore = types.NewServiceContext(c, spanCtx, consts.PHASE_LIST_BEFORE)
			return svc.ListBefore(serviceCtxBefore, &data)
		}); err != nil {
			log.Errorz("service operation failed", zap.Error(err))
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}
		// 2.Let the service rewrite the query condition and options; the typical
		// use is row-level data scoping. Filter runs once and the result is
		// shared by List and Count below, so both see the same condition set.
		queryOpts := types.QueryOptions{
			AllowEmpty:    true,
			PresentFields: present,
			Filters:       filters,
		}
		if m, queryOpts, err = svc.Filter(ctx, m, queryOpts); err != nil {
			log.Errorz("service operation failed", zap.Error(err))
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}
		// 3.List resources from database.
		if err = database.Database[M](requestContext(c)).
			WithPagination(urlquery.Pagination(query, m)).
			WithQuery(m, queryOpts).
			WithCursor(cursor).
			WithExpand(expands, orders...).
			WithOrder(orders...).
			List(&data); err != nil {
			log.Errorz("parse query parameter failed", zap.Error(err))
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		// 4.Perform business logic processing after list resources.
		var serviceCtxAfter *types.ServiceContext
		if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_LIST_AFTER, func(spanCtx context.Context) error {
			serviceCtxAfter = types.NewServiceContext(c, spanCtx, consts.PHASE_LIST_AFTER)
			return svc.ListAfter(serviceCtxAfter, &data)
		}); err != nil {
			log.Errorz("service operation failed", zap.Error(err))
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}
		total := new(int)
		// NOTE: Total count is not provided when using cursor-based pagination.
		if !cursor.Enabled() {
			if err = database.Database[M](requestContext(c)).
				WithQuery(m, queryOpts).
				Count(total); err != nil {
				log.Errorz("database operation failed", zap.Error(err))
				JSON(c, CodeFailure.WithErr(err))
				gstotel.RecordError(span, err)
				return
			}
		}

		// 5.record operation log to database.
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
			log.Warnz("record operation log failed", zap.Error(err))
		}

		JSON(c, CodeSuccess, gin.H{
			"items": data,
			"total": *total,
		})
	}
}
