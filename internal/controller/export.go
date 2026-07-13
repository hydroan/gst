package controller

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

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
	return func(c *gin.Context) {
		ctrlSpanCtx, span := startControllerSpan[M](c, consts.PHASE_EXPORT)
		defer span.End()

		var page, size, limit int
		var startTime, endTime time.Time
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
		timeColumn, _ := c.GetQuery(consts.QUERY_TIME_COLUMN)
		index, _ := c.GetQuery(consts.QUERY_INDEX)
		selects, _ := c.GetQuery(consts.QUERY_SELECT)
		if startTimeStr, ok := c.GetQuery(consts.QUERY_START_TIME); ok {
			startTime, _ = time.ParseInLocation(consts.DATE_TIME_LAYOUT, startTimeStr, time.Local)
		}
		if endTimeStr, ok := c.GetQuery(consts.QUERY_END_TIME); ok {
			endTime, _ = time.ParseInLocation(consts.DATE_TIME_LAYOUT, endTimeStr, time.Local)
		}

		// The underlying type of interface types.Model must be pointer to structure, such as *model.User.
		// 'typ' is the structure type, such as: model.User.
		// 'm' is the structure value, such as: &model.User{ID: myid, Name: myname}.
		typ := reflect.TypeOf(*new(M)).Elem() // the real underlying structure type
		m := reflect.New(typ).Interface().(M) //nolint:errcheck

		if err := serviceregistry.QueryDecoder().Decode(m, c.Request.URL.Query()); err != nil {
			log.Warn("failed to parse uri query parameter into model: ", err)
		}
		log.Info("query parameter: ", m)

		var err error
		var or bool
		var fuzzy bool
		depth := 1
		var expands []string
		data := make([]M, 0)
		if orStr, ok := c.GetQuery(consts.QUERY_OR); ok {
			or, _ = strconv.ParseBool(orStr)
		}
		if fuzzyStr, ok := c.GetQuery(consts.QUERY_FUZZY); ok {
			fuzzy, _ = strconv.ParseBool(fuzzyStr)
		}
		if depthStr, ok := c.GetQuery(consts.QUERY_DEPTH); ok {
			depth, _ = strconv.Atoi(depthStr)
			if depth < 1 || depth > 99 {
				depth = 1
			}
		}
		if expandStr, ok := c.GetQuery(consts.QUERY_EXPAND); ok {
			var _expands []string
			items := strings.Split(expandStr, ",")
			if len(items) > 0 {
				if items[0] == consts.VALUE_ALL { // expand all feilds
					items = m.Expands()
				}
			}
			for _, e := range m.Expands() {
				for _, item := range items {
					if strings.EqualFold(item, e) {
						_expands = append(_expands, e)
					}
				}
			}
			// fmt.Println("_expends: ", _expands)
			fieldsMap := make(map[string]reflect.Kind)
			for field := range typ.Fields() {
				fieldsMap[field.Name] = field.Type.Kind()
			}
			for _, e := range _expands {
				// If the expanding field not exists in the structure fiedls, skip depth expand.
				kind, found := fieldsMap[e]
				if !found {
					expands = append(expands, e)
					continue
				}
				// If the expanding field exists in the structure but the kind is not slice, skip depth expand.
				if kind != reflect.Slice {
					expands = append(expands, e)
					continue
				}
				t := make([]string, depth)
				for i := range depth {
					t[i] = e
				}
				// fmt.Println("t: ", t)
				// If expand="Children" and depth=3, the depth expanded is "Children.Children.Children"
				expands = append(expands, strings.Join(t, "."))
			}
			// fmt.Println("expands: ", expands)
		}

		svc := serviceregistry.Resolve[M, REQ, RSP](consts.PHASE_EXPORT)
		svcCtx := types.NewServiceContext(c, nil, consts.PHASE_EXPORT)
		// 1.Perform business logic processing before list resources.
		if err = traceServiceHook[M](ctrlSpanCtx, consts.PHASE_EXPORT, func(spanCtx context.Context) error {
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
				FuzzyMatch: fuzzy,
				AllowEmpty: true,
				UseOr:      or,
				RawQuery:   svc.FilterRaw(svcCtx),
			}).
			WithExclude(m.Excludes()).
			WithExpand(expands, sortBy).
			WithOrder(sortBy).
			WithTimeRange(timeColumn, startTime, endTime).
			List(&data); err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		// 3.Perform business logic processing after list resources.
		if err = traceServiceHook[M](ctrlSpanCtx, consts.PHASE_EXPORT, func(spanCtx context.Context) error {
			return svc.ListAfter(types.NewServiceContext(c, spanCtx, consts.PHASE_EXPORT), &data)
		}); err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		log.Info("export data length: ", len(data))
		// 4.Export
		exported, err := traceServiceExport[M](ctrlSpanCtx, consts.PHASE_EXPORT, func(spanCtx context.Context) ([]byte, error) {
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
		Data(c, exported, map[string]string{
			"Content-Disposition": "attachment; filename=exported.xlsx",
		})
	}
}
