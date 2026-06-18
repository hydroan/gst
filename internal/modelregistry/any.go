package modelregistry

import (
	"time"

	"github.com/hydroan/gst/types"
	"go.uber.org/zap/zapcore"
)

// Any is a special placeholder model type used for database transactions
// when you don't need to specify a concrete model type.
//
// Usage example:
//
//	_ = database.Database[*model.Any](ctx.DatabaseContext()).TransactionFunc(func(tx any) error {
//	    // Perform database operations within transaction
//	    files := make([]*namespace.File, 0)
//	    if err = database.Database[*namespace.File](ctx.DatabaseContext()).
//	        WithTx(tx).
//	        WithQuery(&namespace.File{Format: namespace.FileFormat("kv")}).
//	        List(&files); err != nil {
//	        return err
//	    }
//	    for _, f := range files {
//	        f.Format = namespace.FileFomatShell
//	    }
//	    return database.Database[*namespace.File](ctx.DatabaseContext()).
//	        WithSelect("format").
//	        WithTx(tx).
//	        Update(files...)
//	})
//
// Note:
//   - Any does not correspond to any database table
//   - It's only used as a type parameter for generic database operations
//   - Unlike model.Empty, model.Any is specifically for transaction placeholders
type Any struct{}

func (*Any) GetTableName() string                             { return "" }
func (*Any) GetCreatedBy() string                             { return "" }
func (*Any) GetUpdatedBy() string                             { return "" }
func (*Any) GetCreatedAt() time.Time                          { return time.Time{} }
func (*Any) GetUpdatedAt() time.Time                          { return time.Time{} }
func (*Any) SetCreatedBy(s string)                            {}
func (*Any) SetUpdatedBy(s string)                            {}
func (*Any) SetCreatedAt(t time.Time)                         {}
func (*Any) SetUpdatedAt(t time.Time)                         {}
func (*Any) GetID() string                                    { return "" }
func (*Any) SetID(id ...string)                               {}
func (*Any) ClearID()                                         {}
func (*Any) Expands() []string                                { return nil }
func (*Any) Excludes() map[string][]any                       { return nil }
func (*Any) Purge() bool                                      { return false }
func (*Any) MarshalLogObject(enc zapcore.ObjectEncoder) error { return nil }

func (*Any) CreateBefore(*types.ModelContext) error { return nil }
func (*Any) CreateAfter(*types.ModelContext) error  { return nil }
func (*Any) DeleteBefore(*types.ModelContext) error { return nil }
func (*Any) DeleteAfter(*types.ModelContext) error  { return nil }
func (*Any) UpdateBefore(*types.ModelContext) error { return nil }
func (*Any) UpdateAfter(*types.ModelContext) error  { return nil }
func (*Any) ListBefore(*types.ModelContext) error   { return nil }
func (*Any) ListAfter(*types.ModelContext) error    { return nil }
func (*Any) GetBefore(*types.ModelContext) error    { return nil }
func (*Any) GetAfter(*types.ModelContext) error     { return nil }
