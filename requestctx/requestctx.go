// Package requestctx is the public entry to the request metadata that
// storage and service logs name a request by. The implementation lives in
// internal/requestctx; this package forwards the composed helper that copied
// module files need, because those files land in consumer projects and may
// only import public framework packages.
package requestctx

import (
	"context"

	"github.com/gin-gonic/gin"
	internalrequestctx "github.com/hydroan/gst/internal/requestctx"
)

// WithGinMetadata returns c's request context carrying the request metadata
// captured from the gin request: route, path, caller identity, tenant and
// trace id. Storage and service calls running on the returned context can
// name the request in their logs.
func WithGinMetadata(c *gin.Context) context.Context {
	return internalrequestctx.WithMetadata(c.Request.Context(), internalrequestctx.FromGin(c))
}
