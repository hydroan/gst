package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/internal/modelregistry"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

type requestData[M types.Model] struct {
	// IDs is the id list that should be batch delete.
	IDs []string `json:"ids,omitempty"`
	// Items is the resource list that should be batch create/update/partial update.
	Items []M `json:"items,omitempty"`
	// Options is the batch operation options.
	Options *options `json:"options,omitempty"`
	// Summary is the batch operation result summary.
	Summary *summary `json:"summary,omitempty"`
}

type options struct {
	Atomic bool `json:"atomic,omitempty"`
	Purge  bool `json:"purge,omitempty"`
}

type summary struct {
	Total     int `json:"total"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

// CreateMany handles a batch create request with the default factory settings.
func CreateMany[M types.Model, REQ types.Request, RSP types.Response](c *gin.Context) {
	CreateManyFactory[M, REQ, RSP]()(c)
}

// CreateManyFactory returns a Gin handler that creates multiple resources.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// requestData[M], fills creator/updater fields on each item, runs batch create
// hooks, writes the items through the configured database handler, records an
// operation log, and returns the request data with a summary when a body was
// provided.
//
// When REQ or RSP differs from M, the handler binds the JSON body into REQ and
// delegates the operation to the phase service's CreateMany method.
func CreateManyFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	handler, _ := extractConfig(cfg...)
	return func(c *gin.Context) {
		var err error
		var reqErr error

		ctrlSpanCtx, span := startControllerSpan[M](c, consts.PHASE_CREATE_MANY)
		defer span.End()

		log := logger.Controller.WithRequestMetadata(types.NewRequestMetadata(c), consts.PHASE_CREATE_MANY)
		svc := serviceregistry.Resolve[M, REQ, RSP](consts.PHASE_CREATE_MANY)

		if !modelregistry.AreTypesEqual[M, REQ, RSP]() {
			var req REQ
			var rsp RSP

			reqTyp := reflect.TypeFor[REQ]()
			switch reqTyp.Kind() {
			case reflect.Struct:
				req = reflect.New(reqTyp).Elem().Interface().(REQ) //nolint:errcheck
			case reflect.Pointer:
				for reqTyp.Kind() == reflect.Pointer {
					reqTyp = reqTyp.Elem()
				}
				req = reflect.New(reqTyp).Interface().(REQ) //nolint:errcheck
			}

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
			if rsp, err = traceServiceOperation[M, RSP](ctrlSpanCtx, consts.PHASE_CREATE_MANY, func(spanCtx context.Context) (RSP, error) {
				serviceCtx = types.NewServiceContext(c, spanCtx).WithPhase(consts.PHASE_CREATE_MANY)
				return svc.CreateMany(serviceCtx, req)
			}); err != nil {
				log.Error(err)
				handleServiceError(c, serviceCtx, err)
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
		typ := reflect.TypeOf(*new(M)).Elem()
		val := reflect.New(typ).Interface().(M) //nolint:errcheck
		if reqErr = c.ShouldBindJSON(&req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
			log.Error(reqErr)
			JSON(c, CodeInvalidParam.WithErr(reqErr))
			gstotel.RecordError(span, reqErr)
			return
		}
		if errors.Is(reqErr, io.EOF) {
			log.Warn(ErrRequestBodyEmpty)
		}

		if req.Options == nil {
			req.Options = new(options)
		}
		for _, m := range req.Items {
			m.SetCreatedBy(c.GetString(consts.CTX_USERNAME))
			m.SetUpdatedBy(c.GetString(consts.CTX_USERNAME))
			log.Infoz("create_many", zap.Bool("atomic", req.Options.Atomic), zap.Object(typ.Name(), m))
		}

		// 1.Perform business logic processing before batch create resource.
		var serviceCtxBefore *types.ServiceContext
		if err = traceServiceHook[M](ctrlSpanCtx, consts.PHASE_CREATE_MANY_BEFORE, func(spanCtx context.Context) error {
			serviceCtxBefore = types.NewServiceContext(c, spanCtx).WithPhase(consts.PHASE_CREATE_MANY_BEFORE)
			return svc.CreateManyBefore(serviceCtxBefore, req.Items...)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, serviceCtxBefore, err)
			gstotel.RecordError(span, err)
			return
		}

		// 2.Batch create resource in database.
		if !errors.Is(reqErr, io.EOF) {
			if err = handler(types.NewDatabaseContext(c)).WithExpand(val.Expands()).Create(req.Items...); err != nil {
				log.Error(err)
				JSON(c, CodeFailure.WithErr(err))
				gstotel.RecordError(span, err)
				return
			}
		}
		// 3.Perform business logic processing after batch create resource
		var serviceCtxAfter *types.ServiceContext
		if err = traceServiceHook[M](ctrlSpanCtx, consts.PHASE_CREATE_MANY_AFTER, func(spanCtx context.Context) error {
			serviceCtxAfter = types.NewServiceContext(c, spanCtx).WithPhase(consts.PHASE_CREATE_MANY_AFTER)
			return svc.CreateManyAfter(serviceCtxAfter, req.Items...)
		}); err != nil {
			log.Error(err)
			handleServiceError(c, serviceCtxAfter, err)
			gstotel.RecordError(span, err)
			return
		}

		// 4.record operation log to database.
		record, _ := json.Marshal(req)
		reqData, _ := json.Marshal(req)
		respData, _ := json.Marshal(req)
		// cb.Enqueue(&modellogmgmt.OperationLog{
		// 	OP:        consts.OP_CREATE_MANY,
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
		if err = am.RecordOperation(types.NewDatabaseContext(c), val, &modellogmgmt.OperationLog{
			OP:        consts.OP_CREATE_MANY,
			Model:     typ.Name(),
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

		// FIXME: 如果某些字段增加了 gorm unique tag, 则更新成功后的资源 ID 时随机生成的，并不是数据库中的
		if !errors.Is(reqErr, io.EOF) {
			req.Summary = &summary{
				Total:     len(req.Items),
				Succeeded: len(req.Items),
				Failed:    0,
			}
		}
		JSON(c, CodeSuccess.WithStatus(http.StatusCreated), req)
	}
}
