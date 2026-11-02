// Package interceptor mounts the session check and the authorization on the
// gRPC services, the counterpart of the HTTP middleware.
package interceptor

import "github.com/hydroan/gst/interceptor"

func init() {
	interceptor.RegisterAuth(interceptor.IAMSession(), interceptor.Authz())
}
