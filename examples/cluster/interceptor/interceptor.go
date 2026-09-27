// Package interceptor registers the application's gRPC interceptors, the
// counterpart of middleware for the gRPC listener, each in a file of its
// own: interceptor.Register applies to every call, interceptor.RegisterAuth
// to the calls of methods not declared Public(). The session check of the
// iam module is mounted here the way middleware mounts it for the HTTP
// routes: a call names its session in the authorization metadata or is
// refused.
package interceptor

import "github.com/hydroan/gst/interceptor"

func init() {
	interceptor.Register(servedBy)
	interceptor.RegisterAuth(interceptor.IAMSession())
}
