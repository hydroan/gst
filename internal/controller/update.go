package controller

import (
	"context"
	"encoding/json"
	"io"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/requestctx"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
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
// by the body is ignored), sets the updater field, runs update hooks, writes
// the replacement through the configured database handler, and records an
// operation log. Existence is enforced by the database layer instead of a
// pre-read: a missing or soft-deleted record surfaces as
// database.ErrRecordNotFound and renders 404, and a unique-key collision
// renders 409. The UpdateBefore service hook therefore runs before existence
// is known. After a successful write the handler backfills the creation audit
// columns (created_at/created_by) from the persisted row, keeping the rest of
// the response object intact so hook-populated fields survive.
//
// When REQ or RSP differs from M, the handler binds the JSON body into REQ and
// delegates the operation to the phase service's Update method.
func UpdateFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	meta := newFactoryMeta[M, REQ, RSP](routeFromConfig(cfg...), consts.PHASE_UPDATE, consts.PHASE_UPDATE_BEFORE, consts.PHASE_UPDATE_AFTER)
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

			if reqErr = bindJSONRequest(c, &req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
				log.Errorz("bind request body failed", zap.Error(reqErr))
				JSON(c, CodeInvalidParam.WithErr(reqErr))
				gstotel.RecordError(span, reqErr)
				return
			}
			meta.normalizeRequest(&req)
			var serviceCtx *types.ServiceContext
			if rsp, err = meta.traceServiceOperation(ctrlSpanCtx, consts.PHASE_UPDATE, func(spanCtx context.Context) (RSP, error) {
				serviceCtx = types.NewServiceContext(c, spanCtx, consts.PHASE_UPDATE)
				return svc.Update(serviceCtx, req)
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
		if reqErr := bindJSONRequest(c, &req); reqErr != nil {
			// A full update replaces the resource, so an absent body is refused
			// rather than tolerated as "nothing to change".
			reqErr = requiredBodyError(reqErr)
			log.Errorz("bind request body failed", zap.Error(reqErr))
			JSON(c, CodeInvalidParam.WithErr(reqErr))
			gstotel.RecordError(span, reqErr)
			return
		}
		meta.normalizeModel(&req)

		// The resource id comes from the configured route parameter only.
		var id string
		if len(cfg) > 0 {
			id = reqMeta.Param(util.Deref(cfg[0]).ParamName)
		}
		if len(id) == 0 {
			log.Errorz(missingRouteParamMsg)
			JSON(c, CodeInvalidParam.WithMsg(missingRouteParamMsg))
			gstotel.RecordError(span, errors.New(missingRouteParamMsg))
			return
		}
		// 'm' is a fresh model instance, such as: &model.User{ID: myid}.
		m := meta.newModel()
		if !setRouteID(m, id) {
			// An id the model rejects cannot match any row; answer 404 without
			// touching the database.
			log.Errorz("route id rejected by model", zap.String("id", id))
			JSON(c, CodeNotFound)
			return
		}
		req.SetID(id)
		req.SetUpdatedBy(c.GetString(consts.CTX_USERNAME)) // set updated_by to current user

		// 1.Perform business logic processing before update resource.
		var serviceCtxBefore *types.ServiceContext
		if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_UPDATE_BEFORE, svc, func(spanCtx context.Context) error {
			serviceCtxBefore = types.NewServiceContext(c, spanCtx, consts.PHASE_UPDATE_BEFORE)
			return svc.UpdateBefore(serviceCtxBefore, req)
		}); err != nil {
			log.Errorz("service operation failed", zap.Error(err))
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}
		// 2.Update resource in database. The database layer answers existence:
		// ErrRecordNotFound renders 404, ErrDuplicatedKey renders 409.
		if err = database.Database[M](requestContext(c)).Update(req); err != nil {
			log.Errorz("database operation failed", zap.Error(err))
			JSON(c, databaseErrorCoder(err))
			gstotel.RecordError(span, err)
			return
		}
		// 3.Perform business logic processing after update resource.
		var serviceCtxAfter *types.ServiceContext
		if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_UPDATE_AFTER, svc, func(spanCtx context.Context) error {
			serviceCtxAfter = types.NewServiceContext(c, spanCtx, consts.PHASE_UPDATE_AFTER)
			return svc.UpdateAfter(serviceCtxAfter, req)
		}); err != nil {
			log.Errorz("service operation failed", zap.Error(err))
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}
		// Backfill the creation audit columns from the persisted row: Update
		// never writes created_at/created_by, so the request object holds
		// whatever the client sent. Only these two fields are copied — the
		// response keeps req so values populated by service hooks (including
		// non-persistent fields) survive. On a reload failure keep req as is:
		// the update itself already committed.
		reloaded := meta.newModel()
		if reloadErr := database.Database[M](requestContext(c)).Get(reloaded, id); reloadErr != nil {
			log.Warnz("reload audit columns failed", zap.Error(reloadErr))
		} else {
			req.SetCreatedAt(reloaded.GetCreatedAt())
			req.SetCreatedBy(reloaded.GetCreatedBy())
		}

		// 4.record operation log to database.
		// Record, Request, and Response carry the same serialized payload on
		// this action, so one marshal feeds all three columns.
		record, _ := json.Marshal(req)
		if err = am.RecordOperation(requestContext(c), req, &modellogmgmt.OperationLog{
			OP:        consts.OP_UPDATE,
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
