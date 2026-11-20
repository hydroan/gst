package controller

import (
	"context"
	"io"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
)

// This file holds the handler of an action served by its phase service's
// method: the one CreateHandler and its kind return when M, REQ and RSP
// differ, an action declaring a Payload or Result of its own having no flow
// of the framework's, the service doing the work. Its gRPC counterpart is
// ServiceCall (see call_grpc.go); both dispatch through serviceMethod (see
// action.go).

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
	binds := a.phase != apiaction.List && a.phase != apiaction.Get
	return func(c *gin.Context) {
		ctrlSpanCtx, span, ok := a.beginRequest(c)
		defer span.End()
		if !ok {
			return
		}

		log := logger.Controller.WithContext(c.Request.Context(), a.phase)
		req := a.newRequest()
		svc := a.service()

		// A Create sent as a multipart form carries a file, not JSON: the
		// service reads the form itself.
		form := a.phase == apiaction.Create && strings.EqualFold(c.ContentType(), "multipart/form-data")
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
				response.Error(c, reportFailure(log, span, "bind request body failed", reqErr, invalidArgument(reqErr)))
				return
			}
		}
		rsp, err := a.traceServiceOperation(ctrlSpanCtx, a.phase, func(spanCtx context.Context) (RSP, error) {
			return invoke(svc, types.NewServiceContext(c, spanCtx, a.phase), req)
		})
		if err != nil {
			response.Error(c, failService(ctrlSpanCtx, log, err))
			return
		}
		// Check if response is already written (e.g., SSE streaming)
		if !c.Writer.Written() {
			response.JSON(c, rsp)
		}
	}
}
