// Package requestctx is the public entry to the request metadata that
// storage and service logs name a request by. The implementation lives in
// internal/requestctx; this package forwards the keys a project's own
// middleware stores the caller under and the composed helper that copied
// module files need, because those files land in consumer projects and may
// only import public framework packages.
package requestctx

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/requestctx"
)

// WithGinMetadata returns c's request context carrying the request metadata
// captured from the gin request: route, path, caller identity, tenant and
// trace id. Storage and service calls running on the returned context can
// name the request in their logs.
func WithGinMetadata(c *gin.Context) context.Context {
	return requestctx.WithMetadata(c.Request.Context(), requestctx.FromGin(c))
}

// The keys the caller is stored under on the gin context: the username and
// id of the authenticated user, the session the request carries and the
// tenant it acts in. The service context and the request logs read them as
// they stand, so a project middleware establishing the caller from
// somewhere else, a verified header or a token, writes the same keys:
// c.Set(KeyTenantID, id).
const (
	KeyUsername  = requestctx.KeyUsername
	KeyUserID    = requestctx.KeyUserID
	KeySessionID = requestctx.KeySessionID
	KeyTenantID  = requestctx.KeyTenantID
)
