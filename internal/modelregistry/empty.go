package modelregistry

import (
	"time"

	"github.com/hydroan/gst/types"
	"go.uber.org/zap/zapcore"
)

// Empty is a no-op types.Model implementation for non-persistent actions.
//
// Key characteristics:
//   - Structs with an anonymous model.Empty field are never migrated to the database
//   - All interface methods return zero values or no-op implementations
//   - IsEmpty reports true for structs containing only model.Empty or model.Any markers
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

func (*Empty) GetTableName() string                             { return "" }
func (*Empty) GetCreatedBy() string                             { return "" }
func (*Empty) GetUpdatedBy() string                             { return "" }
func (*Empty) GetCreatedAt() time.Time                          { return time.Time{} }
func (*Empty) GetUpdatedAt() time.Time                          { return time.Time{} }
func (*Empty) SetCreatedBy(s string)                            {}
func (*Empty) SetUpdatedBy(s string)                            {}
func (*Empty) SetCreatedAt(t time.Time)                         {}
func (*Empty) SetUpdatedAt(t time.Time)                         {}
func (*Empty) GetID() string                                    { return "" }
func (*Empty) SetID(id ...string)                               {}
func (*Empty) ClearID()                                         {}
func (*Empty) Expands() []string                                { return nil }
func (*Empty) Excludes() map[string][]any                       { return nil }
func (*Empty) Purge() bool                                      { return false }
func (*Empty) MarshalLogObject(enc zapcore.ObjectEncoder) error { return nil }

func (*Empty) CreateBefore(*types.ModelContext) error { return nil }
func (*Empty) CreateAfter(*types.ModelContext) error  { return nil }
func (*Empty) DeleteBefore(*types.ModelContext) error { return nil }
func (*Empty) DeleteAfter(*types.ModelContext) error  { return nil }
func (*Empty) UpdateBefore(*types.ModelContext) error { return nil }
func (*Empty) UpdateAfter(*types.ModelContext) error  { return nil }
func (*Empty) ListBefore(*types.ModelContext) error   { return nil }
func (*Empty) ListAfter(*types.ModelContext) error    { return nil }
func (*Empty) GetBefore(*types.ModelContext) error    { return nil }
func (*Empty) GetAfter(*types.ModelContext) error     { return nil }
