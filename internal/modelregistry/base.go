package modelregistry

import (
	"reflect"
	"time"

	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap/zapcore"
	"gorm.io/gorm"
)

// Base implements types.Model for database-backed resources.
// Custom models can override these default methods when needed.
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
