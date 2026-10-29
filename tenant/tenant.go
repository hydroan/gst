// Package tenant scopes a model's rows to the tenant the caller acts in.
//
// A model opts in by embedding Scope, and that embedding is the only
// declaration there is: it brings the column, its type, and the scoping the
// type carries. The column's type is not part of this package, so a model can
// neither hold a tenant column that does not scope, nor one without what
// Scope's tags guarantee, a column written once, on insert, and never null.
//
// The scoping itself is not this package's invention and not the framework's
// either. The column's type implements Gorm's clause interfaces, which is the
// same mechanism gorm.DeletedAt uses for soft deletion — Gorm finds them on the
// field and applies them to every statement it builds for a schema carrying
// that field. Nothing in the persistence layer has to know a tenant exists:
// reads, updates and deletes carry the predicate, inserts carry the stamp, and
// a query written years from now carries both without being told to.
//
// The tenant comes from the context and from nowhere else. Inside a request it
// is the tenant the authorization was decided in, so the rows a request reaches
// can never be wider than what it was allowed. Outside a request, In supplies
// it and Across drops it.
//
// # What it does not cover
//
// A table without Scope is not protected by any of this. That is the right
// default — most tables belong to no tenant — but it leaves one shape worth
// naming: a table that is global while the permission to write it is granted
// per tenant. Every tenant's administrators can then reach it, and through it
// every other tenant. Scoping cannot answer that; it is a question about who
// may write a global record, and it has to be answered where that permission is
// granted.
//
// The implementation lives in internal/tenant; this package forwards the part
// a project uses.
package tenant

import (
	"context"

	"github.com/hydroan/gst/internal/tenant"
)

// Default is the tenant everything acts in when the context names none. A
// deployment in which nothing ever names a tenant runs entirely inside it,
// which is what makes single tenancy the case that needs no configuration
// at all.
const Default = tenant.Default

var (
	// ErrTenantRequired reports an insert made under a cross-tenant scope
	// without naming the tenant the row belongs to.
	//
	// A cross-tenant caller is the one caller the framework cannot stamp for:
	// it is acting in every tenant, so there is no single one to write.
	// Refusing is the only safe answer — defaulting would file the row under
	// whichever tenant the framework picked, silently and in the one situation
	// where the caller had the reach to mean any of them.
	ErrTenantRequired = tenant.ErrTenantRequired

	// ErrTenantImmutable reports an update carrying a tenant other than the one
	// the caller is acting in.
	//
	// The column is write-once, so the update could not have moved the row
	// whatever it said. Refusing is what keeps that from being a silent no-op:
	// a client that believed it moved a row and received a success would be
	// wrong about where its data is, and nothing in the response would say so.
	ErrTenantImmutable = tenant.ErrTenantImmutable
)

// Scope is embedded by models whose rows each belong to one tenant.
//
// The column is tenant_id, written once, on insert, and never again: the
// framework stamps it from the context and Gorm's create-only permission keeps
// every later write off it. A row therefore cannot change hands, and a client
// cannot move one by sending a different value. GetTenantID reports the tenant
// a row belongs to.
//
// The framework names the column itself, so Scope is embedded without a
// prefix; the start and gg check refuse a model renaming the column.
type Scope = tenant.Scope

// In returns a context acting inside one tenant.
//
// It is how code running outside a request — a scheduled job, a startup seed, a
// command — supplies what a request gets from its authorization. It is not a
// way to widen a request: a request already carries the tenant it was
// authorized in, and calling In with another one inside a request hands that
// request data it was never authorized for.
func In(ctx context.Context, id string) context.Context { return tenant.In(ctx, id) }

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
func Across(ctx context.Context) context.Context { return tenant.Across(ctx) }

// From reports the tenant a context acts in, and whether it acts in exactly
// one. A cross-tenant context acts in none, so ok is false and the caller has
// to get the tenant from somewhere it names explicitly.
//
// It exists for the one thing the stamp cannot do for a model: derive something
// from the tenant before the row is written. A model computing its own key from
// the tenant has to use the tenant the row will end up carrying, and the value
// it arrived with is not that.
func From(ctx context.Context) (string, bool) { return tenant.From(ctx) }
