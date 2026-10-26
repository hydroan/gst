package modelregistry

import (
	"context"
	"time"
	"uuid"

	"github.com/hydroan/gst/internal/types"
	"gorm.io/gorm"
)

var _ types.Model = (*Base)(nil)

// Base implements types.Model for database-backed resources.
// Custom models can override these default methods when needed.
//
// Indexes are not declared through gorm tags; models declare them through
// the optional Indexes() []Index method instead.
//
// Associations are declared so queries can preload them, not to create
// physical foreign keys, so they carry constraint:- and leave referential
// integrity to the model hooks; a write zeroes them and touches the model's
// own table alone (see database.Create). Their gorm tags read like
//
//	gorm:"foreignKey:ParentID;constraint:-"
//	gorm:"foreignKey:ParentID;references:ID;constraint:-"
//
// Timestamps: created_at/updated_at are NOT NULL and carry no database
// default. The framework always provides both, in UTC via dbruntime.NowUTC
// (Create/Upsert force them, Update refreshes updated_at). A writer that
// bypasses the framework must set both columns itself, in UTC; omitting them
// fails fast under strict SQL mode instead of silently storing NULL, which
// would break created_at ordering, cursor pagination, and time-range
// filtering. A database default is ruled out on purpose: CURRENT_TIMESTAMP
// fills in the session-timezone wall clock, which silently diverges from the
// framework's UTC time base. deleted_at stays nullable: NULL marks the live
// row under soft deletion.
//
// created_at and created_by are creation facts: gorm's create-only permission
// writes them on insert and keeps every later write off them, Update and the
// conflict update of an Upsert included.
//
// The uuid columns take a size instead of an explicit char type: postgres
// blank-pads char values, so an id shorter than 36 would come back with
// trailing spaces there. varchar stores every id verbatim on all dialects.
// The note belongs here rather than above the field: doc comment extraction
// prefers a field's doc comment over its trailing one, so an implementation
// note placed above a field becomes that field's API-facing description.
type Base struct {
	ID string `json:"id" gorm:"primaryKey;size:36" query:"id" url:"-"` // UUIDv7 identifier for the record

	CreatedBy string         `json:"created_by,omitempty" gorm:"size:36;<-:create" query:"created_by" url:"-"` // ID of the user who created the record
	UpdatedBy string         `json:"updated_by,omitempty" gorm:"size:36" query:"updated_by" url:"-"`           // ID of the user who last updated the record
	CreatedAt time.Time      `json:"created_at,omitzero" gorm:"not null;<-:create" query:"created_at" url:"-"` // Timestamp when the record was created
	UpdatedAt time.Time      `json:"updated_at,omitzero" gorm:"not null" query:"updated_at" url:"-"`           // Timestamp when the record was last updated
	DeletedAt gorm.DeletedAt `json:"-" query:"-" url:"-"`                                                      // Timestamp when the record was deleted
}

func (b *Base) TableName() string        { return "" } // Business models must override with an explicit table name
func (b *Base) GetCreatedBy() string     { return b.CreatedBy }
func (b *Base) GetUpdatedBy() string     { return b.UpdatedBy }
func (b *Base) GetCreatedAt() time.Time  { return b.CreatedAt }
func (b *Base) GetUpdatedAt() time.Time  { return b.UpdatedAt }
func (b *Base) SetCreatedBy(s string)    { b.CreatedBy = s }
func (b *Base) SetUpdatedBy(s string)    { b.UpdatedBy = s }
func (b *Base) SetCreatedAt(t time.Time) { b.CreatedAt = t }
func (b *Base) SetUpdatedAt(t time.Time) { b.UpdatedAt = t }
func (b *Base) GetID() string            { return b.ID }
func (b *Base) ClearID()                 { b.ID = "" }
func (b *Base) Expands() []string        { return nil }
func (b *Base) Purge() bool              { return false } // Default to soft delete

// SetID sets the ID to the given id, or to a new UUIDv7 when none or an
// empty one is given. An already set ID is kept.
func (b *Base) SetID(id ...string) {
	if len(b.ID) != 0 {
		return
	}
	if len(id) == 0 || len(id[0]) == 0 {
		b.ID = uuid.NewV7().String()
		return
	}
	b.ID = id[0]
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
