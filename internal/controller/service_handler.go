package controller

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"go.uber.org/zap"
)

// This file holds the handler of an action served by its phase service's
// method: the one CreateHandler and its kind return when M, REQ and RSP
// differ, an action declaring a Payload or Result of its own having no flow
// of the framework's, the service doing the work. Its gRPC counterpart is
// ServiceCall (see call.go); serviceMethod is what both dispatch through.

// serviceHandler returns the handler of a's action for a request whose
// payload and result are the service's own types. It binds the JSON body
// into REQ, an absent one, a JSON null included, as the zero request, which
// meets the binding tags all the same, a required field refusing it the way
// the gRPC call refuses a payload the message left unset; unless the action
// is a Get or a List, whose GET request carries no body, or a Create sent
// as a multipart form, whose body is a file the service reads itself. It
// runs the phase service's method in its service span on a service context
// of the request, and writes the result in the envelope unless the service
// wrote the response itself, streaming or sending a file.
func (a *action[M, REQ, RSP]) serviceHandler() gin.HandlerFunc {
	invoke := serviceMethod[M, REQ, RSP](a.phase)
	binds := a.phase != consts.List && a.phase != consts.Get
	return func(c *gin.Context) {
		ctrlSpanCtx, span := a.startControllerSpan(c)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), a.phase)
		req := a.newRequest()
		svc := a.service()

		// A Create sent as a multipart form carries a file, not JSON: the
		// service reads the form itself.
		form := a.phase == consts.Create && strings.EqualFold(c.ContentType(), "multipart/form-data")
		if binds && !form {
			reqErr := bindJSONRequest(c, &req)
			if errors.Is(reqErr, io.EOF) {
				// An absent body, or a JSON null, binds nothing: the zero
				// request, restored from the nil pointer the null leaves, is
				// validated all the same, a required field refusing it the
				// way the gRPC call refuses a payload the message left unset.
				a.normalizeRequest(&req)
				reqErr = nil
				if err := validateRequest(req); err != nil {
					reqErr = clientSafeBindError(err)
				}
			}
			if reqErr != nil {
				log.Errorz("bind request body failed", zap.Error(reqErr))
				response.Error(c, invalidArgument(reqErr))
				gstotel.RecordError(span, reqErr)
				return
			}
		}
		rsp, err := a.traceServiceOperation(ctrlSpanCtx, a.phase, func(spanCtx context.Context) (RSP, error) {
			return invoke(svc, types.NewServiceContext(c, spanCtx, a.phase), req)
		})
		if err != nil {
			log.Errorz("service operation failed", zap.Error(err))
			response.Error(c, err)
			gstotel.RecordError(span, err)
			return
		}
		// Check if response is already written (e.g., SSE streaming)
		if !c.Writer.Written() {
			response.JSON(c, rsp)
		}
	}
}

// serviceMethod returns the method of a service serving phase, the one the
// handler and the call of an action with a payload or result of its own
// dispatch to. The HTTP-only actions Import, Export and SSE, served by
// handlers of their own, and the hook phases have no such method, so it
// panics; only a gRPC ServiceCall can name one of them here, the HTTP
// handler being built for actions with a payload or result alone.
func serviceMethod[M types.Model, REQ types.Request, RSP types.Response](phase consts.Phase) func(svc types.Service[M, REQ, RSP], sc *types.ServiceContext, req REQ) (RSP, error) {
	switch phase {
	case consts.Create:
		return types.Service[M, REQ, RSP].Create
	case consts.Delete:
		return types.Service[M, REQ, RSP].Delete
	case consts.Update:
		return types.Service[M, REQ, RSP].Update
	case consts.Patch:
		return types.Service[M, REQ, RSP].Patch
	case consts.List:
		return types.Service[M, REQ, RSP].List
	case consts.Get:
		return types.Service[M, REQ, RSP].Get
	case consts.CreateMany:
		return types.Service[M, REQ, RSP].CreateMany
	case consts.DeleteMany:
		return types.Service[M, REQ, RSP].DeleteMany
	case consts.UpdateMany:
		return types.Service[M, REQ, RSP].UpdateMany
	case consts.PatchMany:
		return types.Service[M, REQ, RSP].PatchMany
	}
	panic(fmt.Sprintf("controller: phase %q has no rpc; ServiceCall serves the actions of a model's gRPC service", phase))
}
