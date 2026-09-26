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

// DeleteManyHandler returns a Gin handler that deletes multiple resources.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// batch[M], runs the batch delete flow (see deleteManyFlow), and
// returns a success response.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its DeleteMany method runs on the bound payload.
func DeleteManyHandler[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), consts.PHASE_DELETE_MANY, consts.PHASE_DELETE_MANY_BEFORE, consts.PHASE_DELETE_MANY_AFTER)
	if !a.typesEqual {
		return a.serviceHandler()
	}
	return func(c *gin.Context) {
		// The span context lives on in the request context the span start
		// rebinds, which requestContext reads for the flow.
		_, span := a.startControllerSpan(c)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_DELETE_MANY)

		var req batch[M]
		if reqErr := bindJSONRequest(c, &req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
			log.Errorz("bind request body failed", zap.Error(reqErr))
			JSON(c, CodeInvalidParam.WithErr(reqErr))
			gstotel.RecordError(span, reqErr)
			return
		}
		normalizeBatch(&req)

		if err := a.deleteManyFlow(requestContext(c), ginServiceContext(c), &req); err != nil {
			JSON(c, failureCoder(err))
			return
		}
		JSON(c, CodeSuccess)
	}
}

// DeleteManyCall returns the batch delete call of M on route, the
// counterpart of the handler DeleteManyHandler returns for the generated
// handler of a DeleteMany rpc: given the route parameters and the ids the
// request message carries, it validates the batch the way the handler
// validates a bound body, runs the batch delete flow (see deleteManyFlow)
// and answers nothing, or the status the failure maps to (see call).
func DeleteManyCall[M types.Model](route string) func(ctx context.Context, params map[string]string, ids []string) error {
	a := newAction[M, M, M](route, consts.PHASE_DELETE_MANY, consts.PHASE_DELETE_MANY_BEFORE, consts.PHASE_DELETE_MANY_AFTER)
	return func(ctx context.Context, params map[string]string, ids []string) error {
		c := a.beginCall(ctx, params, nil)
		defer c.end()
		req := batch[M]{IDs: ids}
		normalizeBatch(&req)
		if err := validateRequest(&req); err != nil {
			return c.invalidMessage(err)
		}
		if err := a.deleteManyFlow(c.ctx, c.serviceContext, &req); err != nil {
			return c.fail(err)
		}
		return c.finish()
	}
}

// deleteManyFlow runs the batch delete flow on the ids of req: it converts
// them into model instances, which become the items of req, runs the batch
// delete hooks around the write, and records the operation. An empty id, or
// one of whitespace alone, names no record and fails the whole batch before
// anything is deleted; an id the model rejects is skipped, which keeps the
// batch idempotent. Whether the rows are purged is the model's decision (its
// Purge method), never the request's.
func (a *action[M, REQ, RSP]) deleteManyFlow(ctx context.Context, newServiceContext serviceContextFunc, req *batch[M]) error {
	log := logger.Controller.WithContext(ctx, consts.PHASE_DELETE_MANY)
	svc := a.service()

	// 1.Perform business logic processing before batch delete resources.
	req.Items = make([]M, 0, len(req.IDs))
	for _, id := range req.IDs {
		// An empty id, or one of whitespace alone, names no record: a
		// defective request, refused before anything is deleted.
		// Setting an empty one on a UUID-keyed model would mint a fresh
		// id instead. Any other id is used as sent, never trimmed.
		if strings.TrimSpace(id) == "" {
			err := errors.Wrapf(database.ErrIDRequired, "delete many %s", a.name)
			return failWith(ctx, log, "batch delete with an empty id", databaseErrorCoder(err), err)
		}
		m := a.newModel()
		if !setID(m, id) {
			// An id the model rejects cannot match any row; skip it to keep
			// batch delete idempotent instead of failing the whole batch.
			log.Warnz("skip id rejected by model", zap.String("id", id))
			continue
		}
		req.Items = append(req.Items, m)
	}
	if err := a.traceServiceHook(ctx, consts.PHASE_DELETE_MANY_BEFORE, svc, func(spanCtx context.Context) error {
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
	if err := a.traceServiceHook(ctx, consts.PHASE_DELETE_MANY_AFTER, svc, func(spanCtx context.Context) error {
		return svc.DeleteManyAfter(newServiceContext(spanCtx, consts.PHASE_DELETE_MANY_AFTER), req.Items...)
	}); err != nil {
		return failService(ctx, log, err)
	}

	// 4.record operation log to database.
	if err := audit.RecordOperation(ctx, a.newModel(), consts.OP_DELETE_MANY,
		func() *modellogmgmt.OperationLog {
			record, _ := json.Marshal(req)
			entry := operationLog(ctx, a.name)
			entry.Record = util.BytesToString(record)
			return entry
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}
	return nil
}
