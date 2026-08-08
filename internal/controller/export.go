package controller

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/modelregistry"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/urlquery"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/hydroan/gst/pkg/filetype"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

// Export format identifiers accepted via the QUERY_FORMAT query parameter.
const (
	exportFormatXLSX = "xlsx"
	exportFormatCSV  = "csv"
)

// Download file names and MIME types for each export format.
const (
	exportFileXLSX = "exported.xlsx"
	exportFileCSV  = "exported.csv"

	exportMIMEXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	exportMIMECSV  = "text/csv; charset=utf-8"
)

// resolveExportFormat decides the export format from the query parameter,
// honoring an explicit valid value and otherwise sniffing the produced bytes so
// the response never relies solely on the client-supplied format. Bytes detected
// as an xlsx workbook resolve to xlsx; anything else resolves to csv.
func resolveExportFormat(queryFormat string, data []byte) string {
	switch queryFormat {
	case exportFormatXLSX, exportFormatCSV:
		return queryFormat
	}
	if ft, _ := filetype.DetectBytes(data); ft == filetype.FiletypeXLSX {
		return exportFormatXLSX
	}
	return exportFormatCSV
}

// exportAttachment returns the download file name and MIME type for the given
// export format, defaulting to xlsx for empty or unknown formats.
func exportAttachment(format string) (filename, contentType string) {
	switch format {
	case exportFormatCSV:
		return exportFileCSV, exportMIMECSV
	default:
		return exportFileXLSX, exportMIMEXLSX
	}
}

// Export handles an export request with the default factory settings.
func Export[M types.Model, REQ types.Request, RSP types.Response](c *gin.Context) {
	ExportFactory[M, REQ, RSP]()(c)
}

// ExportFactory returns a Gin handler that exports resources.
//
// The handler decodes query parameters into M, applies service filters, runs
// list hooks, queries the configured database handler with export-oriented limit
// and query options, delegates byte generation to the phase service's Export
// method, and writes the result as an attachment
func ExportFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	meta := newFactoryMeta[M, REQ, RSP](routeFromConfig(cfg...), consts.PHASE_EXPORT)
	return func(c *gin.Context) {
		ctrlSpanCtx, span := meta.startControllerSpan(c)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_EXPORT)

		// 'm' is a fresh model instance, such as: &model.User{ID: myid, Name: myname}.
		m := meta.newModel()
		svc := meta.service()

		// A virtual resource has no table behind it, so the controller-side
		// listing below would query a table that does not exist. Its service
		// owns the whole request — mirroring how a List action with a custom
		// result takes over — so the exporter receives no rows and parses the
		// query parameters itself.
		data := make([]M, 0)
		if !modelregistry.IsVirtual(m) {
			var page, size, limit int
			if pageStr, ok := c.GetQuery(consts.QUERY_PAGE); ok {
				page, _ = strconv.Atoi(pageStr)
			}
			if sizeStr, ok := c.GetQuery(consts.QUERY_SIZE); ok {
				size, _ = strconv.Atoi(sizeStr)
			}
			if limitStr, ok := c.GetQuery(consts.QUERY_LIMIT); ok {
				limit, _ = strconv.Atoi(limitStr)
			}
			// The URL query is parsed once and shared by every parser below;
			// url.URL.Query re-parses the raw query string on each call.
			query := c.Request.URL.Query()

			var err error
			// A query that cannot decode is a client error, same as on the List
			// path: rejecting it keeps the export from running with a silently
			// dropped condition set.
			if err = urlquery.Decode(query, m); err != nil {
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

			expands := parseExpandQuery(c, m)
			svcCtx := types.NewServiceContext(c, nil, consts.PHASE_EXPORT)
			// 1.Perform business logic processing before list resources.
			if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_EXPORT, func(spanCtx context.Context) error {
				return svc.ListBefore(types.NewServiceContext(c, spanCtx, consts.PHASE_EXPORT), &data)
			}); err != nil {
				log.Errorz("service operation failed", zap.Error(err))
				JSON(c, CodeFailure.WithErr(err))
				gstotel.RecordError(span, err)
				return
			}
			_, _ = page, size
			// 2.Let the service rewrite the query condition and options; the
			// typical use is row-level data scoping, sharing the exact List
			// semantics so an export can never see rows the list hides.
			queryOpts := types.QueryOptions{
				AllowEmpty:    true,
				PresentFields: present,
				Filters:       filters,
			}
			if m, queryOpts, err = svc.Filter(svcCtx, m, queryOpts); err != nil {
				log.Errorz("service operation failed", zap.Error(err))
				handleServiceError(c, err)
				gstotel.RecordError(span, err)
				return
			}
			// 3.List resources from database.
			if err = database.Database[M](requestContext(c)).
				// WithPagination(page, size). // don't use WithPagination, it makes WithLimit ineffective
				WithLimit(limit).
				WithQuery(m, queryOpts).
				WithExclude(m.Excludes()).
				WithExpand(expands, orders...).
				WithOrder(orders...).
				List(&data); err != nil {
				log.Errorz("database operation failed", zap.Error(err))
				JSON(c, CodeFailure.WithErr(err))
				gstotel.RecordError(span, err)
				return
			}
			// 4.Perform business logic processing after list resources.
			if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_EXPORT, func(spanCtx context.Context) error {
				return svc.ListAfter(types.NewServiceContext(c, spanCtx, consts.PHASE_EXPORT), &data)
			}); err != nil {
				log.Errorz("service operation failed", zap.Error(err))
				JSON(c, CodeFailure.WithErr(err))
				gstotel.RecordError(span, err)
				return
			}
		}
		// 5.Export
		exported, err := meta.traceServiceExport(ctrlSpanCtx, consts.PHASE_EXPORT, func(spanCtx context.Context) ([]byte, error) {
			return svc.Export(types.NewServiceContext(c, spanCtx, consts.PHASE_EXPORT), data...)
		})
		if err != nil {
			log.Errorz("service operation failed", zap.Error(err))
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		// // 5.record operation log to database.
		// var tableName string
		// items := strings.Split(typ.Name(), ".")
		// if len(items) > 0 {
		// 	tableName = pluralizeCli.Plural(strings.ToLower(items[len(items)-1]))
		// }
		// record, _ := json.Marshal(data)
		// if err := database.Database[*model.OperationLog]().Create(&model.OperationLog{
		// 	Op:        model.OperationTypeExport,
		// 	Model:     typ.Name(),
		// 	Table:     tableName,
		// 	Record:    util.BytesToString(record),
		// 	IP:        c.ClientIP(),
		// 	User:      c.GetString(consts.CTX_USERNAME),
		// 	TraceID: c.GetString(consts.TRACE_ID),
		// 	URI:       c.Request.RequestURI,
		// 	Method:    c.Request.Method,
		// 	UserAgent: c.Request.UserAgent(),
		// }); err != nil {
		// 	log.Error("failed to write operation log to database: ", err.Error())
		// }
		format := resolveExportFormat(c.Query(consts.QUERY_FORMAT), exported)
		filename, contentType := exportAttachment(format)
		Attachment(c, exported, filename, contentType)
	}
}
