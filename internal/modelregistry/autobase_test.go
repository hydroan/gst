package modelregistry_test

import (
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

type AutoUser struct {
	Name string `json:"name,omitempty"`

	modelregistry.AutoBase
}

func TestAutoBaseImplementsModel(t *testing.T) {
	require.Implements(t, (*types.Model)(nil), &modelregistry.AutoBase{})
	require.True(t, modelregistry.IsValid[*AutoUser]())
	require.False(t, modelregistry.IsEmpty[AutoUser]())
}

// TestAutoBaseTimestampColumnsNotNull pins the same NOT NULL timestamp
// contract as Base for the auto-increment variant.
func TestAutoBaseTimestampColumnsNotNull(t *testing.T) {
	requireTimestampColumnsNotNull(t, &AutoUser{})
}

func TestAutoBaseGetID(t *testing.T) {
	b := new(modelregistry.AutoBase)
	// An unset ID reports empty so framework emptiness checks keep working.
	require.Empty(t, b.GetID())

	b.ID = 42
	require.Equal(t, "42", b.GetID())
}

func TestAutoBaseSetID(t *testing.T) {
	b := new(modelregistry.AutoBase)

	// Without arguments the ID stays unset; the database assigns it on insert.
	b.SetID()
	require.Zero(t, b.ID)

	// An empty argument keeps the ID unset as well.
	b.SetID("")
	require.Zero(t, b.ID)

	// A non-numeric argument keeps the ID unset.
	b.SetID("abc")
	require.Zero(t, b.ID)

	// A numeric argument sets the ID.
	b.SetID("123")
	require.Equal(t, uint64(123), b.ID)

	// An existing ID is never overwritten.
	b.SetID("456")
	require.Equal(t, uint64(123), b.ID)
}

func TestAutoBaseClearID(t *testing.T) {
	b := &modelregistry.AutoBase{ID: 7}
	b.ClearID()
	require.Zero(t, b.ID)
	require.Empty(t, b.GetID())
}
