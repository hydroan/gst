package controller

import (
	"context"
	"encoding/json"
	"io"
	"reflect"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/database"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/modelregistry"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// PatchManyFactory returns a Gin handler that partially updates multiple resources.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// requestData[M] along with the set of fields each item carried, and runs the
// batch patch flow (see patchManyFlow), which answers with the patched
// records.
//
// When REQ or RSP differs from M, the handler binds the JSON body into REQ and
// delegates the operation to the phase service's PatchMany method.
func PatchManyFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	meta := newFactoryMeta[M, REQ, RSP](routeFromConfig(cfg...), consts.PHASE_PATCH_MANY, consts.PHASE_PATCH_MANY_BEFORE, consts.PHASE_PATCH_MANY_AFTER)
	return func(c *gin.Context) {
		ctrlSpanCtx, span := meta.startControllerSpan(c)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_PATCH_MANY)

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
			if rsp, err = meta.traceServiceOperation(ctrlSpanCtx, consts.PHASE_PATCH_MANY, func(spanCtx context.Context) (RSP, error) {
				return svc.PatchMany(types.NewServiceContext(c, spanCtx, consts.PHASE_PATCH_MANY), req)
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
		body, err := readJSONRequestBody(c)
		if err != nil {
			log.Errorz("bind request body failed", zap.Error(err))
			JSON(c, CodeInvalidParam.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		fieldSets, fieldErr := patchManyFieldSetsFromJSONBody(meta.typ, body)
		if fieldErr != nil && !errors.Is(fieldErr, io.EOF) {
			log.Errorz("bind request body failed", zap.Error(fieldErr))
			JSON(c, CodeInvalidParam.WithErr(fieldErr))
			gstotel.RecordError(span, fieldErr)
			return
		}
		if reqErr := bindJSONRequest(c, &req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
			log.Errorz("bind request body failed", zap.Error(reqErr))
			JSON(c, CodeInvalidParam.WithErr(reqErr))
			gstotel.RecordError(span, reqErr)
			return
		}
		normalizeBatchRequest(&req)
		// A versioned model must carry a version on every item, exactly like
		// the single-resource patch; failing the whole batch up front keeps
		// the all-or-nothing shape a defective request deserves. See
		// modelregistry.Version.
		if versionField, versioned := modelregistry.VersionFieldName(meta.newModel()); versioned {
			for i := range req.Items {
				itemFields := patchFieldSet{}
				if i < len(fieldSets) {
					itemFields = fieldSets[i]
				}
				if _, ok := itemFields[versionField]; !ok {
					log.Errorz("versioned model patched without its version",
						zap.Int("item", i), zap.String("field", versionField))
					JSON(c, databaseErrorCoder(database.ErrVersionRequired))
					gstotel.RecordError(span, database.ErrVersionRequired)
					return
				}
			}
		}

		rsp, err := meta.patchManyFlow(requestContext(c), ginServiceContext(c), &req, fieldSets)
		if err != nil {
			JSON(c, failureCoder(err))
			return
		}
		JSON(c, CodeSuccess, rsp)
	}
}

// patchManyFlow runs the batch patch flow on the items of req: it loads the
// record each item names, copies the fields the item carried (fieldSets, one
// set per item in order; an item past its end carries none) into that
// record, runs the batch patch hooks around the write, records the
// operation, and returns the batch with the patched records, as the hooks
// left them, in place of the items. The batch patches all of its items or
// none, as a batch update does: an item without an id fails the whole batch
// with 400 before any record is read, and an item whose record does not exist
// fails it with 404 before anything is written.
//
// Each write is the whole record loaded, not only the fields its item
// carried, so concurrent patches of one record resolve as last writer wins
// exactly as in patchFlow: a later patch puts back the fields an earlier one
// changed, even different ones, and both answer success. A model that needs
// the stale write refused instead declares model.Version.
func (meta *factoryMeta[M, REQ, RSP]) patchManyFlow(ctx context.Context, newServiceContext serviceContextFunc, req *requestData[M], fieldSets []patchFieldSet) (requestData[M], error) {
	var zero requestData[M]
	log := logger.Controller.WithContext(ctx, consts.PHASE_PATCH_MANY)
	svc := meta.service()

	var shouldUpdates []M
	for i, m := range req.Items {
		// An item without an id names no record: a defective request,
		// refused before any record is read. Setting an empty id on a
		// UUID-keyed model would mint a fresh one instead.
		if len(m.GetID()) == 0 {
			err := errors.Wrapf(database.ErrIDRequired, "patch many %s item %d", meta.name, i)
			return zero, failWith(ctx, log, "batch patch item without its id", databaseErrorCoder(err), err)
		}
		var results []M
		v := meta.newModel()
		v.SetID(m.GetID())
		// Pinned to the primary: the row read here is merged with the
		// patch and written straight back, so a stale one would write
		// the untouched fields back as they were on the replica.
		if err := database.Database[M](ctx).WithReplica(false).WithLimit(1).WithQuery(v).List(&results); err != nil {
			return zero, failDatabase(ctx, log, err)
		}
		if len(results) != 1 || len(results[0].GetID()) == 0 {
			err := errors.Wrapf(database.ErrRecordNotFound, "patch many %s id=%s", meta.name, m.GetID())
			return zero, failWith(ctx, log, "partial update resource not found", databaseErrorCoder(err), err)
		}
		oldVal, newVal := reflect.ValueOf(results[0]).Elem(), reflect.ValueOf(m).Elem()
		fields := patchFieldSet{}
		if i < len(fieldSets) {
			fields = fieldSets[i]
		}
		patchValue(log, meta.typ, oldVal, newVal, fields)
		shouldUpdates = append(shouldUpdates, oldVal.Addr().Interface().(M)) //nolint:errcheck
	}

	// 1.Perform business logic processing before batch patch resource.
	if err := meta.traceServiceHook(ctx, consts.PHASE_PATCH_MANY_BEFORE, svc, func(spanCtx context.Context) error {
		return svc.PatchManyBefore(newServiceContext(spanCtx, consts.PHASE_PATCH_MANY_BEFORE), shouldUpdates...)
	}); err != nil {
		return zero, failService(ctx, log, err)
	}
	// 2.Batch partial update resource in database. The rows were loaded
	// above, so ErrRecordNotFound only fires when one vanished in between;
	// unique-key collisions from the patched values render 409. Either way
	// the transaction rolls the whole batch back. A batch without items
	// writes nothing.
	if err := database.Database[M](ctx).Update(shouldUpdates...); err != nil {
		return zero, failDatabase(ctx, log, err)
	}
	// 3.Perform business logic processing after batch patch resource.
	if err := meta.traceServiceHook(ctx, consts.PHASE_PATCH_MANY_AFTER, svc, func(spanCtx context.Context) error {
		return svc.PatchManyAfter(newServiceContext(spanCtx, consts.PHASE_PATCH_MANY_AFTER), shouldUpdates...)
	}); err != nil {
		return zero, failService(ctx, log, err)
	}

	// The response carries the patched records, whose fields the hooks
	// may have set, not the items of the request.
	rsp := *req
	rsp.Items = shouldUpdates

	// 4.record operation log to database.
	// NOTE: We should record the `req` instead of `oldVal`, the req is `newVal`.
	// Record and Request both carry the request payload, so one marshal
	// feeds both columns; Response carries the patched records instead.
	if err := am.RecordOperation(ctx, meta.newModel(), consts.OP_PATCH_MANY,
		func() *modellogmgmt.OperationLog {
			record, _ := json.Marshal(req)
			respData, _ := json.Marshal(rsp)
			entry := operationLog(ctx, meta.name)
			entry.Record = util.BytesToString(record)
			entry.Request = util.BytesToString(record)
			entry.Response = util.BytesToString(respData)
			return entry
		}); err != nil {
		log.Warnz("record operation log failed", zap.Error(err))
	}
	return rsp, nil
}
