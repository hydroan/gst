package modelregistry

import (
	"context"
	"strconv"
	"time"

	"github.com/hydroan/gst/types"
	"go.uber.org/zap/zapcore"
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
//   - The comma-separated multi-ID query trick supported by Base's string ID
//     does not apply to the integer ID.
//   - Updating a record whose ID is unset inserts a new row, mirroring how
//     Base generates a fresh UUID for records without an ID.
type AutoBase struct {
	ID uint64 `json:"id" gorm:"primaryKey;autoIncrement" query:"id" url:"-"` // Auto-increment identifier assigned by the database

	CreatedBy string         `json:"created_by,omitempty" gorm:"type:char(36)" query:"created_by" url:"-"` // UUIDv7 user ID who created the record
	UpdatedBy string         `json:"updated_by,omitempty" gorm:"type:char(36)" query:"updated_by" url:"-"` // UUIDv7 user ID who last updated the record
	CreatedAt time.Time      `json:"created_at,omitzero" query:"-" url:"-"`                                // Timestamp when the record was created
	UpdatedAt time.Time      `json:"updated_at,omitzero" query:"-" url:"-"`                                // Timestamp when the record was last updated
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index" query:"-" url:"-"`                                     // Timestamp when the record was deleted
}

func (b *AutoBase) GetTableName() string       { return "" }
func (b *AutoBase) GetCreatedBy() string       { return b.CreatedBy }
func (b *AutoBase) GetUpdatedBy() string       { return b.UpdatedBy }
func (b *AutoBase) GetCreatedAt() time.Time    { return b.CreatedAt }
func (b *AutoBase) GetUpdatedAt() time.Time    { return b.UpdatedAt }
func (b *AutoBase) SetCreatedBy(s string)      { b.CreatedBy = s }
func (b *AutoBase) SetUpdatedBy(s string)      { b.UpdatedBy = s }
func (b *AutoBase) SetCreatedAt(t time.Time)   { b.CreatedAt = t }
func (b *AutoBase) SetUpdatedAt(t time.Time)   { b.UpdatedAt = t }
func (b *AutoBase) Expands() []string          { return nil }
func (b *AutoBase) Excludes() map[string][]any { return nil }
func (b *AutoBase) Purge() bool                { return false } // Default to soft delete

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

func (b *AutoBase) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if b == nil {
		return nil
	}
	enc.AddUint64("id", b.ID)
	enc.AddString("created_by", b.CreatedBy)
	enc.AddString("updated_by", b.UpdatedBy)
	return nil
}

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
