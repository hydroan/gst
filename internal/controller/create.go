package controller

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// Create handles a create request with the default factory settings.
func Create[M types.Model, REQ types.Request, RSP types.Response](c *gin.Context) {
	CreateFactory[M, REQ, RSP]()(c)
}

// CreateFactory returns a Gin handler that creates one resource.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// M, fills the creator/updater fields, runs the create hooks, writes the model
// through the configured database handler, records an operation log, and
// returns the created model.
//
// When REQ or RSP differs from M, the handler binds the JSON body into REQ and
// delegates the operation to the phase service's Create method. Multipart form
// requests are left unbound so the service can read the request directly.
func CreateFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	meta := newFactoryMeta[M, REQ, RSP](routeFromConfig(cfg...), consts.PHASE_CREATE, consts.PHASE_CREATE_BEFORE, consts.PHASE_CREATE_AFTER)
	return func(c *gin.Context) {
		var err error
		var reqErr error

		ctrlSpanCtx, span := meta.startControllerSpan(c)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_CREATE)
		svc := meta.service()

		if !meta.typesEqual {
			var rsp RSP
			req := meta.newRequest()

			// If the request content type if "multipart/form-data", then the request body is a file.
			// We should not try to parse it as JSON.
			if !strings.EqualFold(c.ContentType(), "multipart/form-data") {
				if reqErr = bindJSONRequest(c, &req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
					log.Errorz("bind request body failed", zap.Error(reqErr))
					JSON(c, CodeInvalidParam.WithErr(reqErr))
					gstotel.RecordError(span, reqErr)
					return
				}
				meta.normalizeRequest(&req)
			}
			var serviceCtx *types.ServiceContext
			if rsp, err = meta.traceServiceOperation(ctrlSpanCtx, consts.PHASE_CREATE, func(spanCtx context.Context) (RSP, error) {
				serviceCtx = types.NewServiceContext(c, spanCtx, consts.PHASE_CREATE)
				return svc.Create(serviceCtx, req)
			}); err != nil {
				log.Errorz("service operation failed", zap.Error(err))
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
		if reqErr = bindJSONRequest(c, &req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
			log.Errorz("bind request body failed", zap.Error(reqErr))
			JSON(c, CodeInvalidParam.WithErr(reqErr))
			gstotel.RecordError(span, reqErr)
			return
		}
		meta.normalizeModel(&req)
		if !errors.Is(reqErr, io.EOF) {
			req.SetCreatedBy(c.GetString(consts.CTX_USERNAME))
			req.SetUpdatedBy(c.GetString(consts.CTX_USERNAME))
		}

		// 1.Perform business logic processing before create resource.
		var serviceCtxBefore *types.ServiceContext
		if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_CREATE_BEFORE, func(spanCtx context.Context) error {
			serviceCtxBefore = types.NewServiceContext(c, spanCtx, consts.PHASE_CREATE_BEFORE)
			return svc.CreateBefore(serviceCtxBefore, req)
		}); err != nil {
			log.Errorz("service operation failed", zap.Error(err))
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}
		// 2.Create resource in database. Create is a pure INSERT: a primary or
		// unique key collision (including one held by a soft-deleted row)
		// surfaces as ErrDuplicatedKey and renders 409.
		if !errors.Is(reqErr, io.EOF) {
			if err = database.Database[M](requestContext(c)).WithExpand(req.Expands()).Create(req); err != nil {
				log.Errorz("database operation failed", zap.Error(err))
				JSON(c, writeErrorCoder(err))
				gstotel.RecordError(span, err)
				return
			}
		}
		// 3.Perform business logic processing after create resource
		var serviceCtxAfter *types.ServiceContext
		if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_CREATE_AFTER, func(spanCtx context.Context) error {
			serviceCtxAfter = types.NewServiceContext(c, spanCtx, consts.PHASE_CREATE_AFTER)
			return svc.CreateAfter(serviceCtxAfter, req)
		}); err != nil {
			log.Errorz("service operation failed", zap.Error(err))
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}

		// 4.record operation log to database.
		// Record, Request, and Response carry the same serialized payload on
		// this action, so one marshal feeds all three columns.
		record, _ := json.Marshal(req)
		if err = am.RecordOperation(requestContext(c), req, &modellogmgmt.OperationLog{
			OP:        consts.OP_CREATE,
			Model:     meta.name,
			RecordID:  req.GetID(),
			Record:    util.BytesToString(record),
			Request:   util.BytesToString(record),
			Response:  util.BytesToString(record),
			IP:        c.ClientIP(),
			User:      c.GetString(consts.CTX_USERNAME),
			TraceID:   c.GetString(consts.TRACE_ID),
			URI:       c.Request.RequestURI,
			Method:    c.Request.Method,
			UserAgent: c.Request.UserAgent(),
		}); err != nil {
			log.Warnz("record operation log failed", zap.Error(err))
		}

		JSON(c, CodeSuccess, req)
	}
}
