package controller

import (
	"context"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/pkg/filetype"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
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
	handler, _ := extractConfig(cfg...)
	meta := newFactoryMeta[M, REQ, RSP](routeFromConfig(cfg...), consts.PHASE_EXPORT)
	return func(c *gin.Context) {
		ctrlSpanCtx, span := meta.startControllerSpan(c)
		defer span.End()

		var page, size, limit int
		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_EXPORT)
		if pageStr, ok := c.GetQuery(consts.QUERY_PAGE); ok {
			page, _ = strconv.Atoi(pageStr)
		}
		if sizeStr, ok := c.GetQuery(consts.QUERY_SIZE); ok {
			size, _ = strconv.Atoi(sizeStr)
		}
		if limitStr, ok := c.GetQuery(consts.QUERY_LIMIT); ok {
			limit, _ = strconv.Atoi(limitStr)
		}
		index, _ := c.GetQuery(consts.QUERY_INDEX)
		selects, _ := c.GetQuery(consts.QUERY_SELECT)

		// 'm' is a fresh model instance, such as: &model.User{ID: myid, Name: myname}.
		m := meta.newModel()

		var err error
		if err = serviceregistry.QueryDecoder().Decode(m, stripFieldConditionKeys(c.Request.URL.Query())); err != nil {
			log.Warn("failed to parse uri query parameter into model: ", err)
		}
		var fieldConditions []types.FieldCondition
		if fieldConditions, err = parseFieldConditionsQuery(m, c.Request.URL.Query()); err != nil {
			log.Error(err)
			JSON(c, CodeInvalidParam.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		log.Info("query parameter: ", m)
		present := presentQueryFields(c.Request.URL.Query())

		var or bool
		data := make([]M, 0)
		if orStr, ok := c.GetQuery(consts.QUERY_OR); ok {
			or, _ = strconv.ParseBool(orStr)
		}
		expands := parseExpandQuery(c, m)

		svc := meta.service()
		svcCtx := types.NewServiceContext(c, nil, consts.PHASE_EXPORT)
		// 1.Perform business logic processing before list resources.
		if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_EXPORT, func(spanCtx context.Context) error {
			return svc.ListBefore(types.NewServiceContext(c, spanCtx, consts.PHASE_EXPORT), &data)
		}); err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		sortBy, _ := c.GetQuery(consts.QUERY_SORT_BY)
		_, _ = page, size
		// 2.List resources from database.
		if err = handler(requestContext(c)).
			// WithPagination(page, size). // 不要使用 WithPagination, 否则 WithLimit 不生效
			WithLimit(limit).
			WithIndex(index).
			WithSelect(strings.Split(selects, ",")...).
			WithQuery(svc.Filter(svcCtx, m), types.QueryConfig{
				AllowEmpty:      true,
				UseOr:           or,
				RawQuery:        svc.FilterRaw(svcCtx),
				PresentFields:   present,
				FieldConditions: fieldConditions,
			}).
			WithExclude(m.Excludes()).
			WithExpand(expands, sortBy).
			WithOrder(sortBy).
			List(&data); err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		// 3.Perform business logic processing after list resources.
		if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_EXPORT, func(spanCtx context.Context) error {
			return svc.ListAfter(types.NewServiceContext(c, spanCtx, consts.PHASE_EXPORT), &data)
		}); err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		log.Info("export data length: ", len(data))
		// 4.Export
		exported, err := meta.traceServiceExport(ctrlSpanCtx, consts.PHASE_EXPORT, func(spanCtx context.Context) ([]byte, error) {
			return svc.Export(types.NewServiceContext(c, spanCtx, consts.PHASE_EXPORT), data...)
		})
		if err != nil {
			log.Error(err)
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
		// if err := database.Database[*model.OperationLog]().WithDB(db).Create(&model.OperationLog{
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
