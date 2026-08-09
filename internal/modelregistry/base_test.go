package modelregistry_test

import (
	"sync"
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"
)

type BaseSample struct {
	Name string `json:"name,omitempty"`

	modelregistry.Base
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
	s, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
	require.NoError(t, err)
	for _, name := range []string{"CreatedAt", "UpdatedAt"} {
		field := s.LookUpField(name)
		require.NotNil(t, field, name)
		require.True(t, field.NotNull, name)
		require.Empty(t, field.DefaultValue, name)
	}
}
