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
	"go.uber.org/zap"
)

// DeleteMany handles a batch delete request with the default factory settings.
func DeleteMany[M types.Model, REQ types.Request, RSP types.Response](c *gin.Context) {
	DeleteManyFactory[M, REQ, RSP]()(c)
}

// DeleteManyFactory returns a Gin handler that deletes multiple resources.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// requestData[M], converts ids into model instances, runs batch delete hooks,
// deletes the models through the configured database handler, records an
// operation log, and returns a success response.
//
// When REQ or RSP differs from M, the handler binds the JSON body into REQ and
// delegates the operation to the phase service's DeleteMany method.
func DeleteManyFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	handler, _ := extractConfig(cfg...)
	meta := newFactoryMeta[M, REQ, RSP](routeFromConfig(cfg...), consts.PHASE_DELETE_MANY, consts.PHASE_DELETE_MANY_BEFORE, consts.PHASE_DELETE_MANY_AFTER)
	return func(c *gin.Context) {
		var err error
		var reqErr error

		ctrlSpanCtx, span := meta.startControllerSpan(c)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_DELETE_MANY)
		svc := meta.service()

		if !meta.typesEqual {
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
			if rsp, err = meta.traceServiceOperation(ctrlSpanCtx, consts.PHASE_DELETE_MANY, func(spanCtx context.Context) (RSP, error) {
				serviceCtx = types.NewServiceContext(c, spanCtx, consts.PHASE_DELETE_MANY)
				return svc.DeleteMany(serviceCtx, req)
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
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, reqErr)
			return
		}
		if errors.Is(reqErr, io.EOF) {
			log.Warn(ErrRequestBodyEmpty)
		}

		// 1.Perform business logic processing before batch delete resources.
		req.Items = make([]M, 0, len(req.IDs))
		for _, id := range req.IDs {
			m := meta.newModel()
			if !setRouteID(m, id) {
				// An id the model rejects cannot match any row; skip it to keep
				// batch delete idempotent instead of failing the whole batch.
				log.Warnz("skip id rejected by model", zap.String("id", id))
				continue
			}
			req.Items = append(req.Items, m)
		}
		var serviceCtxBefore *types.ServiceContext
		if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_DELETE_MANY_BEFORE, func(spanCtx context.Context) error {
			serviceCtxBefore = types.NewServiceContext(c, spanCtx, consts.PHASE_DELETE_MANY_BEFORE)
			return svc.DeleteManyBefore(serviceCtxBefore, req.Items...)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}
		if req.Options == nil {
			req.Options = new(options)
		}
		// 2.Batch delete resources in database.
		if !errors.Is(reqErr, io.EOF) {
			// purge mode is current not allowed in request.
			//
			// if err = handler(requestContext(c)).WithPurge(req.Options.Purge).Delete(req.Items...); err != nil {
			if err = handler(requestContext(c)).Delete(req.Items...); err != nil {
				log.Error(err)
				JSON(c, CodeFailure.WithErr(err))
				gstotel.RecordError(span, err)
				return
			}
		}
		// 3.Perform business logic processing after batch delete resources.
		var serviceCtxAfter *types.ServiceContext
		if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_DELETE_MANY_AFTER, func(spanCtx context.Context) error {
			serviceCtxAfter = types.NewServiceContext(c, spanCtx, consts.PHASE_DELETE_MANY_AFTER)
			return svc.DeleteManyAfter(serviceCtxAfter, req.Items...)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}

		// 4.record operation log to database.
		record, _ := json.Marshal(req)
		// cb.Enqueue(&modellogmgmt.OperationLog{
		// 	OP:        consts.OP_DELETE_MANY,
		// 	Model:     typ.Name(),
		// 	Table:     tableName,
		// 	Record:    util.BytesToString(record),
		// 	IP:        c.ClientIP(),
		// 	User:      c.GetString(consts.CTX_USERNAME),
		// 	TraceID: c.GetString(consts.TRACE_ID),
		// 	URI:       c.Request.RequestURI,
		// 	Method:    c.Request.Method,
		// 	UserAgent: c.Request.UserAgent(),
		// })
		m := meta.newModel()
		if err = am.RecordOperation(requestContext(c), m, &modellogmgmt.OperationLog{
			OP:        consts.OP_DELETE_MANY,
			Model:     meta.name,
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
