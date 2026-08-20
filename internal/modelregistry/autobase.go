package modelregistry

import (
	"context"
	"strconv"
	"time"

	"github.com/hydroan/gst/types"
	"gorm.io/gorm"
)

var _ types.Model = (*AutoBase)(nil)

// AutoBase implements types.Model for database-backed resources that use an
// auto-increment integer primary key instead of Base's UUIDv7 string key.
// A narrow monotonic primary key keeps the clustered index append-only and
// keeps every secondary index entry small, which suits high-growth tables.
//
// Key behavior differences from Base:
//   - SetID never generates an ID; the database assigns one on insert.
//   - GetID returns "" while the ID is unset (0) so framework emptiness
//     checks such as seeding and not-found detection keep working.
//
// Caveats:
//   - Seed records passed to model.Register must set an explicit ID or rely
//     on a unique index; idempotent seeding depends on conflicting keys.
//   - Updating a record whose ID is unset fails with database.ErrIDRequired;
//     use database.Upsert for insert-or-update semantics.
//
// created_at/updated_at follow the same NOT NULL, no-database-default
// contract as Base: every writer provides both explicitly, in UTC.
//
// The uuid columns take a size instead of an explicit char type for the same
// reason as Base: postgres blank-pads char values on read. The note belongs
// here rather than above the field, for the reason Base's comment spells out.
type AutoBase struct {
	ID uint64 `json:"id" gorm:"primaryKey;autoIncrement" query:"id" url:"-"` // Auto-increment identifier assigned by the database

	CreatedBy string         `json:"created_by,omitempty" gorm:"size:36" query:"created_by" url:"-"` // UUIDv7 user ID who created the record
	UpdatedBy string         `json:"updated_by,omitempty" gorm:"size:36" query:"updated_by" url:"-"` // UUIDv7 user ID who last updated the record
	CreatedAt time.Time      `json:"created_at,omitzero" gorm:"not null" query:"-" url:"-"`          // Timestamp when the record was created
	UpdatedAt time.Time      `json:"updated_at,omitzero" gorm:"not null" query:"-" url:"-"`          // Timestamp when the record was last updated
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index" query:"-" url:"-"`                               // Timestamp when the record was deleted
}

func (b *AutoBase) TableName() string        { return "" } // Business models must override with an explicit table name
func (b *AutoBase) GetCreatedBy() string     { return b.CreatedBy }
func (b *AutoBase) GetUpdatedBy() string     { return b.UpdatedBy }
func (b *AutoBase) GetCreatedAt() time.Time  { return b.CreatedAt }
func (b *AutoBase) GetUpdatedAt() time.Time  { return b.UpdatedAt }
func (b *AutoBase) SetCreatedBy(s string)    { b.CreatedBy = s }
func (b *AutoBase) SetUpdatedBy(s string)    { b.UpdatedBy = s }
func (b *AutoBase) SetCreatedAt(t time.Time) { b.CreatedAt = t }
func (b *AutoBase) SetUpdatedAt(t time.Time) { b.UpdatedAt = t }
func (b *AutoBase) Expands() []string        { return nil }
func (b *AutoBase) Purge() bool              { return false } // Default to soft delete

// GetID returns the decimal form of the ID, or "" while the ID is unset.
func (b *AutoBase) GetID() string {
	if b.ID == 0 {
		return ""
	}
	return strconv.FormatUint(b.ID, 10)
}

// SetID parses the given decimal id into the ID field. Unlike Base it never
// generates an ID: without a parsable argument the ID stays unset and the
// database assigns one on insert. An already set ID is kept.
func (b *AutoBase) SetID(id ...string) {
	if b.ID != 0 {
		return
	}
	if len(id) == 0 || len(id[0]) == 0 {
		return
	}
	if v, err := strconv.ParseUint(id[0], 10, 64); err == nil {
		b.ID = v
	}
}

func (b *AutoBase) ClearID() { b.ID = 0 }

func (*AutoBase) CreateBefore(context.Context) error { return nil }
func (*AutoBase) CreateAfter(context.Context) error  { return nil }
func (*AutoBase) DeleteBefore(context.Context) error { return nil }
func (*AutoBase) DeleteAfter(context.Context) error  { return nil }
func (*AutoBase) UpdateBefore(context.Context) error { return nil }
func (*AutoBase) UpdateAfter(context.Context) error  { return nil }
func (*AutoBase) ListBefore(context.Context) error   { return nil }
func (*AutoBase) ListAfter(context.Context) error    { return nil }
func (*AutoBase) GetBefore(context.Context) error    { return nil }
func (*AutoBase) GetAfter(context.Context) error     { return nil }
