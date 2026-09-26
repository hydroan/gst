package controller

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/consts"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"go.uber.org/zap"
)

// This file holds the handler of an action served by its phase service's
// method: the one CreateHandler and its kind return when M, REQ and RSP
// differ, an
// action declaring a Payload or Result of its own having no flow of the
// framework's, the service doing the work. Its gRPC counterpart is
// ServiceCall (see call.go); serviceMethod is what both dispatch through.

// serviceHandler returns the handler of a's action for a request whose
// payload and result are the service's own types. It binds the JSON body
// into REQ, tolerating an absent one, unless the action is a Get or a List,
// whose GET request carries no body, or a Create sent as a multipart form,
// whose body is a file the service reads itself; runs the phase service's
// method in its service span on a service context of the request; and
// writes the result in the envelope unless the service wrote the response
// itself, streaming or sending a file.
func (a *action[M, REQ, RSP]) serviceHandler() gin.HandlerFunc {
	invoke := serviceMethod[M, REQ, RSP](a.phase)
	binds := a.phase != consts.PHASE_LIST && a.phase != consts.PHASE_GET
	return func(c *gin.Context) {
		ctrlSpanCtx, span := a.startControllerSpan(c)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), a.phase)
		req := a.newRequest()
		svc := a.service()

		// A Create sent as a multipart form carries a file, not JSON: the
		// service reads the form itself.
		form := a.phase == consts.PHASE_CREATE && strings.EqualFold(c.ContentType(), "multipart/form-data")
		if binds && !form {
			if reqErr := bindJSONRequest(c, &req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
				log.Errorz("bind request body failed", zap.Error(reqErr))
				JSON(c, CodeInvalidParam.WithErr(reqErr))
				gstotel.RecordError(span, reqErr)
				return
			}
			a.normalizeRequest(&req)
		}
		rsp, err := a.traceServiceOperation(ctrlSpanCtx, a.phase, func(spanCtx context.Context) (RSP, error) {
			return invoke(svc, types.NewServiceContext(c, spanCtx, a.phase), req)
		})
		if err != nil {
			log.Errorz("service operation failed", zap.Error(err))
			handleServiceError(c, err)
			gstotel.RecordError(span, err)
			return
		}
		// Check if response is already written (e.g., SSE streaming)
		if !c.Writer.Written() {
			JSON(c, CodeSuccess, rsp)
		}
	}
}

// serviceMethod returns the method of a service serving phase, the one the
// handler and the call of an action with a payload or result of its own
// dispatch to. A phase gRPC does not serve — the HTTP-only actions Import,
// Export and SSE, and the hook phases — has neither, so it panics.
func serviceMethod[M types.Model, REQ types.Request, RSP types.Response](phase consts.Phase) func(svc types.Service[M, REQ, RSP], sc *types.ServiceContext, req REQ) (RSP, error) {
	switch phase {
	case consts.PHASE_CREATE:
		return types.Service[M, REQ, RSP].Create
	case consts.PHASE_DELETE:
		return types.Service[M, REQ, RSP].Delete
	case consts.PHASE_UPDATE:
		return types.Service[M, REQ, RSP].Update
	case consts.PHASE_PATCH:
		return types.Service[M, REQ, RSP].Patch
	case consts.PHASE_LIST:
		return types.Service[M, REQ, RSP].List
	case consts.PHASE_GET:
		return types.Service[M, REQ, RSP].Get
	case consts.PHASE_CREATE_MANY:
		return types.Service[M, REQ, RSP].CreateMany
	case consts.PHASE_DELETE_MANY:
		return types.Service[M, REQ, RSP].DeleteMany
	case consts.PHASE_UPDATE_MANY:
		return types.Service[M, REQ, RSP].UpdateMany
	case consts.PHASE_PATCH_MANY:
		return types.Service[M, REQ, RSP].PatchMany
	}
	panic(fmt.Sprintf("controller: phase %q has no rpc; ServiceCall serves the actions of a model's gRPC service", phase))
}
