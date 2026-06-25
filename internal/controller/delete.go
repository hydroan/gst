package controller

import (
	"context"
	"encoding/json"
	"fmt"
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
)

// Delete is a generic function to product gin handler to delete one or multiple resources.
// The resource type depends on the type of interface types.Model.
//
// Resource id must be specify and all resources that id matched will be deleted in database.
//
// Delete one resource:
// - specify resource `id` in "router parameter", eg: localhost:9000/api/myresource/myid
// - specify resource `id` in "query parameter", eg: localhost:9000/api/myresource?id=myid
//
// Delete multiple resources:
// - specify resource `id` slice in "http body data".
func Delete[M types.Model, REQ types.Request, RSP types.Response](c *gin.Context) {
	DeleteFactory[M, REQ, RSP]()(c)
}

// DeleteFactory returns a Gin handler that deletes one or more resources.
//
// When M, REQ, and RSP are the same type, the handler collects ids from the
// query string, the configured route parameter, and an optional JSON string
// array body. It deduplicates ids, runs delete hooks for each model, deletes the
// models through the configured database handler, records operation logs, and
// returns a no-content response status.
//
// When REQ or RSP differs from M, the handler binds the JSON body into REQ and
// delegates the operation to the phase service's Delete method.
func DeleteFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	handler, _ := extractConfig(cfg...)
	return func(c *gin.Context) {
		ctrlSpanCtx, span := startControllerSpan[M](c, consts.PHASE_DELETE)
		defer span.End()

		meta := types.RequestMetadataFromGin(c)
		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_DELETE)
		svc := serviceregistry.Resolve[M, REQ, RSP](consts.PHASE_DELETE)

		if !modelregistry.AreTypesEqual[M, REQ, RSP]() {
			var err error
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

			if reqErr := c.ShouldBindJSON(&req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
				log.Error(reqErr)
				JSON(c, CodeInvalidParam.WithErr(reqErr))
				gstotel.RecordError(span, reqErr)
				return
			}
			var serviceCtx *types.ServiceContext
			if rsp, err = traceServiceOperation[M, RSP](ctrlSpanCtx, consts.PHASE_DELETE, func(spanCtx context.Context) (RSP, error) {
				serviceCtx = types.NewServiceContext(c, spanCtx).WithPhase(consts.PHASE_DELETE)
				return svc.Delete(serviceCtx, req)
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

		// The underlying type of interface types.Model must be pointer to structure, such as *model.User.
		// 'typ' is the structure type, such as: model.User.
		typ := reflect.TypeOf(*new(M)).Elem()
		ml := make([]M, 0)
		idsSet := make(map[string]struct{})

		addID := func(id string) {
			if len(id) == 0 {
				return
			}
			if _, exists := idsSet[id]; exists {
				return
			}
			// 'm' is the structure value such as: &model.User{ID: myid, Name: myname}.
			m := reflect.New(typ).Interface().(M) //nolint:errcheck
			m.SetID(id)
			ml = append(ml, m)
			idsSet[id] = struct{}{}
		}

		// Delete one record accoding to "query parameter `id`".
		if id, ok := c.GetQuery(consts.QUERY_ID); ok {
			addID(id)
		}
		// Delete one record accoding to "route parameter `id`".
		if len(cfg) > 0 {
			addID(meta.Param(util.Deref(cfg[0]).ParamName))
		}
		// Delete multiple records accoding to "http body data".
		bodyIDs := make([]string, 0)
		if err := c.ShouldBindJSON(&bodyIDs); err == nil && len(bodyIDs) > 0 {
			for _, id := range bodyIDs {
				addID(id)
			}
		}

		ids := make([]string, 0, len(idsSet))
		for id := range idsSet {
			ids = append(ids, id)
		}
		log.Info(fmt.Sprintf("%s delete %v", typ.Name(), ids))

		// 1.Perform business logic processing before delete resources.
		// TODO: Should there be one service hook(DeleteBefore), or multiple?
		for _, m := range ml {
			var serviceCtxBefore *types.ServiceContext
			if err := traceServiceHook[M](ctrlSpanCtx, consts.PHASE_DELETE_BEFORE, func(spanCtx context.Context) error {
				serviceCtxBefore = types.NewServiceContext(c, spanCtx).WithPhase(consts.PHASE_DELETE_BEFORE)
				return svc.DeleteBefore(serviceCtxBefore, m)
			}); err != nil {
				log.Error(err)
				handleServiceError(c, serviceCtxBefore, err)
				gstotel.RecordError(span, err)
				return
			}
		}

		// find out the records and record to operation log.
		copied := make([]M, len(ml))
		for i := range ml {
			m := reflect.New(typ).Interface().(M) //nolint:errcheck
			m.SetID(ml[i].GetID())
			if err := handler(requestContext(c)).WithExpand(m.Expands()).Get(m, ml[i].GetID()); err != nil {
				log.Error(err)
				gstotel.RecordError(span, err)
			}
			copied[i] = m
		}

		// 2.Delete resources in database.
		if err := handler(requestContext(c)).Delete(ml...); err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		// 3.Perform business logic processing after delete resources.
		// TODO: Should there be one service hook(DeleteAfter), or multiple?
		for _, m := range ml {
			var serviceCtxAfter *types.ServiceContext
			if err := traceServiceHook[M](ctrlSpanCtx, consts.PHASE_DELETE_AFTER, func(spanCtx context.Context) error {
				serviceCtxAfter = types.NewServiceContext(c, spanCtx).WithPhase(consts.PHASE_DELETE_AFTER)
				return svc.DeleteAfter(serviceCtxAfter, m)
			}); err != nil {
				log.Error(err)
				handleServiceError(c, serviceCtxAfter, err)
				gstotel.RecordError(span, err)
				return
			}
		}

		// 4.record operation log to database.
		for i := range ml {
			record, _ := json.Marshal(copied[i])
			// cb.Enqueue(&modellogmgmt.OperationLog{
			// 	OP:        consts.OP_DELETE,
			// 	Model:     typ.Name(),
			// 	Table:     tableName,
			// 	RecordID:  ml[i].GetID(),
			// 	Record:    util.BytesToString(record),
			// 	IP:        c.ClientIP(),
			// 	User:      c.GetString(consts.CTX_USERNAME),
			// 	TraceID: c.GetString(consts.TRACE_ID),
			// 	URI:       c.Request.RequestURI,
			// 	Method:    c.Request.Method,
			// 	UserAgent: c.Request.UserAgent(),
			// })
			m := reflect.New(typ).Interface().(M) //nolint:errcheck
			if err := am.RecordOperation(requestContext(c), m, &modellogmgmt.OperationLog{
				OP:        consts.OP_DELETE,
				Model:     typ.Name(),
				RecordID:  ml[i].GetID(),
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
		}

		JSON(c, CodeSuccess.WithStatus(http.StatusNoContent))
	}
}
