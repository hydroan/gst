package interceptor

import (
	"context"
	"os"
	"regexp"

	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/config"
	gstgrpc "github.com/hydroan/gst/grpc"
)

// routeParam matches a parameter of a route as the router registers it,
// :name. Authz writes it {name}, the spelling the route list (router.Routes)
// gives a route, so a policy written for the list decides for a call the
// way it does for a request. The HTTP line keeps the same rule for its list:
// the two lines share no code, each carrying its own.
var routeParam = regexp.MustCompile(`:([a-zA-Z0-9_]+)`)

// Authz authorizes calls using RBAC, through rbac.Enforce, the one decision
// path middleware.Authz takes as well: the subject is the caller an
// authentication interceptor established, the action is the HTTP method of
// the call's action (STREAM for a Stream action) and the object is the
// route the action is served at with every parameter written {name}, the
// way the route list spells it (see grpc.Route and routeParam), so a policy
// written for the route list decides for both listeners; a refusal is
// answered as the gRPC status the service error maps to. Over HTTP the
// middleware judges the concrete path of the request, so a policy naming a
// concrete path, /api/records/42, grants an HTTP request alone: a call
// carries no path, only the route. Authz must be called before config.Init
// so config.Init can read AUTH_RBAC_ENABLED from the environment and enable
// RBAC initialization.
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
		act, route := gstgrpc.Route(ctx)
		obj := routeParam.ReplaceAllString(route, "{$1}")
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
