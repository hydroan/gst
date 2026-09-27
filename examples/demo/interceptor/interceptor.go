// Package interceptor registers the application's gRPC interceptors, the
// counterpart of middleware for the gRPC listener, each of its own in a
// file of its own: interceptor.Register applies to every call,
// interceptor.RegisterAuth to the calls of methods not declared Public(),
// and both run their interceptors in registration order. The session check
// of the iam module is mounted here too, ahead of what reads the caller it
// establishes.
package interceptor

import "github.com/hydroan/gst/interceptor"

func init() {
	interceptor.Register(servedBy)
	interceptor.RegisterAuth(interceptor.IAMSession(), actor)
}
