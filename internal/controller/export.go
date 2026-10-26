package controller

import (
	"context"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/filetype"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/urlquery"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
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
// honoring an explicit valid value and otherwise sniffing the produced bytes
// so the response never relies solely on the client-supplied format. Bytes
// detected as an xlsx workbook resolve to xlsx; anything else resolves to
// csv.
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

// ExportHandler returns a Gin handler that exports resources.
//
// The handler decodes query parameters into M, applies service filters, runs
// list hooks, queries the configured database handler with export-oriented
// limit and query options, delegates byte generation to the phase service's
// Export method, and writes the result as an attachment.
func ExportHandler[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), consts.Export, consts.ListBefore, consts.ListAfter)
	return func(c *gin.Context) {
		ctrlSpanCtx, span, ok := a.beginRequest(c)
		defer span.End()
		if !ok {
			return
		}

		log := logger.Controller.WithContext(c.Request.Context(), consts.Export)

		// An export lists every record the query matches, capped by its own
		// _limit alone, so the paging and cursor parameters a List of the
		// model takes are read by nothing here, a virtual resource's service
		// included, and are refused rather than ignored.
		query := requestctx.GinQuery(c)
		paging := matchQueryKeys(query, listPageQueryKey)
		paging = append(paging, matchQueryKeys(query, listSizeQueryKey)...)
		paging = append(paging, matchQueryKeys(query, listCursorQueryKeys)...)
		if len(paging) > 0 {
			err := urlquery.UnsupportedParameterError(paging)
			log.Errorz("parse query parameter failed", zap.Error(err))
			response.Error(c, invalidArgument(err))
			gstotel.RecordError(span, err)
			return
		}

		m := a.newModel()
		svc := a.service()

		// A virtual resource has no table behind it, so the controller-side
		// listing below would query a table that does not exist. Its service
		// owns the whole request — mirroring how a List action with a custom
		// result takes over — so the exporter receives no rows and parses the
		// query parameters itself.
		data := make([]M, 0)
		if !modelregistry.IsVirtual(m) {
			// QUERY_LIMIT caps the records exported, none or a value of zero or
			// less exporting every record. A value that is no integer is
			// refused rather than read as no cap.
			var limit int
			if raw := c.Query(consts.QUERY_LIMIT); raw != "" {
				var err error
				if limit, err = strconv.Atoi(raw); err != nil {
					err = errors.Newf("invalid query parameter %q: expect a numeric value, got %q", consts.QUERY_LIMIT, raw)
					log.Errorz("parse query parameter failed", zap.Error(err))
					response.Error(c, invalidArgument(err))
					gstotel.RecordError(span, err)
					return
				}
			}
			// The parsers below read the request's query without the export's
			// own QUERY_FORMAT, resolved after the export bytes exist, and
			// QUERY_LIMIT, read above: they refuse both from any other action.
			// A virtual resource's service parsing on its own gets them taken
			// off the same way (see serviceregistry's QueryModel).
			query = urlquery.WithoutExportKeys(query)

			var err error
			// A query that cannot decode is a client error, same as on the List
			// path: rejecting it keeps the export from running with a silently
			// dropped condition set.
			if err = urlquery.Decode(query, m); err != nil {
				log.Errorz("parse query parameter failed", zap.Error(err))
				response.Error(c, invalidArgument(err))
				gstotel.RecordError(span, err)
				return
			}
			var filters []types.Filter
			if filters, err = urlquery.Filters(query, m); err != nil {
				log.Errorz("parse query parameter failed", zap.Error(err))
				response.Error(c, invalidArgument(err))
				gstotel.RecordError(span, err)
				return
			}
			present := urlquery.PresentFields(query)

			var orders []types.Order
			if orders, err = urlquery.Orders(query, m); err != nil {
				log.Errorz("parse query parameter failed", zap.Error(err))
				response.Error(c, invalidArgument(err))
				gstotel.RecordError(span, err)
				return
			}

			expands := parseExpandQuery(query, m)
			svcCtx := types.NewServiceContext(c, nil, consts.Export)
			// The list hooks of an export run on contexts of the Export phase,
			// the phase of the action they serve.
			exportServiceContext := func(ctx context.Context, _ consts.Phase) *types.ServiceContext {
				return types.NewServiceContext(c, ctx, consts.Export)
			}
			// 1.Perform business logic processing before list resources.
			if err = a.traceServiceHook(ctrlSpanCtx, consts.ListBefore, svc, exportServiceContext, func(sc *types.ServiceContext) error {
				return svc.ListBefore(sc, &data)
			}); err != nil {
				response.Error(c, failService(ctrlSpanCtx, log, err))
				return
			}
			// 2.Let the service rewrite the query condition and options; the
			// typical use is row-level data scoping, sharing the exact List
			// semantics so an export can never see rows the list hides.
			queryOpts := types.QueryOptions{
				AllowEmpty:    true,
				PresentFields: present,
				Filters:       filters,
			}
			if m, queryOpts, err = svc.Filter(svcCtx, m, queryOpts); err != nil {
				response.Error(c, failService(ctrlSpanCtx, log, err))
				return
			}
			// 3.List resources from database. An export is bounded by the limit
			// alone and never paginated.
			if err = database.Database[M](requestContext(c)).
				WithLimit(limit).
				WithQuery(m, queryOpts).
				WithExpand(expands, orders...).
				WithOrder(orders...).
				List(&data); err != nil {
				response.Error(c, failDatabase(ctrlSpanCtx, log, err))
				return
			}
			// 4.Perform business logic processing after list resources.
			if err = a.traceServiceHook(ctrlSpanCtx, consts.ListAfter, svc, exportServiceContext, func(sc *types.ServiceContext) error {
				return svc.ListAfter(sc, &data)
			}); err != nil {
				response.Error(c, failService(ctrlSpanCtx, log, err))
				return
			}
		}
		// 5.Export
		exported, err := traceServiceCall(ctrlSpanCtx, a.serviceSpan(consts.Export), a.name, func(spanCtx context.Context) ([]byte, error) {
			return svc.Export(types.NewServiceContext(c, spanCtx, consts.Export), data...)
		})
		if err != nil {
			response.Error(c, failService(ctrlSpanCtx, log, err))
			return
		}
		format := resolveExportFormat(c.Query(consts.QUERY_FORMAT), exported)
		filename, contentType := exportAttachment(format)
		response.Attachment(c, exported, filename, contentType)
	}
}
