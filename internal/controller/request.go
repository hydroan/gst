package controller

import (
	"bytes"
	"context"
	"io"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/types"
)

// This file holds what a handler reads off its request before the flow
// runs: the route the action registered under, the id the request names a
// record by, the body's bytes, and the context the flow runs on.

// missingRouteParamMsg answers, with 400, a request whose configured route
// parameter is absent.
const missingRouteParamMsg = "not found router param"

// routeFromConfig returns the route carried by the controller config, or an
// empty string when no config declares one.
func routeFromConfig[M types.Model](cfg ...*types.ControllerConfig[M]) string {
	if len(cfg) > 0 && cfg[0] != nil {
		return cfg[0].Route
	}
	return ""
}

// setID copies the id the request names a record by — the route parameter
// over HTTP, the id field of the message over gRPC — into the model and
// reports whether the model accepted it. UUID-keyed models (Base) accept any
// non-empty string, so the check never changes their behavior. Integer-keyed
// models (AutoBase) leave the id unset when the raw value does not parse
// into their key type; handlers must answer such requests with "not found"
// before any database access, because an unset id would silently drop the
// intended row filter and passing the raw value to SQL would rely on the
// database's implicit string-to-integer coercion (MySQL matches id=7 for
// '7abc').
//
// The caller must pass a non-empty id: UUID-keyed models generate a fresh id
// when given an empty value.
func setID(m types.Model, id string) bool {
	m.SetID(id)
	return len(m.GetID()) > 0
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
