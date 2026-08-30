package modelregistry

import (
	"strings"
	"testing"

	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

// The samples below all map to one table on purpose: each differs from
// fingerprintSample in a single declaration, so a declaration that failed to
// reach the rendering is the only thing that could make two of them read the
// same.

type fingerprintSample struct {
	Name string `gorm:"size:64"`

	Base
}

func (*fingerprintSample) TableName() string { return "fingerprint_samples" }

func (*fingerprintSample) Indexes() []Index {
	return []Index{{Fields: []string{"Name"}}}
}

// fingerprintSampleWithoutField drops a column. It is the case a migration
// cannot repair afterwards: it never drops a column a model stopped
// declaring, so the fingerprint has to be what notices.
type fingerprintSampleWithoutField struct {
	Base
}

func (*fingerprintSampleWithoutField) TableName() string { return "fingerprint_samples" }

func (*fingerprintSampleWithoutField) Indexes() []Index {
	return []Index{{Fields: []string{"Name"}}}
}

// fingerprintSampleWiderField keeps the column and moves its definition.
type fingerprintSampleWiderField struct {
	Name string `gorm:"size:128"`

	Base
}

func (*fingerprintSampleWiderField) TableName() string { return "fingerprint_samples" }

func (*fingerprintSampleWiderField) Indexes() []Index {
	return []Index{{Fields: []string{"Name"}}}
}

// fingerprintSampleWithoutIndex drops an index, the other change a migration
// leaves in place once it exists.
type fingerprintSampleWithoutIndex struct {
	Name string `gorm:"size:64"`

	Base
}

func (*fingerprintSampleWithoutIndex) TableName() string { return "fingerprint_samples" }

// fingerprintSampleUniqueIndex keeps the index and makes it unique.
type fingerprintSampleUniqueIndex struct {
	Name string `gorm:"size:64"`

	Base
}

func (*fingerprintSampleUniqueIndex) TableName() string { return "fingerprint_samples" }

func (*fingerprintSampleUniqueIndex) Indexes() []Index {
	return []Index{{Fields: []string{"Name"}, Unique: true}}
}

// fingerprintSampleOtherTable changes nothing but the table it maps to.
type fingerprintSampleOtherTable struct {
	Name string `gorm:"size:64"`

	Base
}

func (*fingerprintSampleOtherTable) TableName() string { return "other_fingerprint_samples" }

func (*fingerprintSampleOtherTable) Indexes() []Index {
	return []Index{{Fields: []string{"Name"}}}
}

func TestModelDeclaration(t *testing.T) {
	baseline := modelDeclaration(&fingerprintSample{})

	t.Run("the_same_model_declares_the_same_thing_every_time", func(t *testing.T) {
		require.Equal(t, baseline, modelDeclaration(&fingerprintSample{}))
	})

	t.Run("every_declaration_a_migration_cannot_repair_shows_up", func(t *testing.T) {
		for name, model := range map[string]types.Model{
			"a dropped field":      &fingerprintSampleWithoutField{},
			"a dropped index":      &fingerprintSampleWithoutIndex{},
			"a widened field":      &fingerprintSampleWiderField{},
			"an index made unique": &fingerprintSampleUniqueIndex{},
			"another table":        &fingerprintSampleOtherTable{},
		} {
			require.NotEqual(t, baseline, modelDeclaration(model), name)
		}
	})

	t.Run("fields_promoted_from_an_embedded_struct_are_declared_too", func(t *testing.T) {
		// Base carries the primary key and the timestamps, which are columns
		// like any other; a declaration blind to them would let a change to
		// them pass for no change at all.
		require.Contains(t, baseline, "field=ID")
		require.Contains(t, baseline, "field=CreatedAt")
	})

	t.Run("the_table_name_is_declared", func(t *testing.T) {
		require.Contains(t, baseline, "table=fingerprint_samples")
	})
}

func TestSchemaFingerprint(t *testing.T) {
	t.Run("nothing_registered_has_no_fingerprint", func(t *testing.T) {
		registeredMu.Lock()
		kept := registeredModels
		registeredModels = nil
		registeredMu.Unlock()
		t.Cleanup(func() {
			registeredMu.Lock()
			registeredModels = kept
			registeredMu.Unlock()
		})

		require.Empty(t, SchemaFingerprint())
	})

	t.Run("the_same_models_fingerprint_the_same_however_they_arrived", func(t *testing.T) {
		registeredMu.Lock()
		kept := registeredModels
		registeredModels = []types.Model{&fingerprintSample{}, &fingerprintSampleOtherTable{}}
		registeredMu.Unlock()
		defer func() {
			registeredMu.Lock()
			registeredModels = kept
			registeredMu.Unlock()
		}()

		inOrder := SchemaFingerprint()
		require.NotEmpty(t, inOrder)
		require.Len(t, strings.TrimSpace(inOrder), len(inOrder), "the fingerprint goes into a database name")

		registeredMu.Lock()
		registeredModels = []types.Model{&fingerprintSampleOtherTable{}, &fingerprintSample{}}
		registeredMu.Unlock()

		require.Equal(t, inOrder, SchemaFingerprint())
	})

	t.Run("a_changed_model_changes_the_fingerprint", func(t *testing.T) {
		registeredMu.Lock()
		kept := registeredModels
		registeredModels = []types.Model{&fingerprintSample{}}
		registeredMu.Unlock()
		defer func() {
			registeredMu.Lock()
			registeredModels = kept
			registeredMu.Unlock()
		}()

		before := SchemaFingerprint()

		registeredMu.Lock()
		registeredModels = []types.Model{&fingerprintSampleWithoutField{}}
		registeredMu.Unlock()

		require.NotEqual(t, before, SchemaFingerprint())
	})
}
