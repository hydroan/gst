package modelregistry

import (
	"context"
	"time"

	"github.com/hydroan/gst/types"
)

var _ types.Model = (*Empty)(nil)

// Empty is a no-op types.Model implementation for non-persistent actions.
//
// Key characteristics:
//   - Structs with an anonymous model.Empty field are never migrated to the database
//   - All interface methods return zero values or no-op implementations
//   - IsEmpty reports true for structs containing only model.Empty markers
//   - IsVirtual reports true for any struct embedding model.Empty, however many
//     other fields it declares
//   - Service hooks are bypassed when AreTypesEqual returns false for Empty types
//   - Commonly used for request/response DTOs that don't require persistence
//
// Usage example:
//
//	type LoginRequest struct {
//	    model.Empty
//	    Username string `json:"username"`
//	    Password string `json:"password"`
//	}
type Empty struct{}

// virtualModel marks Empty as the opt-in marker for virtual resources. The
// pointer receiver matches every other Empty method; models travel through
// the framework as pointers, so the marker is always in the method set where
// IsVirtual looks for it.
func (*Empty) virtualModel() {}

// Virtual is implemented by models that embed Empty.
//
// The marker method is unexported, so embedding Empty is the only way to
// satisfy Virtual: models outside this package can neither declare the method
// themselves nor accidentally opt in. A virtual resource has routes and
// services but no database table behind it, so table-touching controller
// phases must be skipped for it instead of querying a table that does not
// exist.
type Virtual interface {
	virtualModel()
}

// IsVirtual reports whether m is a virtual, table-less resource by embedding
// Empty directly or through a pointer. m must be a model pointer, which is
// the only shape models flow through the framework in.
func IsVirtual(m any) bool {
	_, ok := m.(Virtual)
	return ok
}

func (*Empty) GetTableName() string     { return "" }
func (*Empty) GetCreatedBy() string     { return "" }
func (*Empty) GetUpdatedBy() string     { return "" }
func (*Empty) GetCreatedAt() time.Time  { return time.Time{} }
func (*Empty) GetUpdatedAt() time.Time  { return time.Time{} }
func (*Empty) SetCreatedBy(s string)    {}
func (*Empty) SetUpdatedBy(s string)    {}
func (*Empty) SetCreatedAt(t time.Time) {}
func (*Empty) SetUpdatedAt(t time.Time) {}
func (*Empty) GetID() string            { return "" }
func (*Empty) SetID(id ...string)       {}
func (*Empty) ClearID()                 {}
func (*Empty) Expands() []string        { return nil }
func (*Empty) Purge() bool              { return false }

func (*Empty) CreateBefore(context.Context) error { return nil }
func (*Empty) CreateAfter(context.Context) error  { return nil }
func (*Empty) DeleteBefore(context.Context) error { return nil }
func (*Empty) DeleteAfter(context.Context) error  { return nil }
func (*Empty) UpdateBefore(context.Context) error { return nil }
func (*Empty) UpdateAfter(context.Context) error  { return nil }
func (*Empty) ListBefore(context.Context) error   { return nil }
func (*Empty) ListAfter(context.Context) error    { return nil }
func (*Empty) GetBefore(context.Context) error    { return nil }
func (*Empty) GetAfter(context.Context) error     { return nil }
