package gst

import (
	"context"

	"github.com/hydroan/gst/internal/types"
)

// ServiceContext is the per-request context the framework hands to every
// service method. It implements context.Context by delegating to the request
// context, exposes request metadata (route, params, user identity, trace),
// and carries the response helpers a service needs without touching Gin
// directly.
type ServiceContext = types.ServiceContext

// RequestUserID reports the authenticated subject of the request ctx descends
// from, or "" when no request is behind it.
func RequestUserID(ctx context.Context) string {
	return types.RequestUserID(ctx)
}
