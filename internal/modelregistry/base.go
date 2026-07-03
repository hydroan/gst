package modelregistry

import (
	"context"
	"reflect"
	"time"

	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap/zapcore"
	"gorm.io/gorm"
)

var _ types.Model = (*Base)(nil)

// Base implements types.Model for database-backed resources.
// Custom models can override these default methods when needed.
//
// Usually, there are some gorm tags that may be of interest to you.
// gorm:"unique"
// gorm:"foreignKey:ParentID"
// gorm:"foreignKey:ParentID,references:ID"
type Base struct {
	ID string `json:"id" gorm:"primaryKey;type:char(36)" schema:"id" url:"-"` // UUIDv7 identifier for the record

	CreatedBy string         `json:"created_by,omitempty" gorm:"type:char(36)" schema:"created_by" url:"-"` // UUIDv7 user ID who created the record
	UpdatedBy string         `json:"updated_by,omitempty" gorm:"type:char(36)" schema:"updated_by" url:"-"` // UUIDv7 user ID who last updated the record
	CreatedAt time.Time      `json:"created_at,omitzero" schema:"-" url:"-"`                                // Timestamp when the record was created
	UpdatedAt time.Time      `json:"updated_at,omitzero" schema:"-" url:"-"`                                // Timestamp when the record was last updated
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index" schema:"-" url:"-"`                                     // Timestamp when the record was deleted
}

func (b *Base) GetTableName() string       { return "" }
func (b *Base) GetCreatedBy() string       { return b.CreatedBy }
func (b *Base) GetUpdatedBy() string       { return b.UpdatedBy }
func (b *Base) GetCreatedAt() time.Time    { return b.CreatedAt }
func (b *Base) GetUpdatedAt() time.Time    { return b.UpdatedAt }
func (b *Base) SetCreatedBy(s string)      { b.CreatedBy = s }
func (b *Base) SetUpdatedBy(s string)      { b.UpdatedBy = s }
func (b *Base) SetCreatedAt(t time.Time)   { b.CreatedAt = t }
func (b *Base) SetUpdatedAt(t time.Time)   { b.UpdatedAt = t }
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
	return nil
}

func (*Base) CreateBefore(context.Context) error { return nil }
func (*Base) CreateAfter(context.Context) error  { return nil }
func (*Base) DeleteBefore(context.Context) error { return nil }
func (*Base) DeleteAfter(context.Context) error  { return nil }
func (*Base) UpdateBefore(context.Context) error { return nil }
func (*Base) UpdateAfter(context.Context) error  { return nil }
func (*Base) ListBefore(context.Context) error   { return nil }
func (*Base) ListAfter(context.Context) error    { return nil }
func (*Base) GetBefore(context.Context) error    { return nil }
func (*Base) GetAfter(context.Context) error     { return nil }

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
