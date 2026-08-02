package tenant

import (
	"context"
	"strings"

	"github.com/hydroan/gst/internal/requestctx"
)

// Default is the authorization domain everything acts in when nothing resolved
// another one. A deployment that never configures a tenant resolver runs
// entirely inside it, which is what makes single tenancy the case that needs no
// configuration at all.
const Default = "default"

// scope is the tenant a context acts in. The zero value is not a valid scope;
// resolve never returns one.
type scope struct {
	id string

	// across spans every tenant. It is what the framework gives a subject whose
	// authorization was not decided in any one tenant, and what a caller with
	// no request at all asks for deliberately.
	across bool
}

type scopeKey struct{}

// In returns a context acting inside one tenant.
//
// It is how code running outside a request — a scheduled job, a startup seed, a
// command — supplies what a request gets from its authorization. It is not a
// way to widen a request: a request already carries the tenant it was
// authorized in, and calling In with another one inside a request hands that
// request data it was never authorized for.
func In(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	id = strings.TrimSpace(id)
	if id == "" {
		id = Default
	}
	return context.WithValue(ctx, scopeKey{}, scope{id: id})
}

// Across returns a context spanning every tenant.
//
// Reserved for work that is genuinely platform-wide: a subject whose
// authorization was not decided in any one tenant, or framework work that has
// to reach rows in all of them — recomputing what a global record implies for
// every tenant's roles, for instance.
//
// Nothing about it is implicit. A context that was never given a scope acts in
// one tenant, so forgetting this call yields too little data rather than too
// much, and the mistake shows up as something missing rather than as something
// leaked.
func Across(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, scopeKey{}, scope{across: true})
}

// resolve reports the tenant a statement built from ctx belongs to.
//
// The order is deliberate. An explicit scope wins, because the only way to get
// one is to ask. A request falls back to the tenant its authorization was
// decided in, which is the one value that cannot be wider than what the request
// was allowed. Anything else acts in the default tenant: never nothing, and
// never everything.
func resolve(ctx context.Context) scope {
	if ctx == nil {
		return scope{id: Default}
	}
	if s, ok := ctx.Value(scopeKey{}).(scope); ok {
		if s.across && s.id == "" {
			// A cross-tenant caller reads everywhere but still writes
			// somewhere. The request it arrived on names the tenant it is
			// acting in, and that is the least surprising place for a row it
			// did not file itself.
			s.id = requestTenant(ctx)
		}
		return s
	}
	if id := requestTenant(ctx); id != "" {
		return scope{id: id}
	}
	return scope{id: Default}
}

// requestTenant reports the tenant a request was authorized in, or empty when
// ctx did not come from one.
func requestTenant(ctx context.Context) string {
	return strings.TrimSpace(requestctx.FromContext(ctx).TenantID())
}

// From reports the tenant a context acts in, and whether it acts in exactly
// one. A cross-tenant context acts in none, so ok is false and the caller has
// to get the tenant from somewhere it names explicitly.
//
// It exists for the one thing the stamp cannot do for a model: derive something
// from the tenant before the row is written. A model computing its own key from
// the tenant has to use the tenant the row will end up carrying, and the value
// it arrived with is not that.
func From(ctx context.Context) (string, bool) {
	s := resolve(ctx)
	if s.across {
		return "", false
	}
	return s.id, true
}
