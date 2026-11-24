package model_test

import (
	"testing"

	"github.com/hydroan/gst/internal/model"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

type AutoRecord struct {
	Name string `json:"name,omitempty"`

	model.AutoBase
}

func TestAutoBaseImplementsModel(t *testing.T) {
	require.Implements(t, (*types.Model)(nil), &model.AutoBase{})
	require.True(t, model.IsTableModel[*AutoRecord]())
	require.False(t, model.IsEmpty[AutoRecord]())
}

// TestAutoBaseTimestampColumnsNotNull pins the same NOT NULL timestamp
// contract as Base for the auto-increment variant.
func TestAutoBaseTimestampColumnsNotNull(t *testing.T) {
	requireTimestampColumnsNotNull(t, &AutoRecord{})
}

func TestAutoBaseGetID(t *testing.T) {
	b := new(model.AutoBase)
	// An unset ID reports empty so framework emptiness checks keep working.
	require.Empty(t, b.GetID())

	b.ID = 42
	require.Equal(t, "42", b.GetID())
}

func TestAutoBaseSetID(t *testing.T) {
	b := new(model.AutoBase)

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
	b := &model.AutoBase{ID: 7}
	b.ClearID()
	require.Zero(t, b.ID)
	require.Empty(t, b.GetID())
}
