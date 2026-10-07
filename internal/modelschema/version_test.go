package modelschema_test

import (
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/stretchr/testify/require"
)

type versionedRecord struct {
	Name    string
	Version modelschema.Version `json:"version,omitempty" gorm:"not null;default:1"`

	modelregistry.Base
}

func (*versionedRecord) TableName() string { return "versioned_records" }

type taggedVersionRecord struct {
	Revision modelschema.Version `json:"rev,omitempty" gorm:"column:rev;not null;default:1"`

	modelregistry.Base
}

func (*taggedVersionRecord) TableName() string { return "tagged_version_records" }

// capitalizedTagVersionRecord spells the column tag key as gorm reads it too,
// case-insensitively: gorm builds and writes the column ver.
type capitalizedTagVersionRecord struct {
	Version modelschema.Version `json:"version,omitempty" gorm:"Column:ver;not null;default:1"`

	modelregistry.Base
}

func (*capitalizedTagVersionRecord) TableName() string { return "capitalized_tag_version_records" }

type plainRecord struct {
	Name string

	modelregistry.Base
}

func (*plainRecord) TableName() string { return "plain_records" }

func TestVersionFieldDetection(t *testing.T) {
	require.True(t, modelschema.IsVersioned(&versionedRecord{}))
	require.False(t, modelschema.IsVersioned(&plainRecord{}))
	require.False(t, modelschema.IsVersioned(nil))

	column, ok := modelschema.VersionColumn(&versionedRecord{})
	require.True(t, ok)
	require.Equal(t, "version", column, "the naming strategy renders the field name")

	column, ok = modelschema.VersionColumn(&taggedVersionRecord{})
	require.True(t, ok)
	require.Equal(t, "rev", column, "an explicit column tag wins")

	column, ok = modelschema.VersionColumn(&capitalizedTagVersionRecord{})
	require.True(t, ok)
	require.Equal(t, "ver", column, "the column is the one gorm resolves, the tag key read case-insensitively")

	_, ok = modelschema.VersionColumn(&plainRecord{})
	require.False(t, ok)
}

func TestVersionDeclarationEnforcement(t *testing.T) {
	// An embedded Version would not be recognized and the lock would
	// silently not engage; first touch fails instead.
	type embeddedVersionRecord struct {
		modelschema.Version

		modelregistry.Base
	}
	require.PanicsWithValue(t,
		"model modelschema_test.embeddedVersionRecord embeds model.Version; optimistic locking requires a named field: Version model.Version `json:\"version,omitempty\" gorm:\"not null;default:1\"` (an embedded Version is not recognized and the lock would silently not engage)",
		func() { modelschema.IsVersioned(&embeddedVersionRecord{}) })

	// A missing default:1 would backfill adopted rows to zero and lock them
	// out of Update; a missing not null weakens the column contract.
	type missingDefaultRecord struct {
		Version modelschema.Version `json:"version,omitempty" gorm:"not null"`

		modelregistry.Base
	}
	require.Panics(t, func() { modelschema.IsVersioned(&missingDefaultRecord{}) })

	type bareVersionRecord struct {
		Version modelschema.Version

		modelregistry.Base
	}
	require.Panics(t, func() { modelschema.IsVersioned(&bareVersionRecord{}) })

	// A json tag without omitempty serializes an unset version as an
	// explicit zero the write paths reject; json:"-" hides the version
	// clients must hand back. Both fail on first touch.
	type missingOmitemptyRecord struct {
		Version modelschema.Version `json:"version" gorm:"not null;default:1"`

		modelregistry.Base
	}
	require.Panics(t, func() { modelschema.IsVersioned(&missingOmitemptyRecord{}) })

	type hiddenVersionRecord struct {
		Version modelschema.Version `json:"-" gorm:"not null;default:1"`

		modelregistry.Base
	}
	require.Panics(t, func() { modelschema.IsVersioned(&hiddenVersionRecord{}) })

	// The compliant shape passes, whitespace and case tolerated, and a bare
	// json:",omitempty" (wire name from the field) is compliant too.
	type tolerantTagRecord struct {
		Version modelschema.Version `json:",omitempty" gorm:"NOT  NULL; default: 1"`

		modelregistry.Base
	}
	require.True(t, modelschema.IsVersioned(&tolerantTagRecord{}))
}

func TestVersionValueRoundTrip(t *testing.T) {
	record := &versionedRecord{Version: 7}

	v, ok := modelschema.VersionValue(record)
	require.True(t, ok)
	require.EqualValues(t, 7, v)

	modelschema.SetVersionValue(record, 9)
	require.EqualValues(t, 9, record.Version)

	// Models without the field answer false and ignore writes.
	plain := &plainRecord{}
	_, ok = modelschema.VersionValue(plain)
	require.False(t, ok)
	modelschema.SetVersionValue(plain, 3)

	// A nil model answers the zero value instead of panicking.
	var nilRecord *versionedRecord
	v, ok = modelschema.VersionValue(nilRecord)
	require.False(t, ok)
	require.Zero(t, v)
	modelschema.SetVersionValue(nilRecord, 3)
}
