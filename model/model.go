package model

import (
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap/zapcore"
	"gorm.io/gorm"
)

var ErrMobileLength = errors.New("mobile number length must be 11")

var (
	mu sync.Mutex

	// TableChan is a buffered channel for asynchronous table registration.
	// It receives table models from Register() function for processing by InitDatabase.
	// The channel supports concurrent registration and real-time processing during initialization.
	TableChan = make(chan types.Model, 10240)

	// TableDBChan is a buffered channel for asynchronous table registration with custom database targeting.
	// It receives TableDB structs from RegisterTo() function for processing by InitDatabase.
	// The channel supports concurrent registration and real-time processing during initialization.
	TableDBChan = make(chan *TableDB, 1024)

	// RecordChan is a buffered channel for asynchronous record insertion.
	// It receives Record structs from Register() and RegisterTo() functions for processing by InitDatabase.
	// The channel supports concurrent registration and ensures records are inserted after table creation.
	//
	// Records are table records that must pre-exist before any database CRUD operations.
	// They are registered by Register() or RegisterTo() functions and processed asynchronously.
	RecordChan = make(chan *Record, 1024)
)

// Record is table record
type Record struct {
	Table   types.Model
	Rows    any
	Expands []string
	DBName  string
}

// TableDB represents a table model with its target database name for custom database registration.
// It is used by RegisterTo() function to associate a table with a specific database instance.
// The struct is sent through TableDBChan for asynchronous processing by InitDatabase.
type TableDB struct {
	Table  types.Model // The table model to be registered
	DBName string      // The target database name (case-insensitive)
}

// Register associates the model with database table and will be created automatically.
// If records provided, they will be inserted when application bootstrapping.
// This function supports asynchronous registration and can be called at any stage - before, during, or after InitDatabase execution.
//
// Key features:
//   - Thread-safe concurrent registration using mutex protection
//   - Asynchronous processing via goroutines and channels
//   - Automatic ID generation for records without IDs
//   - Real-time integration with InitDatabase process
//
// Parameters:
//   - records: Optional initial records to be seeded into the table. Can be single or multiple records.
//
// Examples:
//
//	// Create table 'users' only
//	Register[*model.User]()
//
//	// Create table 'users' and insert one record
//	Register[*model.User](&model.User{ID: 1, Name: "admin"})
//
//	// Create table 'users' and insert a single user record
//	Register[*model.User](user)
//
//	// Create table 'users' and insert multiple records
//	Register[*model.User](users...)  // where users is []*model.User
//
// NOTE:
//  1. Can be called in init() or at any time during application lifecycle.
//  2. Ensure the model package is imported in main.go.
//  3. The function is thread-safe and supports concurrent registration.
func Register[M types.Model](records ...M) {
	if !IsValid[M]() {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	table := reflect.New(reflect.TypeOf(*new(M)).Elem()).Interface().(M) //nolint:errcheck
	TableChan <- table

	// NOTE: it's necessary to set id before insert.
	for i := range records {
		if len(records[i].GetID()) == 0 {
			records[i].SetID()
		}
	}

	if len(records) != 0 {
		RecordChan <- &Record{Table: table, Rows: records, Expands: table.Expands()}
	}
}

// RegisterTo works identically to Register(), but registers the model on the specified database instance.
// This function supports asynchronous registration and can be called at any stage - before, during, or after InitDatabase execution.
//
// Key features:
//   - Thread-safe concurrent registration using mutex protection
//   - Asynchronous processing via goroutines and channels
//   - Automatic ID generation for records without IDs
//   - Real-time integration with InitDatabase process
//   - Custom database instance targeting
//
// Parameters:
//   - dbname: The name of the target database instance (case-insensitive)
//   - records: Optional initial records to be seeded into the table
//
// For more details and examples, see: Register().
func RegisterTo[M types.Model](dbname string, records ...M) {
	if !IsValid[M]() {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	dbname = strings.ToLower(dbname)
	table := reflect.New(reflect.TypeOf(*new(M)).Elem()).Interface().(M) //nolint:errcheck

	TableDBChan <- &TableDB{Table: table, DBName: dbname}

	if len(records) != 0 {
		RecordChan <- &Record{Table: table, Rows: records, Expands: table.Expands(), DBName: dbname}
	}
}

var (
	_ types.Model = (*Base)(nil)
	_ types.Model = (*Empty)(nil)
	_ types.Model = (*Any)(nil)
)

// Base implement types.Model interface.
// Each model must be expands the Base structure.
// You can implements your custom method to overwrite the defaults methods.
//
// Usually, there are some gorm tags that may be of interest to you.
// gorm:"unique"
// gorm:"foreignKey:ParentID"
// gorm:"foreignKey:ParentID,references:ID"
type Base struct {
	ID string `json:"id" gorm:"primaryKey;size:191" schema:"id" url:"-"` // Unique identifier for the record

	CreatedBy string         `json:"created_by,omitempty" gorm:"size:191;index" schema:"created_by" url:"-"` // User ID who created the record
	UpdatedBy string         `json:"updated_by,omitempty" gorm:"size:191;index" schema:"updated_by" url:"-"` // User ID who last updated the record
	CreatedAt *time.Time     `json:"created_at,omitempty" gorm:"index" schema:"-" url:"-"`                   // Timestamp when the record was created
	UpdatedAt *time.Time     `json:"updated_at,omitempty" gorm:"index" schema:"-" url:"-"`                   // Timestamp when the record was last updated
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index" schema:"-" url:"-"`                                      // Timestamp when the record was deleted

	// Query parameter
	Page       uint    `json:"-" gorm:"-" schema:"page" url:"page,omitempty"`                 // Pagination: page number (e.g., page=2)
	Size       uint    `json:"-" gorm:"-" schema:"size" url:"size,omitempty"`                 // Pagination: page size (e.g., size=10)
	Expand     *string `json:"-" gorm:"-" schema:"_expand" url:"_expand,omitempty"`           // Query parameter: fields to expand (e.g., _expand=children,parent)
	Depth      *uint   `json:"-" gorm:"-" schema:"_depth" url:"_depth,omitempty"`             // Query parameter: expansion depth (e.g., _depth=3)
	Fuzzy      *bool   `json:"-" gorm:"-" schema:"_fuzzy" url:"_fuzzy,omitempty"`             // Query parameter: enable fuzzy search (e.g., _fuzzy=true)
	SortBy     string  `json:"-" gorm:"-" schema:"_sortby" url:"_sortby,omitempty"`           // Query parameter: field to sort by (e.g., _sortby=name)
	NoCache    bool    `json:"-" gorm:"-" schema:"_nocache" url:"_nocache,omitempty"`         // Query parameter: disable cache (e.g., _nocache=false)
	ColumnName string  `json:"-" gorm:"-" schema:"_column_name" url:"_column_name,omitempty"` // Query parameter: column name for time range filtering (e.g., _column_name=created_at)
	StartTime  string  `json:"-" gorm:"-" schema:"_start_time" url:"_start_time,omitempty"`   // Query parameter: start time for range filtering (e.g., _start_time=2024-04-29+23:59:59)
	EndTime    string  `json:"-" gorm:"-" schema:"_end_time" url:"_end_time,omitempty"`       // Query parameter: end time for range filtering (e.g., _end_time=2024-04-29+23:59:59)
	Or         *bool   `json:"-" gorm:"-" schema:"_or" url:"_or,omitempty"`                   // Query parameter: use OR logic for conditions (e.g., _or=true)
	Index      string  `json:"-" gorm:"-" schema:"_index" url:"_index,omitempty"`             // Query parameter: index name for search (e.g., _index=name)
	Select     string  `json:"-" gorm:"-" schema:"_select" url:"_select,omitempty"`           // Query parameter: specific fields to select (e.g., _select=field1,field2)
	Nototal    bool    `json:"-" gorm:"-" schema:"_nototal" url:"_nototal,omitempty"`         // Query parameter: skip total count calculation (e.g., _nototal=true)
	// Cursor pagination
	CursorValue  *string `json:"-" gorm:"-" schema:"_cursor_value" url:"_cursor_value,omitempty"`   // Query parameter: cursor value for pagination (e.g., _cursor_value=0196a0b3-c9d1-713c-870e-adc76af9f857)
	CursorFields string  `json:"-" gorm:"-" schema:"_cursor_fields" url:"_cursor_fields,omitempty"` // Query parameter: fields used for cursor pagination (e.g., _cursor_fields=field1,field2)
	CursorNext   bool    `json:"-" gorm:"-" schema:"_cursor_next" url:"_cursor_next,omitempty"`     // Query parameter: direction for cursor pagination (e.g., _cursor_next=true)
}

func (b *Base) GetTableName() string       { return "" }
func (b *Base) GetCreatedBy() string       { return b.CreatedBy }
func (b *Base) GetUpdatedBy() string       { return b.UpdatedBy }
func (b *Base) GetCreatedAt() time.Time    { return util.Deref(b.CreatedAt) }
func (b *Base) GetUpdatedAt() time.Time    { return util.Deref(b.UpdatedAt) }
func (b *Base) SetCreatedBy(s string)      { b.CreatedBy = s }
func (b *Base) SetUpdatedBy(s string)      { b.UpdatedBy = s }
func (b *Base) SetCreatedAt(t time.Time)   { b.CreatedAt = &t }
func (b *Base) SetUpdatedAt(t time.Time)   { b.UpdatedAt = &t }
func (b *Base) GetID() string              { return b.ID }
func (b *Base) SetID(id ...string)         { setID(b, id...) }
func (b *Base) ClearID()                   { clearID(b) }
func (b *Base) Expands() []string          { return nil }
func (b *Base) Excludes() map[string][]any { return nil }
func (b *Base) Purge() bool                { return false } // Default to soft delete
func (b *Base) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if b == nil {
		return nil
	}
	enc.AddString("id", b.ID)
	enc.AddString("created_by", b.CreatedBy)
	enc.AddString("updated_by", b.UpdatedBy)
	enc.AddUint("page", b.Page)
	enc.AddUint("size", b.Size)
	return nil
}

func (*Base) CreateBefore(*types.ModelContext) error { return nil }
func (*Base) CreateAfter(*types.ModelContext) error  { return nil }
func (*Base) DeleteBefore(*types.ModelContext) error { return nil }
func (*Base) DeleteAfter(*types.ModelContext) error  { return nil }
func (*Base) UpdateBefore(*types.ModelContext) error { return nil }
func (*Base) UpdateAfter(*types.ModelContext) error  { return nil }
func (*Base) ListBefore(*types.ModelContext) error   { return nil }
func (*Base) ListAfter(*types.ModelContext) error    { return nil }
func (*Base) GetBefore(*types.ModelContext) error    { return nil }
func (*Base) GetAfter(*types.ModelContext) error     { return nil }

func setID(m types.Model, id ...string) {
	val := reflect.ValueOf(m).Elem()
	idField := val.FieldByName(consts.FIELD_ID)
	if len(idField.String()) != 0 {
		return
	}
	if len(id) == 0 {
		idField.SetString(util.UUID())
		return
	}

	// zap.S().Debug("setting id: " + id[0])
	if len(id[0]) == 0 {
		idField.SetString(util.UUID())
	} else {
		idField.SetString(id[0])
	}
}

func clearID(m types.Model) {
	val := reflect.ValueOf(m).Elem()
	idField := val.FieldByName(consts.FIELD_ID)
	idField.SetString("")
}

// Empty is a special model implementation that provides a no-op implementation of the types.Model interface.
// It serves as a marker type for structs that should be excluded from database operations and service hooks.
//
// Key characteristics:
//   - Structs with an anonymous model.Empty field are never migrated to the database
//   - All interface methods return zero values or no-op implementations
//   - IsModelEmpty() function returns true for structs containing only model.Empty
//   - Service hooks are bypassed when AreTypesEqual() returns false for Empty types
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
