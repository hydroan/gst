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
	"github.com/hydroan/gst/internal/requestctx"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// Patch handles a partial update request with the default factory settings.
func Patch[M types.Model, REQ types.Request, RSP types.Response](c *gin.Context) {
	PatchFactory[M, REQ, RSP]()(c)
}

// PatchFactory returns a Gin handler that partially updates one resource.
//
// When M, REQ, and RSP are the same type, the handler reads the resource id
// from the configured route parameter (the id carried by the body is ignored),
// loads the existing record, copies fields present in the request body into
// that record, sets the updater field, runs patch hooks, writes the patched
// model through the configured database handler, and records an operation log.
//
// When REQ or RSP differs from M, the handler binds the JSON body into REQ and
// delegates the operation to the phase service's Patch method.
func PatchFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	handler, _ := extractConfig(cfg...)
	meta := newFactoryMeta[M, REQ, RSP](consts.PHASE_PATCH, consts.PHASE_PATCH_BEFORE, consts.PHASE_PATCH_AFTER)
	return func(c *gin.Context) {
		var id string

		ctrlSpanCtx, span := meta.startControllerSpan(c)
		defer span.End()

		reqMeta := requestctx.FromGin(c)
		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_PATCH)
		svc := meta.service()

		if !meta.typesEqual {
			var err error
			var reqErr error
			var rsp RSP
			req := meta.newRequest()

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
			if rsp, err = meta.traceServiceOperation(ctrlSpanCtx, consts.PHASE_PATCH, func(spanCtx context.Context) (RSP, error) {
				serviceCtx = types.NewServiceContext(c, spanCtx, consts.PHASE_PATCH)
				return svc.Patch(serviceCtx, req)
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

		req := meta.newModel()
		body, err := readJSONRequestBody(c)
		if err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		fields, err := patchFieldSetFromJSONBody(meta.typ, body)
		if err != nil && !errors.Is(err, io.EOF) {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		// The resource id comes from the configured route parameter only.
		if len(cfg) > 0 {
			id = reqMeta.Param(util.Deref(cfg[0]).ParamName)
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		if len(id) == 0 {
			log.Error(CodeNotFoundRouteParam)
			JSON(c, CodeNotFoundRouteParam)
			gstotel.RecordError(span, errors.New(CodeNotFoundRouteParam.Msg()))
			return
		}
		data := make([]M, 0)
		// 'm' is a fresh model instance, such as: &model.User{ID: myid, Name: myname}.
		m := meta.newModel()
		if !setRouteID(m, id) {
			// An id the model rejects cannot match any row; answer 404 without
			// relying on the empty-query safety net below.
			log.Errorz("route id rejected by model", zap.String("id", id))
			JSON(c, CodeNotFound)
			return
		}

		// Make sure the record must be already exists.
		if err := handler(requestContext(c)).WithLimit(1).WithQuery(m).List(&data); err != nil {
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
		// req.SetCreatedAt(data[0].GetCreatedAt())
		// req.SetCreatedBy(data[0].GetCreatedBy())
		// req.SetUpdatedBy(c.GetString(CTX_USERNAME))
		data[0].SetUpdatedBy(c.GetString(consts.CTX_USERNAME))

		newVal := reflect.ValueOf(req).Elem()
		oldVal := reflect.ValueOf(data[0]).Elem()
		patchValue(log, meta.typ, oldVal, newVal, fields)
		cur := oldVal.Addr().Interface().(M) //nolint:errcheck

		// 1.Perform business logic processing before partial update resource.
		var serviceCtxBefore *types.ServiceContext
		if err := meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_PATCH_BEFORE, func(spanCtx context.Context) error {
			serviceCtxBefore = types.NewServiceContext(c, spanCtx, consts.PHASE_PATCH_BEFORE)
			return svc.PatchBefore(serviceCtxBefore, cur)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}
		// 2.Partial update resource in database.
		if err := handler(requestContext(c)).Update(cur); err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		// 3.Perform business logic processing after partial update resource.
		var serviceCtxAfter *types.ServiceContext
		if err := meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_PATCH_AFTER, func(spanCtx context.Context) error {
			serviceCtxAfter = types.NewServiceContext(c, spanCtx, consts.PHASE_PATCH_AFTER)
			return svc.PatchAfter(serviceCtxAfter, cur)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}

		// 4.record operation log to database.
		// NOTE: We should record the `req` instead of `oldVal`, the req is `newVal`.
		record, _ := json.Marshal(req)
		reqData, _ := json.Marshal(req)
		respData, _ := json.Marshal(cur)
		// cb.Enqueue(&modellogmgmt.OperationLog{
		// 	OP:        consts.OP_PATCH,
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
		if err := am.RecordOperation(requestContext(c), req, &modellogmgmt.OperationLog{
			OP:        consts.OP_PATCH,
			Model:     meta.name,
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

		JSON(c, CodeSuccess, cur)
	}
}
