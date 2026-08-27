package modelregistry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type versionedRecord struct {
	Name    string
	Version Version `json:"version,omitempty" gorm:"not null;default:1"`

	Base
}

func (*versionedRecord) TableName() string { return "versioned_records" }

type taggedVersionRecord struct {
	Revision Version `json:"rev,omitempty" gorm:"column:rev;not null;default:1"`

	Base
}

func (*taggedVersionRecord) TableName() string { return "tagged_version_records" }

type plainRecord struct {
	Name string

	Base
}

func (*plainRecord) TableName() string { return "plain_records" }

func TestVersionFieldDetection(t *testing.T) {
	require.True(t, IsVersioned(&versionedRecord{}))
	require.False(t, IsVersioned(&plainRecord{}))
	require.False(t, IsVersioned(nil))

	column, ok := VersionColumn(&versionedRecord{})
	require.True(t, ok)
	require.Equal(t, "version", column, "the naming strategy renders the field name")

	column, ok = VersionColumn(&taggedVersionRecord{})
	require.True(t, ok)
	require.Equal(t, "rev", column, "an explicit column tag wins")

	_, ok = VersionColumn(&plainRecord{})
	require.False(t, ok)
}

func TestVersionDeclarationEnforcement(t *testing.T) {
	// An embedded Version would not be recognized and the lock would
	// silently not engage; first touch fails instead.
	type embeddedVersionRecord struct {
		Version

		Base
	}
	require.PanicsWithValue(t,
		"model modelregistry.embeddedVersionRecord embeds model.Version; optimistic locking requires a named field: Version model.Version `json:\"version,omitempty\" gorm:\"not null;default:1\"` (an embedded Version is not recognized and the lock would silently not engage)",
		func() { IsVersioned(&embeddedVersionRecord{}) })

	// A missing default:1 would backfill adopted rows to zero and lock them
	// out of Update; a missing not null weakens the column contract.
	type missingDefaultRecord struct {
		Version Version `json:"version,omitempty" gorm:"not null"`

		Base
	}
	require.Panics(t, func() { IsVersioned(&missingDefaultRecord{}) })

	type bareVersionRecord struct {
		Version Version

		Base
	}
	require.Panics(t, func() { IsVersioned(&bareVersionRecord{}) })

	// A json tag without omitempty serializes an unset version as an
	// explicit zero the write paths reject; json:"-" hides the version
	// clients must hand back. Both fail on first touch.
	type missingOmitemptyRecord struct {
		Version Version `json:"version" gorm:"not null;default:1"`

		Base
	}
	require.Panics(t, func() { IsVersioned(&missingOmitemptyRecord{}) })

	type hiddenVersionRecord struct {
		Version Version `json:"-" gorm:"not null;default:1"`

		Base
	}
	require.Panics(t, func() { IsVersioned(&hiddenVersionRecord{}) })

	// The compliant shape passes, whitespace and case tolerated, and a bare
	// json:",omitempty" (wire name from the field) is compliant too.
	type tolerantTagRecord struct {
		Version Version `json:",omitempty" gorm:"NOT  NULL; default: 1"`

		Base
	}
	require.True(t, IsVersioned(&tolerantTagRecord{}))
}

func TestVersionValueRoundTrip(t *testing.T) {
	record := &versionedRecord{Version: 7}

	v, ok := VersionValue(record)
	require.True(t, ok)
	require.EqualValues(t, 7, v)

	SetVersionValue(record, 9)
	require.EqualValues(t, 9, record.Version)

	// Models without the field answer false and ignore writes.
	plain := &plainRecord{}
	_, ok = VersionValue(plain)
	require.False(t, ok)
	SetVersionValue(plain, 3)

	// A nil model answers the zero value instead of panicking.
	var nilRecord *versionedRecord
	v, ok = VersionValue(nilRecord)
	require.False(t, ok)
	require.Zero(t, v)
	SetVersionValue(nilRecord, 3)
}
