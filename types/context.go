package types

import (
	"context"

	"github.com/gin-gonic/gin"
	itypes "github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/types/consts"
)

// ServiceContext is the per-request context the framework hands to every
// service method. It implements context.Context by delegating to the request
// context, exposes request metadata (route, params, user identity, trace),
// and carries the response helpers a service needs without touching Gin
// directly.
type ServiceContext = itypes.ServiceContext

// NewServiceContext builds a ServiceContext from the Gin request, capturing
// request details, phase, and user metadata.
//
//nolint:revive // ServiceContext is constructed from the Gin request first.
func NewServiceContext(c *gin.Context, ctx context.Context, phase consts.Phase) *ServiceContext {
	return itypes.NewServiceContext(c, ctx, phase)
}

// RequestUserID reports the authenticated subject of the request ctx descends
// from, or "" when no request is behind it.
func RequestUserID(ctx context.Context) string {
	return itypes.RequestUserID(ctx)
}
