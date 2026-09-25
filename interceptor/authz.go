package interceptor

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/consts"
	gstgrpc "github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/tenant"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Authz authorizes calls using RBAC, the way middleware.Authz authorizes
// requests. The subject is the caller an authentication interceptor
// established, and the object and action are the HTTP route and method the
// call's action is served at (see Route), so the one policy set written for
// the HTTP routes decides for both listeners; a call with no caller is
// refused before any decision. Authz must be called before config.Init so
// config.Init can read AUTH_RBAC_ENABLED from the environment and enable
// RBAC initialization.
//
// Authz must run after an authentication interceptor, IAMSession or JwtAuth,
// that establishes the caller; registered ahead of one, it refuses every
// call as anonymous. The call's tenant is the caller's, tenant.Default when
// the caller carries none, and it is trusted twice over, as the middleware
// explains: the decision is made in it and every tenant-scoped row the call
// then reads or writes is scoped to it. A deployment whose tenant arrives
// another way establishes the caller again, with that tenant, in an
// interceptor of its own between the authentication and this one, and never
// from client input passed through as it stands.
func Authz() grpc.UnaryServerInterceptor {
	os.Setenv(config.AUTH_RBAC_ENABLED, "true")

	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// Every entry reports how long the decision took, for the reason the
		// middleware measures itself: deciding is in-memory work over the
		// whole policy set, and it appears in no other log.
		start := time.Now()
		act, obj := gstgrpc.Route(ctx)
		caller := gstgrpc.CallerOf(ctx)

		sub := strings.TrimSpace(caller.UserID)
		if sub == "" {
			// Anonymous calls are refused before the tenant is resolved, so
			// the decision is recorded without one.
			logAuthzDeny(ctx, caller, "", sub, obj, act, consts.DenyReasonUnauthenticated, time.Since(start))
			return nil, status.Error(codes.PermissionDenied, "permission denied")
		}
		tenantID := strings.TrimSpace(caller.TenantID)
		if tenantID == "" {
			tenantID = tenant.Default
			caller.TenantID = tenantID
			ctx = gstgrpc.WithCaller(ctx, caller)
		}

		// An attempt that could not be decided is reported as this server's
		// failure, which is what it is: nothing about the call is wrong, and
		// the client cannot change anything to make the decision reachable.
		decision, err := rbac.RBAC().Authorize(ctx, tenantID, sub, obj, act)
		if err != nil {
			logAuthzFailure(ctx, caller, tenantID, sub, obj, act, err, time.Since(start))
			return nil, status.Error(codes.Internal, "authorization unavailable")
		}
		if decision.Allowed {
			// A subject allowed as system_root was not authorized in any one
			// tenant, so its rows are not bound to one either; the scope is
			// taken from the decision itself, which keeps the reach of the
			// data equal to the reach of the grant.
			if decision.Source == consts.GrantSourceSystemRoot {
				ctx = tenant.Across(ctx)
			}
			logAuthzGrant(ctx, caller, tenantID, sub, obj, act, decision.Source, decision.MatchedRule, time.Since(start))
			return handler(ctx, req)
		}
		logAuthzDeny(ctx, caller, tenantID, sub, obj, act, decision.Reason, time.Since(start))
		return nil, status.Error(codes.PermissionDenied, "permission denied")
	}
}

// The authz log entries of a call carry what the middleware's carry — the
// request tuple, the caller's name, the elapsed time and the effect with
// what decided it — on a logger bound to the call's context, which adds the
// trace id the entries are correlated by along with the call's request
// metadata.

// logAuthzGrant writes one allowed decision, together with what allowed it:
// allowed_by names the rule kind, and matched_rule the policy row when a
// policy is what allowed the call.
func logAuthzGrant(
	ctx context.Context, caller gstgrpc.Caller, tenant, sub, obj, act string,
	source consts.GrantSource, matchedRule []string, elapsed time.Duration,
) {
	if logger.Authz == nil {
		return
	}
	fields := append(
		authzLogFields(caller, tenant, sub, obj, act, elapsed),
		zap.String("eft", string(consts.EffectAllow)),
		zap.String("allowed_by", string(source)),
	)
	if len(matchedRule) > 0 {
		fields = append(fields, zap.Strings("matched_rule", matchedRule))
	}
	logger.Authz.WithContext(ctx, "").Infoz("", fields...)
}

// logAuthzDeny writes one refused call, together with what it was missing:
// denied_by names which of the two steps behind a grant did not happen, the
// subject holding a role here or a permission covering the call, and is
// omitted when nothing could be determined.
func logAuthzDeny(
	ctx context.Context, caller gstgrpc.Caller, tenant, sub, obj, act string, reason consts.DenyReason, elapsed time.Duration,
) {
	if logger.Authz == nil {
		return
	}
	fields := append(
		authzLogFields(caller, tenant, sub, obj, act, elapsed),
		zap.String("eft", string(consts.EffectDeny)),
	)
	if reason != "" {
		fields = append(fields, zap.String("denied_by", string(reason)))
	}
	logger.Authz.WithContext(ctx, "").Infoz("", fields...)
}

// logAuthzFailure writes an authorization attempt that could not be decided,
// with no eft, since policy never allowed or denied it, and at the error
// level, which tells it apart.
func logAuthzFailure(
	ctx context.Context, caller gstgrpc.Caller, tenant, sub, obj, act string, err error, elapsed time.Duration,
) {
	if logger.Authz == nil {
		return
	}
	logger.Authz.WithContext(ctx, "").Errorz("", append(
		authzLogFields(caller, tenant, sub, obj, act, elapsed),
		zap.Error(err),
	)...)
}

// authzLogFieldCount is the six shared fields plus the most any one caller
// appends, the three of a grant; util.LogDuration counts as one, rendering
// two keys from a single inlined field.
const authzLogFieldCount = 9

// authzLogFields builds the field set every authz entry shares.
func authzLogFields(caller gstgrpc.Caller, tenant, sub, obj, act string, elapsed time.Duration) []zap.Field {
	return append(
		make([]zap.Field, 0, authzLogFieldCount),
		zap.String("tenant", tenant),
		zap.String("sub", sub),
		zap.String("obj", obj),
		zap.String("act", act),
		zap.String("username", caller.Username),
		util.LogDuration(elapsed),
	)
}
