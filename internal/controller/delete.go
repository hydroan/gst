package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"

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

// Delete handles a delete request with the default factory settings.
func Delete[M types.Model, REQ types.Request, RSP types.Response](c *gin.Context) {
	DeleteFactory[M, REQ, RSP]()(c)
}

// DeleteFactory returns a Gin handler that deletes one resource.
//
// When M, REQ, and RSP are the same type, the handler reads the resource id
// from the configured route parameter (batch deletion uses the DeleteMany
// action instead), runs delete hooks, deletes the model through the
// configured database handler, records an operation log, and returns a
// success response.
//
// When REQ or RSP differs from M, the handler binds the JSON body into REQ and
// delegates the operation to the phase service's Delete method.
func DeleteFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	handler, _ := extractConfig(cfg...)
	return func(c *gin.Context) {
		ctrlSpanCtx, span := startControllerSpan[M](c, consts.PHASE_DELETE)
		defer span.End()

		meta := requestctx.FromGin(c)
		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_DELETE)
		svc := serviceregistry.Resolve[M, REQ, RSP](consts.PHASE_DELETE)

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

			if reqErr := c.ShouldBindJSON(&req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
				log.Error(reqErr)
				JSON(c, CodeInvalidParam.WithErr(reqErr))
				gstotel.RecordError(span, reqErr)
				return
			}
			var serviceCtx *types.ServiceContext
			if rsp, err = traceServiceOperation[M, RSP](ctrlSpanCtx, consts.PHASE_DELETE, func(spanCtx context.Context) (RSP, error) {
				serviceCtx = types.NewServiceContext(c, spanCtx, consts.PHASE_DELETE)
				return svc.Delete(serviceCtx, req)
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

		// The underlying type of interface types.Model must be pointer to structure, such as *model.User.
		// 'typ' is the structure type, such as: model.User.
		typ := reflect.TypeOf(*new(M)).Elem()

		// The resource id comes from the configured route parameter only.
		var id string
		if len(cfg) > 0 {
			id = meta.Param(util.Deref(cfg[0]).ParamName)
		}
		if len(id) == 0 {
			log.Error(CodeNotFoundRouteParam)
			JSON(c, CodeNotFoundRouteParam)
			gstotel.RecordError(span, errors.New(CodeNotFoundRouteParam.Msg()))
			return
		}
		// 'm' is the structure value such as: &model.User{ID: myid, Name: myname}.
		m := reflect.New(typ).Interface().(M) //nolint:errcheck
		if !setRouteID(m, id) {
			// An id the model rejects cannot match any row; answer 404 instead
			// of passing an unset id to the database layer.
			log.Errorz("route id rejected by model", zap.String("id", id))
			JSON(c, CodeNotFound)
			return
		}
		log.Info(fmt.Sprintf("%s delete %s", typ.Name(), id))

		// 1.Perform business logic processing before delete resource.
		var serviceCtxBefore *types.ServiceContext
		if err := traceServiceHook[M](ctrlSpanCtx, consts.PHASE_DELETE_BEFORE, func(spanCtx context.Context) error {
			serviceCtxBefore = types.NewServiceContext(c, spanCtx, consts.PHASE_DELETE_BEFORE)
			return svc.DeleteBefore(serviceCtxBefore, m)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}

		// find out the record and keep a copy for the operation log.
		copied := reflect.New(typ).Interface().(M) //nolint:errcheck
		copied.SetID(m.GetID())
		if err := handler(requestContext(c)).WithExpand(copied.Expands()).Get(copied, m.GetID()); err != nil {
			log.Error(err)
			gstotel.RecordError(span, err)
		}

		// 2.Delete resource in database.
		if err := handler(requestContext(c)).Delete(m); err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		// 3.Perform business logic processing after delete resource.
		var serviceCtxAfter *types.ServiceContext
		if err := traceServiceHook[M](ctrlSpanCtx, consts.PHASE_DELETE_AFTER, func(spanCtx context.Context) error {
			serviceCtxAfter = types.NewServiceContext(c, spanCtx, consts.PHASE_DELETE_AFTER)
			return svc.DeleteAfter(serviceCtxAfter, m)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}

		// 4.record operation log to database.
		record, _ := json.Marshal(copied)
		if err := am.RecordOperation(requestContext(c), reflect.New(typ).Interface().(M), &modellogmgmt.OperationLog{ //nolint:errcheck
			OP:        consts.OP_DELETE,
			Model:     typ.Name(),
			RecordID:  m.GetID(),
			Record:    util.BytesToString(record),
			IP:        c.ClientIP(),
			User:      c.GetString(consts.CTX_USERNAME),
			TraceID:   c.GetString(consts.TRACE_ID),
			URI:       c.Request.RequestURI,
			Method:    c.Request.Method,
			UserAgent: c.Request.UserAgent(),
		}); err != nil {
			log.Warn(err)
		}

		JSON(c, CodeSuccess)
	}
}
