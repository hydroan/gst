package controller

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/database"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// DeleteManyFactory returns a Gin handler that deletes multiple resources.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// requestData[M], runs the batch delete flow (see deleteManyFlow), and
// returns a success response.
//
// When REQ or RSP differs from M, the handler binds the JSON body into REQ and
// delegates the operation to the phase service's DeleteMany method.
func DeleteManyFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	meta := newFactoryMeta[M, REQ, RSP](routeFromConfig(cfg...), consts.PHASE_DELETE_MANY, consts.PHASE_DELETE_MANY_BEFORE, consts.PHASE_DELETE_MANY_AFTER)
	return func(c *gin.Context) {
		ctrlSpanCtx, span := meta.startControllerSpan(c)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_DELETE_MANY)

		if !meta.typesEqual {
			var err error
			var rsp RSP
			req := meta.newRequest()
			svc := meta.service()

			if reqErr := bindJSONRequest(c, &req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
				log.Errorz("bind request body failed", zap.Error(reqErr))
				JSON(c, CodeInvalidParam.WithErr(reqErr))
				gstotel.RecordError(span, reqErr)
				return
			}
			meta.normalizeRequest(&req)
			if rsp, err = meta.traceServiceOperation(ctrlSpanCtx, consts.PHASE_DELETE_MANY, func(spanCtx context.Context) (RSP, error) {
				return svc.DeleteMany(types.NewServiceContext(c, spanCtx, consts.PHASE_DELETE_MANY), req)
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

		var req requestData[M]
		if reqErr := bindJSONRequest(c, &req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
			log.Errorz("bind request body failed", zap.Error(reqErr))
			JSON(c, CodeInvalidParam.WithErr(reqErr))
			gstotel.RecordError(span, reqErr)
			return
		}
		normalizeBatchRequest(&req)

		if err := meta.deleteManyFlow(requestContext(c), ginServiceContext(c), &req); err != nil {
			JSON(c, failureCoder(err))
			return
		}
		JSON(c, CodeSuccess)
	}
}

// deleteManyFlow runs the batch delete flow on the ids of req: it converts
// them into model instances, which become the items of req, runs the batch
// delete hooks around the write, and records the operation. An empty id, or
// one of whitespace alone, names no record and fails the whole batch before
// anything is deleted; an id the model rejects is skipped, which keeps the
// batch idempotent. Whether the rows are purged is the model's decision (its
// Purge method), never the request's.
func (meta *factoryMeta[M, REQ, RSP]) deleteManyFlow(ctx context.Context, newServiceContext serviceContextFunc, req *requestData[M]) error {
	log := logger.Controller.WithContext(ctx, consts.PHASE_DELETE_MANY)
	svc := meta.service()

	// 1.Perform business logic processing before batch delete resources.
	req.Items = make([]M, 0, len(req.IDs))
	for _, id := range req.IDs {
		// An empty id, or one of whitespace alone, names no record: a
		// defective request, refused before anything is deleted.
		// Setting an empty one on a UUID-keyed model would mint a fresh
		// id instead. Any other id is used as sent, never trimmed.
		if strings.TrimSpace(id) == "" {
			err := errors.Wrapf(database.ErrIDRequired, "delete many %s", meta.name)
			return failWith(ctx, log, "batch delete with an empty id", databaseErrorCoder(err), err)
		}
		m := meta.newModel()
		if !setRouteID(m, id) {
			// An id the model rejects cannot match any row; skip it to keep
			// batch delete idempotent instead of failing the whole batch.
			log.Warnz("skip id rejected by model", zap.String("id", id))
			continue
		}
		req.Items = append(req.Items, m)
	}
	if err := meta.traceServiceHook(ctx, consts.PHASE_DELETE_MANY_BEFORE, svc, func(spanCtx context.Context) error {
		return svc.DeleteManyBefore(newServiceContext(spanCtx, consts.PHASE_DELETE_MANY_BEFORE), req.Items...)
	}); err != nil {
		return failService(ctx, log, err)
	}
	// 2.Batch delete resources in database. A batch without items deletes
	// nothing.
	if err := database.Database[M](ctx).Delete(req.Items...); err != nil {
		return failDatabase(ctx, log, err)
	}
	// 3.Perform business logic processing after batch delete resources.
	if err := meta.traceServiceHook(ctx, consts.PHASE_DELETE_MANY_AFTER, svc, func(spanCtx context.Context) error {
		return svc.DeleteManyAfter(newServiceContext(spanCtx, consts.PHASE_DELETE_MANY_AFTER), req.Items...)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// 4.record operation log to database.
	if err := am.RecordOperation(ctx, meta.newModel(), consts.OP_DELETE_MANY,
		func() *modellogmgmt.OperationLog {
			record, _ := json.Marshal(req)
			entry := operationLog(ctx, meta.name)
			entry.Record = util.BytesToString(record)
			return entry
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}
	return nil
}
