package controller

import (
	"context"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/provider/otel"
	. "github.com/hydroan/gst/response"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// Get is a generic function to product gin handler to list resource in backend.
// The resource type deponds on the type of interface types.Model.
//
// Query parameters:
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
//
// Route parameters:
// - id: string or integer.
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
// When REQ or RSP differs from M, the handler binds the JSON body into REQ and
// delegates the operation to the phase service's Get method.
func GetFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	handler, _ := extractConfig(cfg...)
	return func(c *gin.Context) {
		ctrlSpanCtx, span := startControllerSpan[M](c, consts.PHASE_GET)
		defer span.End()

		cctx := types.NewControllerContext(c)
		log := logger.Controller.WithControllerContext(cctx, consts.PHASE_GET)
		svc := service.NewFactory[M, REQ, RSP]().Service(consts.PHASE_GET)

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

			if err = c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
				log.Error(err)
				JSON(c, CodeInvalidParam.WithErr(err))
				gstotel.RecordError(span, err)
				return
			}
			var serviceCtx *types.ServiceContext
			if rsp, err = traceServiceOperation[M, RSP](ctrlSpanCtx, consts.PHASE_GET, func(spanCtx context.Context) (RSP, error) {
				serviceCtx = types.NewServiceContext(c, spanCtx).WithPhase(consts.PHASE_GET)
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
			param = cctx.Params[util.Deref(cfg[0]).ParamName]
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
		var expands []string
		nocache := true // default disable cache.
		depth := 1
		if nocacheStr, ok := c.GetQuery(consts.QUERY_NOCACHE); ok {
			var _nocache bool
			if _nocache, err = strconv.ParseBool(nocacheStr); err == nil {
				nocache = _nocache
			}
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
				// TODO: if the field type is the structure name, make depth expand.
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
				// If expand="Children" and depth=3, the depth expanded is "Children.Children.Children"
				expands = append(expands, strings.Join(t, "."))
			}
			// fmt.Println("expands: ", expands)
		}
		log.Infoz("", zap.Object(typ.Name(), m))

		// 1.Perform business logic processing before get resource.
		var serviceCtxBefore *types.ServiceContext
		if err = traceServiceHook[M](ctrlSpanCtx, consts.PHASE_GET_BEFORE, func(spanCtx context.Context) error {
			serviceCtxBefore = types.NewServiceContext(c, spanCtx).WithPhase(consts.PHASE_GET_BEFORE)
			return svc.GetBefore(serviceCtxBefore, m)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, serviceCtxBefore, err)
			gstotel.RecordError(span, err)
			return
		}
		// 2.Get resource from database.
		if err = handler(types.NewDatabaseContext(c)).
			WithIndex(index).
			WithSelect(strings.Split(selects, ",")...).
			WithExpand(expands).
			WithCache(!nocache).
			Get(m, param); err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		// 3.Perform business logic processing after get resource.
		var serviceCtxAfter *types.ServiceContext
		if err = traceServiceHook[M](ctrlSpanCtx, consts.PHASE_GET_AFTER, func(spanCtx context.Context) error {
			serviceCtxAfter = types.NewServiceContext(c, spanCtx).WithPhase(consts.PHASE_GET_AFTER)
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
		// 	RequestID: c.GetString(consts.REQUEST_ID),
		// 	URI:       c.Request.RequestURI,
		// 	Method:    c.Request.Method,
		// 	UserAgent: c.Request.UserAgent(),
		// })
		if err = am.RecordOperation(types.NewDatabaseContext(c), m, &modellogmgmt.OperationLog{
			OP:        consts.OP_GET,
			Model:     typ.Name(),
			IP:        c.ClientIP(),
			User:      c.GetString(consts.CTX_USERNAME),
			RequestID: c.GetString(consts.REQUEST_ID),
			URI:       c.Request.RequestURI,
			Method:    c.Request.Method,
			UserAgent: c.Request.UserAgent(),
		}); err != nil {
			log.Warn(err)
		}

		JSON(c, CodeSuccess, m)
	}
}
