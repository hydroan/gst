// Package tenant scopes a model's rows to the tenant the caller acts in.
//
// A model opts in by embedding Scope. That embedding is the whole declaration:
// it brings the column, its type, and the scoping the type carries, so a model
// can never hold a tenant column that does not scope, nor claim scoping without
// the column to do it with.
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
package tenant

import (
	"reflect"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// ErrTenantRequired reports an insert made under a cross-tenant scope without
// naming the tenant the row belongs to.
//
// A cross-tenant caller is the one caller the framework cannot stamp for: it is
// acting in every tenant, so there is no single one to write. Refusing is the
// only safe answer — defaulting would file the row under whichever tenant the
// framework picked, silently and in the one situation where the caller had the
// reach to mean any of them.
var ErrTenantRequired = errors.New("tenant: a cross-tenant insert must name the tenant the row belongs to")

// ErrTenantImmutable reports an update carrying a tenant other than the one the
// caller is acting in.
//
// The column is write-once, so the update could not have moved the row whatever
// it said. Refusing is what keeps that from being a silent no-op: a client that
// believed it moved a row and received a success would be wrong about where its
// data is, and nothing in the response would say so.
var ErrTenantImmutable = errors.New("tenant: a row cannot be moved to another tenant")

// Column is the database column a tenant-scoped model carries.
//
// It is fixed rather than configurable. A name the model chose would have to be
// resolved again by everything that consumes it, and three resolutions of one
// name are three chances to disagree. The framework already fixes id,
// created_at and deleted_at on the same reasoning.
const Column = "tenant_id"

// ID is the tenant column's type, and the whole of the row scoping.
//
// The clause methods below are what Gorm discovers on the field, so a model
// holding this type is scoped by having it. That is why the scoping cannot be
// switched off, shadowed by a method on the model, or missed by a code path
// nobody thought of: it is a property of the column, not of the caller.
type ID string

// String reports the tenant as plain text, for callers comparing it against a
// value that came from somewhere without a type.
func (id ID) String() string { return string(id) }

func (ID) QueryClauses(f *schema.Field) []clause.Interface {
	return []clause.Interface{predicateClause{field: f}}
}

func (ID) UpdateClauses(f *schema.Field) []clause.Interface {
	return []clause.Interface{predicateClause{field: f}, immutableClause{field: f}}
}

func (ID) DeleteClauses(f *schema.Field) []clause.Interface {
	return []clause.Interface{predicateClause{field: f}}
}

func (ID) CreateClauses(f *schema.Field) []clause.Interface {
	return []clause.Interface{stampClause{field: f}}
}

// Scope is embedded by models whose rows each belong to one tenant.
//
// The column is written once, on insert, and never again: the framework stamps
// it from the context and Gorm's create-only permission keeps every later write
// off it. A row therefore cannot change hands, and a client cannot move one by
// sending a different value.
type Scope struct {
	TenantID ID `json:"tenant_id,omitempty" gorm:"column:tenant_id;size:191;not null;default:default;<-:create" query:"tenant_id" url:"-"`
}

// GetTenantID reports the tenant the row belongs to.
func (s *Scope) GetTenantID() string { return string(s.TenantID) }

// predicateClause narrows a statement to the context's tenant.
//
// It deliberately does not consult Statement.Unscoped. That flag turns off soft
// deletion, and a caller reaching for deleted rows is not asking to reach into
// other tenants — the framework's own hard-delete path uses it, and tenant
// scoping has to survive that.
type predicateClause struct{ field *schema.Field }

func (predicateClause) Name() string               { return "" }
func (predicateClause) Build(clause.Builder)       {}
func (predicateClause) MergeClause(*clause.Clause) {}

func (c predicateClause) ModifyStatement(stmt *gorm.Statement) {
	if _, applied := stmt.Clauses[clauseMarker]; applied {
		return
	}
	scope := resolve(stmt.Context)
	if scope.across {
		return
	}

	stmt.AddClause(clause.Where{Exprs: []clause.Expression{
		clause.Eq{
			Column: clause.Column{Table: clause.CurrentTable, Name: c.field.DBName},
			Value:  scope.id,
		},
	}})
	stmt.Clauses[clauseMarker] = clause.Clause{}
}

// stampClause writes the context's tenant onto rows being inserted, replacing
// whatever they arrived with.
//
// Replacing rather than defaulting is the point: the column is data a client
// can send, and a create that honored it would let one tenant plant rows in
// another. Under a cross-tenant scope there is no one tenant to stamp, so the
// value the caller supplied stands; when it supplied none, the tenant its
// request is acting in does, and a caller with no request at all is refused
// rather than having one guessed for it.
type stampClause struct{ field *schema.Field }

func (stampClause) Name() string               { return "" }
func (stampClause) Build(clause.Builder)       {}
func (stampClause) MergeClause(*clause.Clause) {}

func (c stampClause) ModifyStatement(stmt *gorm.Statement) {
	scope := resolve(stmt.Context)

	forEachRow(stmt.ReflectValue, func(row reflect.Value) {
		if stmt.Error != nil {
			return
		}
		if !scope.across {
			if err := c.field.Set(stmt.Context, row, scope.id); err != nil {
				_ = stmt.AddError(err)
			}
			return
		}
		if value, zero := c.field.ValueOf(stmt.Context, row); !zero && value != nil && value != "" {
			return
		}
		// The row named no tenant. A cross-tenant request falls back to the one
		// it is acting in; a caller with no request at all has nothing to fall
		// back to, and guessing for it is how a row ends up in a tenant nobody
		// chose.
		if scope.id == "" {
			_ = stmt.AddError(ErrTenantRequired)
			return
		}
		if err := c.field.Set(stmt.Context, row, scope.id); err != nil {
			_ = stmt.AddError(err)
		}
	})
}

// immutableClause refuses an update that names a tenant other than the one the
// caller acts in.
//
// The refusal is about honesty rather than safety: Gorm's create-only
// permission already keeps the column out of every update statement, so the row
// was never in danger. What was in danger is the caller's belief about it.
//
// A cross-tenant caller is exempt because it acts in no single tenant, so there
// is nothing to compare the value against.
type immutableClause struct{ field *schema.Field }

func (immutableClause) Name() string               { return "" }
func (immutableClause) Build(clause.Builder)       {}
func (immutableClause) MergeClause(*clause.Clause) {}

func (c immutableClause) ModifyStatement(stmt *gorm.Statement) {
	scope := resolve(stmt.Context)
	if scope.across {
		return
	}

	forEachRow(stmt.ReflectValue, func(row reflect.Value) {
		if stmt.Error != nil {
			return
		}
		value, zero := c.field.ValueOf(stmt.Context, row)
		if zero {
			return
		}
		if named, ok := value.(ID); ok && string(named) != scope.id {
			_ = stmt.AddError(errors.Wrapf(ErrTenantImmutable, "named %q while acting in %q", named, scope.id))
		}
	})
}

// forEachRow visits every struct being written, whether the statement carries
// one or a batch.
func forEachRow(value reflect.Value, visit func(reflect.Value)) {
	switch value.Kind() {
	case reflect.Slice, reflect.Array:
		for i := range value.Len() {
			forEachRow(value.Index(i), visit)
		}
	case reflect.Pointer, reflect.Interface:
		if !value.IsNil() {
			forEachRow(value.Elem(), visit)
		}
	case reflect.Struct:
		visit(value)
	default:
	}
}

// clauseMarker records that a statement already carries the tenant predicate,
// so the query, update and delete clauses cannot each add their own.
const clauseMarker = "gst:tenant_scope"
