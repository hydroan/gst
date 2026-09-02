package controller

import (
	"context"
	"encoding/json"
	"io"
	"reflect"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/modelregistry"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// PatchMany handles a batch patch request with the default factory settings.
func PatchMany[M types.Model, REQ types.Request, RSP types.Response](c *gin.Context) {
	PatchManyFactory[M, REQ, RSP]()(c)
}

// PatchManyFactory returns a Gin handler that partially updates multiple resources.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// requestData[M], loads matching existing records for the requested items, copies
// fields present in each item into those records, runs batch patch hooks, updates
// the patched models through the configured database handler, records an operation
// log, and returns the request data with a summary when a body was provided.
//
// When REQ or RSP differs from M, the handler binds the JSON body into REQ and
// delegates the operation to the phase service's PatchMany method.
func PatchManyFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	meta := newFactoryMeta[M, REQ, RSP](routeFromConfig(cfg...), consts.PHASE_PATCH_MANY, consts.PHASE_PATCH_MANY_BEFORE, consts.PHASE_PATCH_MANY_AFTER)
	return func(c *gin.Context) {
		var err error
		var reqErr error

		ctrlSpanCtx, span := meta.startControllerSpan(c)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_PATCH_MANY)
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
			if rsp, err = meta.traceServiceOperation(ctrlSpanCtx, consts.PHASE_PATCH_MANY, func(spanCtx context.Context) (RSP, error) {
				serviceCtx = types.NewServiceContext(c, spanCtx, consts.PHASE_PATCH_MANY)
				return svc.PatchMany(serviceCtx, req)
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
		var shouldUpdates []M
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
		if reqErr = bindJSONRequest(c, &req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
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
		for i, m := range req.Items {
			var results []M
			v := meta.newModel()
			v.SetID(m.GetID())
			if err = database.Database[M](requestContext(c)).WithLimit(1).WithQuery(v).List(&results); err != nil {
				log.Errorz("database operation failed", zap.Error(err))
				gstotel.RecordError(span, err)
				continue
			}
			if len(results) != 1 {
				log.Warnz("partial update resource not found", zap.String("id", m.GetID()), zap.Int("count", len(results)))
				continue
			}
			if len(results[0].GetID()) == 0 {
				log.Warnz("partial update resource matched a row without id", zap.String("id", m.GetID()))
				continue
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
		var serviceCtxBefore *types.ServiceContext
		if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_PATCH_MANY_BEFORE, svc, func(spanCtx context.Context) error {
			serviceCtxBefore = types.NewServiceContext(c, spanCtx, consts.PHASE_PATCH_MANY_BEFORE)
			return svc.PatchManyBefore(serviceCtxBefore, shouldUpdates...)
		}); err != nil {
			log.Errorz("service operation failed", zap.Error(err))
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}
		// 2.Batch partial update resource in database. The rows were loaded
		// above, so ErrRecordNotFound only fires when one vanished in between;
		// unique-key collisions from the patched values render 409. Either way
		// the transaction rolls the whole batch back.
		if !errors.Is(reqErr, io.EOF) {
			if err = database.Database[M](requestContext(c)).Update(shouldUpdates...); err != nil {
				log.Errorz("database operation failed", zap.Error(err))
				JSON(c, databaseErrorCoder(err))
				gstotel.RecordError(span, err)
				return
			}
		}
		// 3.Perform business logic processing after batch patch resource.
		var serviceCtxAfter *types.ServiceContext
		if err = meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_PATCH_MANY_AFTER, svc, func(spanCtx context.Context) error {
			serviceCtxAfter = types.NewServiceContext(c, spanCtx, consts.PHASE_PATCH_MANY_AFTER)
			return svc.PatchManyAfter(serviceCtxAfter, shouldUpdates...)
		}); err != nil {
			log.Errorz("service operation failed", zap.Error(err))
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}

		// 4.record operation log to database.
		// NOTE: We should record the `req` instead of `oldVal`, the req is `newVal`.
		// Record, Request, and Response carry the same serialized payload on
		// this action, so one marshal feeds all three columns.
		record, _ := json.Marshal(req)
		m := meta.newModel()
		if err = am.RecordOperation(requestContext(c), m, &modellogmgmt.OperationLog{
			OP:        consts.OP_PATCH_MANY,
			Model:     meta.name,
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
