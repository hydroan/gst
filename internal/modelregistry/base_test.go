package modelregistry_test

import (
	"sync"
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
	gormschema "gorm.io/gorm/schema"
)

type BaseSample struct {
	Name string `json:"name,omitempty"`

	modelregistry.Base
}

// TestBaseAuditorColumnsHoldAUsername pins the width of created_by and
// updated_by on Base and AutoBase: 191 characters, room for a username of
// any source the framework authenticates, within what MySQL indexes.
func TestBaseAuditorColumnsHoldAUsername(t *testing.T) {
	for _, model := range []any{&BaseSample{}, &AutoUser{}} {
		s, err := gormschema.Parse(model, &sync.Map{}, gormschema.NamingStrategy{})
		require.NoError(t, err)
		for _, name := range []string{"CreatedBy", "UpdatedBy"} {
			field := s.LookUpField(name)
			require.NotNil(t, field, name)
			require.Equal(t, 191, field.Size, "%T.%s", model, name)
		}
	}
}

// TestBaseTimestampColumnsNotNull pins the schema contract of the framework
// bookkeeping timestamps: created_at/updated_at are NOT NULL and carry no
// database default, so every writer must provide both values explicitly.
func TestBaseTimestampColumnsNotNull(t *testing.T) {
	requireTimestampColumnsNotNull(t, &BaseSample{})
}

// requireTimestampColumnsNotNull asserts that created_at/updated_at parse as
// NOT NULL columns without a database default on the given model.
func requireTimestampColumnsNotNull(t *testing.T, model any) {
	t.Helper()
	s, err := gormschema.Parse(model, &sync.Map{}, gormschema.NamingStrategy{})
	require.NoError(t, err)
	for _, name := range []string{"CreatedAt", "UpdatedAt"} {
		field := s.LookUpField(name)
		require.NotNil(t, field, name)
		require.True(t, field.NotNull, name)
		require.Empty(t, field.DefaultValue, name)
	}
}
