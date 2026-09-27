package controller

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/database"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/requestctx"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// UpdateHandler returns a Gin handler that replaces one resource.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// M, reads the resource id from the configured route parameter (the id carried
// by the body is ignored), and runs the update flow (see updateFlow), which
// answers with the replacement.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its Update method runs on the bound payload.
func UpdateHandler[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), consts.Update, consts.UpdateBefore, consts.UpdateAfter)
	if !a.typesEqual {
		return a.serviceHandler()
	}
	return func(c *gin.Context) {
		// The span context lives on in the request context the span start
		// rebinds, which requestContext reads for the flow.
		_, span := a.startControllerSpan(c)
		defer span.End()

		reqMeta := requestctx.FromGin(c)
		log := logger.Controller.WithContext(c.Request.Context(), consts.Update)

		req := a.newModel()
		if reqErr := bindJSONRequest(c, &req); reqErr != nil {
			// A full update replaces the resource, so an absent body is refused
			// rather than tolerated as "nothing to change".
			reqErr = requiredBodyError(reqErr)
			log.Errorz("bind request body failed", zap.Error(reqErr))
			JSON(c, CodeInvalidParam.WithErr(reqErr))
			gstotel.RecordError(span, reqErr)
			return
		}
		a.normalizeModel(&req)

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

		if err := a.updateFlow(requestContext(c), ginServiceContext(c), id, req); err != nil {
			JSON(c, failureCoder(err))
			return
		}
		JSON(c, CodeSuccess, req)
	}
}

// UpdateCall returns the update call of M on route, the counterpart of the
// handler UpdateHandler returns for the generated handler of an Update rpc:
// given the route parameters, the id the request message names the record
// by and the replacement it decoded into, it validates the replacement the
// way the handler validates a bound body, runs the update flow (see
// updateFlow) and answers with the replacement as stored, or with the
// status the failure maps to (see call).
func UpdateCall[M types.Model](route string) func(ctx context.Context, params map[string]string, id string, m M) (M, error) {
	a := newAction[M, M, M](route, consts.Update, consts.UpdateBefore, consts.UpdateAfter)
	return func(ctx context.Context, params map[string]string, id string, m M) (M, error) {
		var zero M
		c, err := a.beginCall(ctx, params, nil)
		defer c.end()
		if err != nil {
			return zero, c.invalid(err)
		}
		if reflect.ValueOf(m).IsNil() {
			return zero, c.missingRecord()
		}
		a.normalizeModel(&m)
		if err := validateRequest(m); err != nil {
			return zero, c.invalidMessage(err)
		}
		if id == "" {
			return zero, c.missingID()
		}
		if err := a.updateFlow(c.ctx, c.serviceContext, id, m); err != nil {
			return zero, c.fail(err)
		}
		return answer(c, m)
	}
}

// updateFlow runs the update flow, replacing the record id names with req: it
// sets the updater from the identity the request carries, runs the update
// hooks around the write, and records the operation. Existence is enforced by
// the database layer instead of a pre-read: a missing or soft-deleted record
// surfaces as database.ErrRecordNotFound and renders 404, and a unique-key
// collision renders 409. The UpdateBefore service hook therefore runs before
// existence is known. After a successful write the flow backfills the
// creation audit columns (created_at/created_by) from the persisted row,
// keeping the rest of req intact so hook-populated fields survive; req is the
// replacement answered. The id req carries is replaced by id, which must not
// be empty (see setID): SetID keeps an id already set, so the one the body
// carries is cleared first, or the replacement would land on the record it
// names; an id the model rejects answers CodeNotFound without touching the
// database.
func (a *action[M, REQ, RSP]) updateFlow(ctx context.Context, newServiceContext serviceContextFunc, id string, req M) error {
	log := logger.Controller.WithContext(ctx, consts.Update)
	svc := a.service()

	// 'm' is a fresh model instance, such as: &model.User{ID: myid}.
	m := a.newModel()
	if !setID(m, id) {
		// An id the model rejects cannot match any row; answer 404 without
		// touching the database.
		log.Errorz("route id rejected by model", zap.String("id", id))
		return &failure{coder: CodeNotFound}
	}
	req.ClearID()
	req.SetID(id)
	req.SetUpdatedBy(requestctx.FromContext(ctx).Username()) // set updated_by to current user

	// 1.Perform business logic processing before update resource.
	if err := a.traceServiceHook(ctx, consts.UpdateBefore, svc, func(spanCtx context.Context) error {
		return svc.UpdateBefore(newServiceContext(spanCtx, consts.UpdateBefore), req)
	}); err != nil {
		return failService(ctx, log, err)
	}
	// 2.Update resource in database. The database layer answers existence:
	// ErrRecordNotFound renders 404, ErrDuplicatedKey renders 409.
	if err := database.Database[M](ctx).Update(req); err != nil {
		return failDatabase(ctx, log, err)
	}
	// 3.Perform business logic processing after update resource.
	if err := a.traceServiceHook(ctx, consts.UpdateAfter, svc, func(spanCtx context.Context) error {
		return svc.UpdateAfter(newServiceContext(spanCtx, consts.UpdateAfter), req)
	}); err != nil {
		return failService(ctx, log, err)
	}
	// Backfill the creation audit columns from the persisted row: Update
	// never writes created_at/created_by, so the request object holds
	// whatever the client sent. Only these two fields are copied — the
	// response keeps req so values populated by service hooks (including
	// non-persistent fields) survive. On a reload failure keep req as is:
	// the update itself already committed.
	reloaded := a.newModel()
	if reloadErr := database.Database[M](ctx).Get(reloaded, id); reloadErr != nil {
		log.Warnz("reload audit columns failed", zap.Error(reloadErr))
	} else {
		req.SetCreatedAt(reloaded.GetCreatedAt())
		req.SetCreatedBy(reloaded.GetCreatedBy())
	}

	// 4.record operation log to database.
	// Record, Request, and Response carry the same serialized payload on
	// this action, so one marshal feeds all three columns.
	if err := audit.RecordOperation(ctx, req, consts.OP_UPDATE,
		func() *modellogmgmt.OperationLog {
			record, _ := json.Marshal(req)
			entry := operationLog(ctx, a.name)
			entry.RecordID = req.GetID()
			entry.Record = util.BytesToString(record)
			entry.Request = util.BytesToString(record)
			entry.Response = util.BytesToString(record)
			return entry
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}
	return nil
}
