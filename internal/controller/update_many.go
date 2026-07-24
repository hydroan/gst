package controller

import (
	"context"
	"encoding/json"
	"io"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
)

// UpdateMany handles a batch update request with the default factory settings.
func UpdateMany[M types.Model, REQ types.Request, RSP types.Response](c *gin.Context) {
	UpdateManyFactory[M, REQ, RSP]()(c)
}

// UpdateManyFactory returns a Gin handler that replaces multiple resources.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// requestData[M], runs batch update hooks, updates the items through the
// configured database handler, records an operation log, and returns the request
// data with a summary when a body was provided.
//
// When REQ or RSP differs from M, the handler binds the JSON body into REQ and
// delegates the operation to the phase service's UpdateMany method.
func UpdateManyFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	handler, _ := extractConfig(cfg...)
	meta := newFactoryMeta[M, REQ, RSP](routeFromConfig(cfg...), consts.PHASE_UPDATE_MANY, consts.PHASE_UPDATE_MANY_BEFORE, consts.PHASE_UPDATE_MANY_AFTER)
	return func(c *gin.Context) {
		var err error
		var reqErr error

		ctrlSpanCtx, span := meta.startControllerSpan(c)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_UPDATE_MANY)
		svc := meta.service()

		if !meta.typesEqual {
			var rsp RSP
			req := meta.newRequest()

			if reqErr = c.ShouldBindJSON(&req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
				log.Error(reqErr)
				JSON(c, CodeFailure.WithErr(reqErr))
				gstotel.RecordError(span, reqErr)
				return
			}
			if errors.Is(reqErr, io.EOF) {
				log.Warn(ErrRequestBodyEmpty)
			}
			var serviceCtx *types.ServiceContext
			if rsp, err = meta.traceServiceOperation(ctrlSpanCtx, consts.PHASE_UPDATE_MANY, func(spanCtx context.Context) (RSP, error) {
				serviceCtx = types.NewServiceContext(c, spanCtx, consts.PHASE_UPDATE_MANY)
				return svc.UpdateMany(serviceCtx, req)
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

		var req requestData[M]
		if reqErr = c.ShouldBindJSON(&req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
			log.Error(reqErr)
			JSON(c, CodeFailure.WithErr(reqErr))
			gstotel.RecordError(span, reqErr)
			return
		}
		if errors.Is(reqErr, io.EOF) {
			log.Warn(ErrRequestBodyEmpty)
		}

		// 1.Perform business logic processing before batch update resource.
		var serviceCtxBefore *types.ServiceContext
		if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_UPDATE_MANY_BEFORE, func(spanCtx context.Context) error {
			serviceCtxBefore = types.NewServiceContext(c, spanCtx, consts.PHASE_UPDATE_MANY_BEFORE)
			return svc.UpdateManyBefore(serviceCtxBefore, req.Items...)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}
		// 2.Batch update resource in database. Pure UPDATE with one transaction
		// around the batch: an item without an id fails the whole request, and
		// an item without a live row renders 404 and rolls the batch back, so
		// the batch endpoint can never insert rows.
		if !errors.Is(reqErr, io.EOF) {
			if err = handler(requestContext(c)).Update(req.Items...); err != nil {
				log.Error(err)
				JSON(c, writeErrorCoder(err))
				gstotel.RecordError(span, err)
				return
			}
		}
		// 3.Perform business logic processing after batch update resource.
		var serviceCtxAfter *types.ServiceContext
		if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_UPDATE_MANY_AFTER, func(spanCtx context.Context) error {
			serviceCtxAfter = types.NewServiceContext(c, spanCtx, consts.PHASE_UPDATE_MANY_AFTER)
			return svc.UpdateManyAfter(serviceCtxAfter, req.Items...)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}

		// 4.record operation log to database.
		record, _ := json.Marshal(req)
		reqData, _ := json.Marshal(req)
		respData, _ := json.Marshal(req)
		// cb.Enqueue(&modellogmgmt.OperationLog{
		// 	OP:        consts.OP_UPDATE_MANY,
		// 	Model:     typ.Name(),
		// 	Table:     tableName,
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
		m := meta.newModel()
		if err = am.RecordOperation(requestContext(c), m, &modellogmgmt.OperationLog{
			OP:        consts.OP_UPDATE_MANY,
			Model:     meta.name,
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

		if !errors.Is(reqErr, io.EOF) {
			req.Summary = &summary{
				Total:     len(req.Items),
				Succeeded: len(req.Items),
				Failed:    0,
			}
		}
		JSON(c, CodeSuccess, req)
	}
}
