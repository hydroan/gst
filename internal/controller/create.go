package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"

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
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// Create is a generic function to product gin handler to create one resource.
// The resource type depends on the type of interface types.Model.
func Create[M types.Model, REQ types.Request, RSP types.Response](c *gin.Context) {
	CreateFactory[M, REQ, RSP]()(c)
}

// CreateFactory returns a Gin handler that creates one resource.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// M, fills the creator/updater fields, runs the create hooks, writes the model
// through the configured database handler, records an operation log, and returns
// the created model with a created response status.
//
// When REQ or RSP differs from M, the handler binds the JSON body into REQ and
// delegates the operation to the phase service's Create method. Multipart form
// requests are left unbound so the service can read the request directly.
func CreateFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	handler, _ := extractConfig(cfg...)
	return func(c *gin.Context) {
		var err error
		var reqErr error

		ctrlSpanCtx, span := startControllerSpan[M](c, consts.PHASE_CREATE)
		defer span.End()

		log := logger.Controller.WithControllerContext(types.NewControllerContext(c), consts.PHASE_CREATE)
		svc := serviceregistry.Resolve[M, REQ, RSP](consts.PHASE_CREATE)

		if !modelregistry.AreTypesEqual[M, REQ, RSP]() {
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

			// If the request content type if "multipart/form-data", then the request body is a file.
			// We should not try to parse it as JSON.
			if !strings.EqualFold(c.ContentType(), "multipart/form-data") {
				if reqErr = c.ShouldBindJSON(&req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
					log.Error(reqErr)
					JSON(c, CodeInvalidParam.WithErr(reqErr))
					gstotel.RecordError(span, reqErr)
					return
				}
			}
			if errors.Is(reqErr, io.EOF) {
				log.Warn(ErrRequestBodyEmpty)
			}
			var serviceCtx *types.ServiceContext
			if rsp, err = traceServiceOperation[M, RSP](ctrlSpanCtx, consts.PHASE_CREATE, func(spanCtx context.Context) (RSP, error) {
				serviceCtx = types.NewServiceContext(c, spanCtx).WithPhase(consts.PHASE_CREATE)
				return svc.Create(serviceCtx, req)
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

		typ := reflect.TypeOf(*new(M)).Elem()
		req := reflect.New(typ).Interface().(M) //nolint:errcheck
		if reqErr = c.ShouldBindJSON(&req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
			log.Error(reqErr)
			JSON(c, CodeInvalidParam.WithErr(reqErr))
			gstotel.RecordError(span, reqErr)
			return
		}
		if errors.Is(reqErr, io.EOF) {
			log.Warn(ErrRequestBodyEmpty)
		} else {
			req.SetCreatedBy(c.GetString(consts.CTX_USERNAME))
			req.SetUpdatedBy(c.GetString(consts.CTX_USERNAME))
			log.Infoz("create", zap.Object(reflect.TypeOf(*new(M)).Elem().String(), req))
		}

		// 1.Perform business logic processing before create resource.
		var serviceCtxBefore *types.ServiceContext
		if err = traceServiceHook[M](ctrlSpanCtx, consts.PHASE_CREATE_BEFORE, func(spanCtx context.Context) error {
			serviceCtxBefore = types.NewServiceContext(c, spanCtx).WithPhase(consts.PHASE_CREATE_BEFORE)
			return svc.CreateBefore(serviceCtxBefore, req)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, serviceCtxBefore, err)
			gstotel.RecordError(span, err)
			return
		}
		// 2.Create resource in database.
		// database.Database().Delete just set "deleted_at" field to current time, not really delete.
		// We should update it instead of creating it, and update the "created_at" and "updated_at" field.
		// NOTE: WithExpand(req.Expands()...) is not a good choices.
		// if err := database.Database[M]().WithExpand(req.Expands()...).Update(req); err != nil {
		if !errors.Is(reqErr, io.EOF) {
			if err = handler(types.NewDatabaseContext(c)).WithExpand(req.Expands()).Create(req); err != nil {
				log.Error(err)
				JSON(c, CodeFailure.WithErr(err))
				gstotel.RecordError(span, err)
				return
			}
		}
		// 3.Perform business logic processing after create resource
		var serviceCtxAfter *types.ServiceContext
		if err = traceServiceHook[M](ctrlSpanCtx, consts.PHASE_CREATE_AFTER, func(spanCtx context.Context) error {
			serviceCtxAfter = types.NewServiceContext(c, spanCtx).WithPhase(consts.PHASE_CREATE_AFTER)
			return svc.CreateAfter(serviceCtxAfter, req)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, serviceCtxAfter, err)
			gstotel.RecordError(span, err)
			return
		}

		// 4.record operation log to database.
		record, _ := json.Marshal(req)
		reqData, _ := json.Marshal(req)
		respData, _ := json.Marshal(req)
		// cb.Enqueue(&modellogmgmt.OperationLog{
		// 	OP:        consts.OP_CREATE,
		// 	Model:     typ.Name(),
		// 	Table:     tableName,
		// 	RecordID:  req.GetID(),
		// 	Record:    util.BytesToString(record),
		// 	Request:   util.BytesToString(reqData),
		// 	Response:  util.BytesToString(respData),
		// 	IP:        c.ClientIP(),
		// 	User:      c.GetString(consts.CTX_USERNAME),
		// 	TraceID: c.GetString(consts.TRACE_ID),
		// 	URI:       c.Request.RequestURI,
		// 	Method:    c.Request.Method,
		// 	UserAgent: c.Request.UserAgent(),
		// })
		if err = am.RecordOperation(types.NewDatabaseContext(c), req, &modellogmgmt.OperationLog{
			OP:        consts.OP_CREATE,
			Model:     typ.Name(),
			RecordID:  req.GetID(),
			Record:    util.BytesToString(record),
			Request:   util.BytesToString(reqData),
			Response:  util.BytesToString(respData),
			IP:        c.ClientIP(),
			User:      c.GetString(consts.CTX_USERNAME),
			TraceID:   c.GetString(consts.TRACE_ID),
			URI:       c.Request.RequestURI,
			Method:    c.Request.Method,
			UserAgent: c.Request.UserAgent(),
		}); err != nil {
			log.Warn(err)
		}

		JSON(c, CodeSuccess.WithStatus(http.StatusCreated), req)
	}
}
