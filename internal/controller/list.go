package controller

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/modelregistry"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

// listQueryKeys are List controller parameters that belong to model.Query
// rather than to the resource model's own filter fields.
var listQueryKeys = map[string]struct{}{
	consts.QUERY_EXPAND:      {},
	consts.QUERY_DEPTH:       {},
	consts.QUERY_OR:          {},
	consts.QUERY_FUZZY:       {},
	consts.QUERY_SORTBY:      {},
	consts.QUERY_COLUMN_NAME: {},
	consts.QUERY_START_TIME:  {},
	consts.QUERY_END_TIME:    {},
	consts.QUERY_NOCACHE:     {},
	consts.QUERY_NOTOTAL:     {},
	consts.QUERY_INDEX:       {},
	consts.QUERY_SELECT:      {},
}

// listPaginationQueryKeys are enabled by model.Pagination. They are split from
// listQueryKeys so a model can allow page and size without enabling fuzzy,
// sorting, expansion, or other framework-owned List controls.
var listPaginationQueryKeys = map[string]struct{}{
	consts.QUERY_PAGE: {},
	consts.QUERY_SIZE: {},
}

// listCursorQueryKeys are enabled by model.Cursor. Cursor pagination is
// intentionally independent from SortBy; the cursor field and direction define
// the stable order used by the database layer.
var listCursorQueryKeys = map[string]struct{}{
	consts.QUERY_CURSOR_VALUE:  {},
	consts.QUERY_CURSOR_FIELDS: {},
	consts.QUERY_CURSOR_NEXT:   {},
}

// List is a generic function to product gin handler to list resources in backend.
// The resource type deponds on the type of interface types.Model.
//
// If you want make a structure field as query parameter, you should add a "query"
// tag for it. for example: query:"name"
//
// TODO:combine query parameter 'page' and 'size' into decoded types.Model
// FIX: retrieve records recursive (current not support in gorm.)
// https://stackoverflow.com/questions/69395891/get-recursive-field-values-in-gorm
// DB.Preload("Category.Category.Category").Find(&Category)
// its works for me.
//
// Query parameters:
//   - All feilds of types.Model's underlying structure but excluding some special fields,
//     such as "password", field value too large, json tag is "-", etc.
//   - `_expand`: strings (multiple items separated by ",").
//     The responsed data to frontend will expanded(retrieve data from external table accoding to foreign key)
//     For examples:
//     /department/myid?_expand=children
//     /department/myid?_expand=children,parent
//   - `_depth`: strings or interger.
//     How depth to retrieve records from datab recursively, default to 1, value scope is [1,99].
//     For examples:
//     /department/myid?_expand=children&_depth=3
//     /department/myid?_expand=children,parent&_depth=10
//   - `_fuzzy`: bool
//     fuzzy match records in database, default to fase.
//     For examples:
//     /department/myid?_fuzzy=true
func List[M types.Model, REQ types.Request, RSP types.Response](c *gin.Context) {
	ListFactory[M, REQ, RSP]()(c)
}

func decodeListQuery[M types.Model](m M, query map[string][]string) error {
	if _, ok := any(m).(modelregistry.Queryable); !ok {
		if err := rejectListQueryKeys(query, listQueryKeys); err != nil {
			return err
		}
	}
	if _, ok := any(m).(modelregistry.Paginatable); !ok {
		if err := rejectListQueryKeys(query, listPaginationQueryKeys); err != nil {
			return err
		}
	}
	if _, ok := any(m).(modelregistry.Cursorable); !ok {
		if err := rejectListQueryKeys(query, listCursorQueryKeys); err != nil {
			return err
		}
	}
	return serviceregistry.QueryDecoder().Decode(m, query)
}

func rejectListQueryKeys(query map[string][]string, keys map[string]struct{}) error {
	for key := range query {
		if _, found := keys[key]; found {
			return errors.Newf("schema: invalid path %q", key)
		}
	}
	return nil
}

// ListFactory returns a Gin handler that lists resources.
//
// When M, REQ, and RSP are the same type, the handler decodes query parameters
// into M, applies service filters, runs list hooks, queries the configured
// database handler, records an operation log, and returns the items with a total
// count unless total counting is disabled or cursor pagination is used.
//
// The automatic listing branch supports model schema fields plus framework query
// parameters for pagination, cursor pagination, expansion, depth, fuzzy matching,
// OR matching, ordering, selection, cache control, database index hints, and time
// ranges.
//
// When REQ or RSP differs from M, the handler delegates the operation to the
// phase service's List method with a zero-value REQ. List handles an HTTP GET
// request whose body carries no semantics, so nothing is bound into REQ;
// custom services read query parameters from ServiceContext.Query().
func ListFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	handler, _ := extractConfig(cfg...)
	return func(c *gin.Context) {
		ctrlSpanCtx, span := startControllerSpan[M](c, consts.PHASE_LIST)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_LIST)
		svc := serviceregistry.Resolve[M, REQ, RSP](consts.PHASE_LIST)
		ctx := types.NewServiceContext(c, nil, consts.PHASE_LIST)

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
			if rsp, err = traceServiceOperation[M, RSP](ctrlSpanCtx, consts.PHASE_LIST, func(spanCtx context.Context) (RSP, error) {
				serviceCtx = types.NewServiceContext(c, spanCtx, consts.PHASE_LIST)
				return svc.List(serviceCtx, req)
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

		var page, size int
		var startTime, endTime time.Time
		if pageStr, ok := c.GetQuery(consts.QUERY_PAGE); ok {
			page, _ = strconv.Atoi(pageStr)
		}
		if sizeStr, ok := c.GetQuery(consts.QUERY_SIZE); ok {
			size, _ = strconv.Atoi(sizeStr)
		}
		columnName, _ := c.GetQuery(consts.QUERY_COLUMN_NAME)
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

		if err := decodeListQuery(m, c.Request.URL.Query()); err != nil {
			log.Error(err)
			JSON(c, CodeInvalidParam.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		log.Infoz(typ.Name()+": list query parameter", zap.Object(typ.String(), m))

		var err error
		var or bool
		var fuzzy bool
		var expands []string
		var cursorNext bool
		var nototal bool // default enable total.
		cursorValue := c.Query(consts.QUERY_CURSOR_VALUE)
		cursorFields := c.Query(consts.QUERY_CURSOR_FIELDS)
		nocache := true // default disable cache.
		depth := 1
		data := make([]M, 0)
		if nocacheStr, ok := c.GetQuery(consts.QUERY_NOCACHE); ok {
			var _nocache bool
			if _nocache, err = strconv.ParseBool(nocacheStr); err == nil {
				nocache = _nocache
			}
		}
		if orStr, ok := c.GetQuery(consts.QUERY_OR); ok {
			or, _ = strconv.ParseBool(orStr)
		}
		if fuzzyStr, ok := c.GetQuery(consts.QUERY_FUZZY); ok {
			fuzzy, _ = strconv.ParseBool(fuzzyStr)
		}
		if cursorNextStr, ok := c.GetQuery(consts.QUERY_CURSOR_NEXT); ok {
			cursorNext, _ = strconv.ParseBool(cursorNextStr)
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

		// 1.Perform business logic processing before list resources.
		var serviceCtxBefore *types.ServiceContext
		if err = traceServiceHook[M](ctrlSpanCtx, consts.PHASE_LIST_BEFORE, func(spanCtx context.Context) error {
			serviceCtxBefore = types.NewServiceContext(c, spanCtx, consts.PHASE_LIST_BEFORE)
			return svc.ListBefore(serviceCtxBefore, &data)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, serviceCtxBefore, err)
			gstotel.RecordError(span, err)
			return
		}
		sortBy, _ := c.GetQuery(consts.QUERY_SORTBY)
		// 2.List resources from database.
		if size == 0 {
			size = defaultLimit
		}
		if err = handler(requestContext(c)).
			WithPagination(page, size).
			WithIndex(index).
			WithSelect(strings.Split(selects, ",")...).
			WithQuery(svc.Filter(ctx, m), types.QueryConfig{
				FuzzyMatch: fuzzy,
				AllowEmpty: true,
				UseOr:      or,
				RawQuery:   svc.FilterRaw(ctx),
			}).
			WithCursor(cursorValue, cursorNext, cursorFields).
			WithExclude(m.Excludes()).
			WithExpand(expands, sortBy).
			WithOrder(sortBy).
			WithTimeRange(columnName, startTime, endTime).
			WithCache(!nocache).
			List(&data); err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		// 3.Perform business logic processing after list resources.
		var serviceCtxAfter *types.ServiceContext
		if err = traceServiceHook[M](ctrlSpanCtx, consts.PHASE_LIST_AFTER, func(spanCtx context.Context) error {
			serviceCtxAfter = types.NewServiceContext(c, spanCtx, consts.PHASE_LIST_AFTER)
			return svc.ListAfter(serviceCtxAfter, &data)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, serviceCtxAfter, err)
			gstotel.RecordError(span, err)
			return
		}
		total := new(int64)
		nototalStr, _ := c.GetQuery(consts.QUERY_NOTOTAL)
		nototal, _ = strconv.ParseBool(nototalStr)
		// NOTE: Total count is not provided when using cursor-based pagination.
		if !nototal && len(cursorValue) == 0 {
			if err = handler(requestContext(c)).
				// WithPagination(page, size). // NOTE: WithPagination should not apply in Count method.
				// WithSelect(strings.Split(selects, ",")...). // NOTE: WithSelect should not apply in Count method.
				WithIndex(index).
				WithQuery(svc.Filter(ctx, m), types.QueryConfig{
					FuzzyMatch: fuzzy,
					AllowEmpty: true,
					UseOr:      or,
					RawQuery:   svc.FilterRaw(ctx),
				}).
				WithExclude(m.Excludes()).
				WithTimeRange(columnName, startTime, endTime).
				WithCache(!nocache).
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

		log.Infoz(fmt.Sprintf("%s: length: %d, total: %d", typ.Name(), len(data), *total), zap.Object(typ.Name(), m))
		if !nototal {
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
