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
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// Update is a generic function to product gin handler to update one resource.
// The resource type depends on the type of interface types.Model.
//
// Update will update one resource and resource "ID" must be specified,
// which can be specify in "router parameter `id`" or "http body data".
//
// "router parameter `id`" has more priority than "http body data".
// It will skip decode id from "http body data" if "router parameter `id`" not empty.
func Update[M types.Model, REQ types.Request, RSP types.Response](c *gin.Context) {
	UpdateFactory[M, REQ, RSP]()(c)
}

// UpdateFactory returns a Gin handler that replaces one resource.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// M, uses the configured route id before the body id, verifies that exactly one
// existing record matches, preserves the original creator fields, sets the
// updater field, runs update hooks, writes the replacement through the
// configured database handler, and records an operation log.
//
// When REQ or RSP differs from M, the handler binds the JSON body into REQ and
// delegates the operation to the phase service's Update method.
func UpdateFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	handler, _ := extractConfig(cfg...)
	return func(c *gin.Context) {
		var err error
		var reqErr error

		ctrlSpanCtx, span := startControllerSpan[M](c, consts.PHASE_UPDATE)
		defer span.End()

		meta := types.RequestMetadataFromGin(c)
		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_UPDATE)
		svc := serviceregistry.Resolve[M, REQ, RSP](consts.PHASE_UPDATE)

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

			if reqErr = c.ShouldBindJSON(&req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
				log.Error(reqErr)
				JSON(c, CodeInvalidParam.WithErr(reqErr))
				gstotel.RecordError(span, reqErr)
				return
			}
			if errors.Is(reqErr, io.EOF) {
				log.Warn(ErrRequestBodyEmpty)
			}
			var serviceCtx *types.ServiceContext
			if rsp, err = traceServiceOperation[M, RSP](ctrlSpanCtx, consts.PHASE_UPDATE, func(spanCtx context.Context) (RSP, error) {
				serviceCtx = types.NewServiceContext(c, spanCtx, consts.PHASE_UPDATE)
				return svc.Update(serviceCtx, req)
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
		if reqErr := c.ShouldBindJSON(&req); reqErr != nil {
			log.Error(reqErr)
			JSON(c, CodeInvalidParam.WithErr(reqErr))
			gstotel.RecordError(span, reqErr)
			return
		}

		// param id has more priority than http body data id
		var paramID string
		if len(cfg) > 0 {
			paramID = meta.Param(util.Deref(cfg[0]).ParamName)
		}
		bodyID := req.GetID()
		var id string
		log.Infoz(
			"update from request",
			zap.String("param_id", paramID),
			zap.String("body_id", bodyID),
			zap.Object(reflect.TypeOf(*new(M)).Elem().String(), req),
		)
		if paramID != "" {
			req.SetID(paramID)
			id = paramID
		} else if bodyID != "" {
			paramID = bodyID //nolint:ineffassign,wastedassign
			id = bodyID
		} else {
			log.Error("id missing")
			JSON(c, CodeFailure.WithErr(errors.New("id missing")))
			gstotel.RecordError(span, err)
			return
		}

		data := make([]M, 0)
		// The underlying type of interface types.Model must be pointer to structure, such as *model.User.
		// 'typ' is the structure type, such as: model.User.
		// 'm' is the structure value such as: &model.User{ID: myid, Name: myname}.
		m := reflect.New(typ).Interface().(M) //nolint:errcheck
		m.SetID(id)
		// Make sure the record must be already exists.
		if err = handler(requestContext(c)).WithLimit(1).WithQuery(m).List(&data); err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		if len(data) != 1 {
			log.Errorz(fmt.Sprintf("the total number of records query from database not equal to 1(%d)", len(data)), zap.String("id", id))
			JSON(c, CodeNotFound)
			return
		}

		req.SetCreatedAt(data[0].GetCreatedAt())           // keep original "created_at"
		req.SetCreatedBy(data[0].GetCreatedBy())           // keep original "created_by"
		req.SetUpdatedBy(c.GetString(consts.CTX_USERNAME)) // set updated_by to current user”

		// 1.Perform business logic processing before update resource.
		var serviceCtxBefore *types.ServiceContext
		if err = traceServiceHook[M](ctrlSpanCtx, consts.PHASE_UPDATE_BEFORE, func(spanCtx context.Context) error {
			serviceCtxBefore = types.NewServiceContext(c, spanCtx, consts.PHASE_UPDATE_BEFORE)
			return svc.UpdateBefore(serviceCtxBefore, req)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, serviceCtxBefore, err)
			gstotel.RecordError(span, err)
			return
		}
		// 2.Update resource in database.
		log.Infoz("update in database", zap.Object(typ.Name(), req))
		if err = handler(requestContext(c)).Update(req); err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		// 3.Perform business logic processing after update resource.
		var serviceCtxAfter *types.ServiceContext
		if err = traceServiceHook[M](ctrlSpanCtx, consts.PHASE_UPDATE_AFTER, func(spanCtx context.Context) error {
			serviceCtxAfter = types.NewServiceContext(c, spanCtx, consts.PHASE_UPDATE_AFTER)
			return svc.UpdateAfter(serviceCtxAfter, req)
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
		// 	OP:        consts.OP_UPDATE,
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
		if err = am.RecordOperation(requestContext(c), req, &modellogmgmt.OperationLog{
			OP:        consts.OP_UPDATE,
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

		JSON(c, CodeSuccess, req)
	}
}
