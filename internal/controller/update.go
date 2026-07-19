package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

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

// Update handles a full update (replace) request with the default factory settings.
func Update[M types.Model, REQ types.Request, RSP types.Response](c *gin.Context) {
	UpdateFactory[M, REQ, RSP]()(c)
}

// UpdateFactory returns a Gin handler that replaces one resource.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// M, reads the resource id from the configured route parameter (the id carried
// by the body is ignored), verifies that exactly one existing record matches,
// preserves the original creator fields, sets the updater field, runs update
// hooks, writes the replacement through the configured database handler, and
// records an operation log.
//
// When REQ or RSP differs from M, the handler binds the JSON body into REQ and
// delegates the operation to the phase service's Update method.
func UpdateFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	handler, _ := extractConfig(cfg...)
	meta := newFactoryMeta[M, REQ, RSP](consts.PHASE_UPDATE, consts.PHASE_UPDATE_BEFORE, consts.PHASE_UPDATE_AFTER)
	return func(c *gin.Context) {
		var err error
		var reqErr error

		ctrlSpanCtx, span := meta.startControllerSpan(c)
		defer span.End()

		reqMeta := requestctx.FromGin(c)
		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_UPDATE)
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
			if rsp, err = meta.traceServiceOperation(ctrlSpanCtx, consts.PHASE_UPDATE, func(spanCtx context.Context) (RSP, error) {
				serviceCtx = types.NewServiceContext(c, spanCtx, consts.PHASE_UPDATE)
				return svc.Update(serviceCtx, req)
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
		if reqErr := c.ShouldBindJSON(&req); reqErr != nil {
			log.Error(reqErr)
			JSON(c, CodeInvalidParam.WithErr(reqErr))
			gstotel.RecordError(span, reqErr)
			return
		}

		// The resource id comes from the configured route parameter only.
		var id string
		if len(cfg) > 0 {
			id = reqMeta.Param(util.Deref(cfg[0]).ParamName)
		}
		if len(id) == 0 {
			log.Error(CodeNotFoundRouteParam)
			JSON(c, CodeNotFoundRouteParam)
			gstotel.RecordError(span, errors.New(CodeNotFoundRouteParam.Msg()))
			return
		}
		req.SetID(id)
		log.Infoz(
			"update from request",
			zap.String("id", id),
			zap.Object(meta.fullName, req),
		)

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
		if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_UPDATE_BEFORE, func(spanCtx context.Context) error {
			serviceCtxBefore = types.NewServiceContext(c, spanCtx, consts.PHASE_UPDATE_BEFORE)
			return svc.UpdateBefore(serviceCtxBefore, req)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}
		// 2.Update resource in database.
		log.Infoz("update in database", zap.Object(meta.name, req))
		if err = handler(requestContext(c)).Update(req); err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		// 3.Perform business logic processing after update resource.
		var serviceCtxAfter *types.ServiceContext
		if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_UPDATE_AFTER, func(spanCtx context.Context) error {
			serviceCtxAfter = types.NewServiceContext(c, spanCtx, consts.PHASE_UPDATE_AFTER)
			return svc.UpdateAfter(serviceCtxAfter, req)
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

		JSON(c, CodeSuccess, req)
	}
}
