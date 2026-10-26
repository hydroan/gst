package modelschema_test

import (
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/stretchr/testify/require"
)

// fingerprintSample and its variants each differ from it in a single
// declaration: fingerprintSampleWithoutField drops a column, which no
// migration repairs, and fingerprintSampleOtherTable maps to another table.

type fingerprintSample struct {
	Name string `gorm:"size:64"`

	modelregistry.Base
}

func (*fingerprintSample) TableName() string { return "fingerprint_samples" }

func (*fingerprintSample) Indexes() []modelschema.Index {
	return []modelschema.Index{{Fields: []string{"Name"}}}
}

type fingerprintSampleWithoutField struct {
	modelregistry.Base
}

func (*fingerprintSampleWithoutField) TableName() string { return "fingerprint_samples" }

func (*fingerprintSampleWithoutField) Indexes() []modelschema.Index {
	return []modelschema.Index{{Fields: []string{"Name"}}}
}

type fingerprintSampleOtherTable struct {
	Name string `gorm:"size:64"`

	modelregistry.Base
}

func (*fingerprintSampleOtherTable) TableName() string { return "other_fingerprint_samples" }

func (*fingerprintSampleOtherTable) Indexes() []modelschema.Index {
	return []modelschema.Index{{Fields: []string{"Name"}}}
}

func TestFingerprint(t *testing.T) {
	t.Run("no_models_have_no_fingerprint", func(t *testing.T) {
		require.Empty(t, modelschema.Fingerprint(nil))
	})

	t.Run("the_same_models_fingerprint_the_same_however_they_arrived", func(t *testing.T) {
		inOrder := modelschema.Fingerprint([]any{&fingerprintSample{}, &fingerprintSampleOtherTable{}})
		require.NotEmpty(t, inOrder)
		require.Len(t, strings.TrimSpace(inOrder), len(inOrder), "the fingerprint goes into a database name")

		require.Equal(t, inOrder, modelschema.Fingerprint([]any{&fingerprintSampleOtherTable{}, &fingerprintSample{}}))
	})

	t.Run("a_changed_model_changes_the_fingerprint", func(t *testing.T) {
		before := modelschema.Fingerprint([]any{&fingerprintSample{}})

		require.NotEqual(t, before, modelschema.Fingerprint([]any{&fingerprintSampleWithoutField{}}))
	})
}
