package controller

import (
	"bytes"
	"context"
	"io"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/util"
	"github.com/hydroan/gst/logger"
	"go.opentelemetry.io/otel/trace"
)

// This file holds how a handler begins and what it reads off its request
// before the flow runs: the controller span it opens, the route the action
// registered under, the id the request names a record by, the body's bytes,
// the context the flow runs on and the service contexts its hooks get.

// beginRequest starts the span for the controller operation of the request
// c serves (see startSpan), rebinds the request context so downstream
// layers nest under it, and reports whether the action serves the request:
// a route parameter it may not carry (see routeParamError) is refused with
// 400, the way beginCall refuses one on a call, the refusal reported like
// any the controller answers (see reportFailure). The caller ends the span
// either way.
func (a *action[M, REQ, RSP]) beginRequest(c *gin.Context) (context.Context, trace.Span, bool) {
	spanCtx, span := a.startSpan(c.Request.Context(), c.Request.Method, c.FullPath())

	// Update request context with new span context
	c.Request = c.Request.WithContext(requestctx.WithMetadata(spanCtx, requestctx.FromGin(c)))

	err := routeParamError(func(yield func(string, string) bool) {
		for _, param := range c.Params {
			if !yield(param.Key, param.Value) {
				return
			}
		}
	}, a.itemParam)
	if err != nil {
		response.Error(c, reportFailure(logger.Controller.WithContext(c.Request.Context(), a.phase), span, "request rejected", err, invalidArgument(err)))
		return spanCtx, span, false
	}
	return spanCtx, span, true
}

// routeFromConfig returns the route carried by the controller config, or an
// empty string when no config declares one.
func routeFromConfig[M gst.Model](cfg ...*gst.ControllerConfig[M]) string {
	if len(cfg) > 0 && cfg[0] != nil {
		return cfg[0].Route
	}
	return ""
}

// routeID returns the id the request c serves names its record by: the
// route parameter the config names, the one place an id comes from over
// HTTP, the body's own ignored. A request naming none, the parameter empty
// or the config naming no parameter, is refused with 400 and
// missingRouteParamMsg, the refusal reported like any the controller
// answers (see reportFailure), and false is returned.
func routeID[M gst.Model](c *gin.Context, cfg []*gst.ControllerConfig[M], log gst.Logger, span trace.Span) (string, bool) {
	var id string
	if len(cfg) > 0 {
		id = requestctx.FromGin(c).Param(util.Deref(cfg[0]).ParamName)
	}
	if len(id) == 0 {
		response.Error(c, reportFailure(log, span, missingRouteParamMsg, nil, badRequest(missingRouteParamMsg)))
		return "", false
	}
	return id, true
}

// readJSONRequestBody reads the request body whole and puts it back for the
// binding that follows; a failure to read it is client-safe (see
// clientSafeBindError), and a request without a body reads as io.EOF.
func readJSONRequestBody(c *gin.Context) ([]byte, error) {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return nil, io.EOF
	}
	body, err := io.ReadAll(c.Request.Body)
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		return body, clientSafeBindError(err)
	}
	return body, nil
}

// requestContext returns the context a flow runs on for the request c
// serves: the request's own, which ends with the client going away, carrying
// the request metadata.
func requestContext(c *gin.Context) context.Context {
	if c == nil || c.Request == nil {
		return context.Background()
	}
	return requestctx.WithMetadata(c.Request.Context(), requestctx.FromGin(c))
}

// ginServiceContext returns the serviceContextFunc of the HTTP handler
// serving c: its service contexts carry the gin context, so the response
// helpers of ServiceContext keep working inside the hooks.
func ginServiceContext(c *gin.Context) serviceContextFunc {
	return func(ctx context.Context, phase apiaction.Phase) *gst.ServiceContext {
		return gst.NewServiceContext(c, ctx, phase)
	}
}
