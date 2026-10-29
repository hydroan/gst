// Package interceptor registers the application's gRPC interceptors, the
// counterpart of middleware for the gRPC listener: interceptor.RegisterAuth
// applies to the calls of methods not declared Public(), in registration
// order. The chain mirrors the HTTP one: the session check, which
// establishes the caller with the tenant the session was opened in, the
// project's own tenant override, then the authorization.
package interceptor

import "github.com/hydroan/gst/interceptor"

func init() {
	interceptor.RegisterAuth(interceptor.IAMSession(), gatewayTenant, interceptor.Authz())
}
