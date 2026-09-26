package interceptor

import (
	"context"
	"os"

	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/config"
	gstgrpc "github.com/hydroan/gst/grpc"
)

// Authz authorizes calls using RBAC, through rbac.Enforce, the one decision
// path middleware.Authz takes as well: the subject is the caller an
// authentication interceptor established, the object and action are the
// HTTP route and method the call's action is served at (see grpc.Route), so
// the one policy set written for the HTTP routes decides for both
// listeners, and a refusal is answered as the gRPC status the service error
// maps to. Authz must be called before config.Init so config.Init can read
// AUTH_RBAC_ENABLED from the environment and enable RBAC initialization.
//
// Authz must run after an authentication interceptor, IAMSession or JwtAuth,
// that establishes the caller; registered ahead of one, it refuses every
// call as anonymous. The call's tenant is the caller's, tenant.Default when
// the caller carries none, and the caller is established again with the
// tenant the decision was made in; a deployment whose tenant arrives
// another way establishes the caller with that tenant in an interceptor of
// its own between the authentication and this one, and never from client
// input passed through as it stands (rbac.Enforce says why).
func Authz() gstgrpc.Interceptor {
	os.Setenv(config.AUTH_RBAC_ENABLED, "true")

	return func(ctx context.Context) (context.Context, error) {
		caller := gstgrpc.CallerOf(ctx)
		act, obj := gstgrpc.Route(ctx)
		subject := rbac.Subject{UserID: caller.UserID, Username: caller.Username, TenantID: caller.TenantID}
		ctx, tenantID, err := rbac.Enforce(ctx, subject, obj, act)
		if err != nil {
			return nil, gstgrpc.StatusError(err)
		}
		if tenantID != caller.TenantID {
			caller.TenantID = tenantID
			ctx = gstgrpc.WithCaller(ctx, caller)
		}
		return ctx, nil
	}
}
