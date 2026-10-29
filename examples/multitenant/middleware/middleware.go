// Package middleware registers the application's HTTP middleware:
// middleware.RegisterAuth applies to the routes behind authentication, in
// registration order. The chain is the session check of the iam module,
// which establishes the caller and the tenant the session was opened in,
// then the project's own tenant override, then the authorization of the
// authz module, which decides in the tenant the request names by then and
// scopes the request's rows to it.
package middleware

import "github.com/hydroan/gst/middleware"

func init() {
	middleware.RegisterAuth(middleware.IAMSession(), gatewayTenant, middleware.Authz())
}
