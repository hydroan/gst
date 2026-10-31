package modelregistry_test

import (
	"context"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
)

// TestBasesAnswerTheStampsTheyHold pins the stamp accessors of the model
// bases: Base and AutoBase answer the creator, the updater and the times
// their fields hold and take what the setters write, declare no expansion
// and soft deletion, and their hooks accept every phase; Empty, which no
// row backs, holds nothing: its getters answer the zero values, its
// setters, SetID and ClearID change nothing, and its hooks accept every
// phase as well.
func TestBasesAnswerTheStampsTheyHold(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	later := at.Add(time.Hour)

	type stamped interface {
		GetCreatedBy() string
		GetUpdatedBy() string
		GetCreatedAt() time.Time
		GetUpdatedAt() time.Time
		SetCreatedBy(string)
		SetUpdatedBy(string)
		SetCreatedAt(time.Time)
		SetUpdatedAt(time.Time)
		Expands() []string
		Purge() bool
	}
	for _, tt := range []struct {
		name string
		base stamped
	}{
		{name: "Base", base: &modelregistry.Base{}},
		{name: "AutoBase", base: &modelregistry.AutoBase{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.base.SetCreatedBy("u-1")
			tt.base.SetUpdatedBy("u-2")
			tt.base.SetCreatedAt(at)
			tt.base.SetUpdatedAt(later)
			require.Equal(t, "u-1", tt.base.GetCreatedBy())
			require.Equal(t, "u-2", tt.base.GetUpdatedBy())
			require.Equal(t, at, tt.base.GetCreatedAt())
			require.Equal(t, later, tt.base.GetUpdatedAt())
			require.Nil(t, tt.base.Expands(), "a base declares no expansion of its own")
			require.False(t, tt.base.Purge(), "a base deletes softly")
		})
	}

	empty := &modelregistry.Empty{}
	empty.SetCreatedBy("u-1")
	empty.SetUpdatedBy("u-2")
	empty.SetCreatedAt(at)
	empty.SetUpdatedAt(later)
	empty.SetID("7")
	require.Empty(t, empty.GetCreatedBy())
	require.Empty(t, empty.GetUpdatedBy())
	require.True(t, empty.GetCreatedAt().IsZero())
	require.True(t, empty.GetUpdatedAt().IsZero())
	require.Empty(t, empty.GetID(), "a virtual model has no id to set")
	empty.ClearID()
	require.Empty(t, empty.GetID())
	require.Empty(t, empty.TableName(), "a virtual model has no table")
	require.Nil(t, empty.Expands())
	require.False(t, empty.Purge())
	require.True(t, modelregistry.IsVirtual(empty))

	ctx := context.Background()
	base, auto := &modelregistry.Base{}, &modelregistry.AutoBase{}
	for name, hooks := range map[string][]func(context.Context) error{
		"Base": {
			base.CreateBefore, base.CreateAfter, base.DeleteBefore, base.DeleteAfter,
			base.UpdateBefore, base.UpdateAfter, base.ListBefore, base.ListAfter,
			base.GetBefore, base.GetAfter,
		},
		"AutoBase": {
			auto.CreateBefore, auto.CreateAfter, auto.DeleteBefore, auto.DeleteAfter,
			auto.UpdateBefore, auto.UpdateAfter, auto.ListBefore, auto.ListAfter,
			auto.GetBefore, auto.GetAfter,
		},
		"Empty": {
			empty.CreateBefore, empty.CreateAfter, empty.DeleteBefore, empty.DeleteAfter,
			empty.UpdateBefore, empty.UpdateAfter, empty.ListBefore, empty.ListAfter,
			empty.GetBefore, empty.GetAfter,
		},
	} {
		for _, hook := range hooks {
			require.NoError(t, hook(ctx), "a hook of %s accepts every phase", name)
		}
	}
}
