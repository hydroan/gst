package modelregistry_test

import (
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
)

type versionedRecord struct {
	Name    string
	Version modelregistry.Version `json:"version,omitempty" gorm:"not null;default:1"`

	modelregistry.Base
}

func (*versionedRecord) TableName() string { return "versioned_records" }

type taggedVersionRecord struct {
	Revision modelregistry.Version `json:"rev,omitempty" gorm:"column:rev;not null;default:1"`

	modelregistry.Base
}

func (*taggedVersionRecord) TableName() string { return "tagged_version_records" }

type plainRecord struct {
	Name string

	modelregistry.Base
}

func (*plainRecord) TableName() string { return "plain_records" }

func TestVersionFieldDetection(t *testing.T) {
	require.True(t, modelregistry.IsVersioned(&versionedRecord{}))
	require.False(t, modelregistry.IsVersioned(&plainRecord{}))
	require.False(t, modelregistry.IsVersioned(nil))

	column, ok := modelregistry.VersionColumn(&versionedRecord{})
	require.True(t, ok)
	require.Equal(t, "version", column, "the naming strategy renders the field name")

	column, ok = modelregistry.VersionColumn(&taggedVersionRecord{})
	require.True(t, ok)
	require.Equal(t, "rev", column, "an explicit column tag wins")

	_, ok = modelregistry.VersionColumn(&plainRecord{})
	require.False(t, ok)
}

func TestVersionDeclarationEnforcement(t *testing.T) {
	// An embedded Version would not be recognized and the lock would
	// silently not engage; first touch fails instead.
	type embeddedVersionRecord struct {
		modelregistry.Version

		modelregistry.Base
	}
	require.PanicsWithValue(t,
		"model modelregistry_test.embeddedVersionRecord embeds model.Version; optimistic locking requires a named field: Version model.Version `json:\"version,omitempty\" gorm:\"not null;default:1\"` (an embedded Version is not recognized and the lock would silently not engage)",
		func() { modelregistry.IsVersioned(&embeddedVersionRecord{}) })

	// A missing default:1 would backfill adopted rows to zero and lock them
	// out of Update; a missing not null weakens the column contract.
	type missingDefaultRecord struct {
		Version modelregistry.Version `json:"version,omitempty" gorm:"not null"`

		modelregistry.Base
	}
	require.Panics(t, func() { modelregistry.IsVersioned(&missingDefaultRecord{}) })

	type bareVersionRecord struct {
		Version modelregistry.Version

		modelregistry.Base
	}
	require.Panics(t, func() { modelregistry.IsVersioned(&bareVersionRecord{}) })

	// A json tag without omitempty serializes an unset version as an
	// explicit zero the write paths reject; json:"-" hides the version
	// clients must hand back. Both fail on first touch.
	type missingOmitemptyRecord struct {
		Version modelregistry.Version `json:"version" gorm:"not null;default:1"`

		modelregistry.Base
	}
	require.Panics(t, func() { modelregistry.IsVersioned(&missingOmitemptyRecord{}) })

	type hiddenVersionRecord struct {
		Version modelregistry.Version `json:"-" gorm:"not null;default:1"`

		modelregistry.Base
	}
	require.Panics(t, func() { modelregistry.IsVersioned(&hiddenVersionRecord{}) })

	// The compliant shape passes, whitespace and case tolerated, and a bare
	// json:",omitempty" (wire name from the field) is compliant too.
	type tolerantTagRecord struct {
		Version modelregistry.Version `json:",omitempty" gorm:"NOT  NULL; default: 1"`

		modelregistry.Base
	}
	require.True(t, modelregistry.IsVersioned(&tolerantTagRecord{}))
}

func TestVersionValueRoundTrip(t *testing.T) {
	record := &versionedRecord{Version: 7}

	v, ok := modelregistry.VersionValue(record)
	require.True(t, ok)
	require.EqualValues(t, 7, v)

	modelregistry.SetVersionValue(record, 9)
	require.EqualValues(t, 9, record.Version)

	// Models without the field answer false and ignore writes.
	plain := &plainRecord{}
	_, ok = modelregistry.VersionValue(plain)
	require.False(t, ok)
	modelregistry.SetVersionValue(plain, 3)

	// A nil model answers the zero value instead of panicking.
	var nilRecord *versionedRecord
	v, ok = modelregistry.VersionValue(nilRecord)
	require.False(t, ok)
	require.Zero(t, v)
	modelregistry.SetVersionValue(nilRecord, 3)
}
