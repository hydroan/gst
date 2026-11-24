package model_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/model"
	"github.com/hydroan/gst/internal/types"
	"github.com/stoewer/go-strcase"
	"github.com/stretchr/testify/require"
)

// hookFreeSample carries no hook overrides: every hook is the promoted
// framework no-op.
type hookFreeSample struct {
	Name string `json:"name,omitempty"`

	model.Base
}

// createHookSample overrides one hook of the create pair only.
type createHookSample struct {
	model.Base
}

func (*createHookSample) CreateBefore(context.Context) error { return nil }

// updateDeleteHookSample overrides one hook of the update pair and one hook
// of the delete pair, leaving the create pair promoted.
type updateDeleteHookSample struct {
	model.Base
}

func (*updateDeleteHookSample) UpdateAfter(context.Context) error  { return nil }
func (*updateDeleteHookSample) DeleteBefore(context.Context) error { return nil }

// readHookSample overrides one hook of the get pair and one hook of the list
// pair, leaving every write hook promoted.
type readHookSample struct {
	model.Base
}

func (*readHookSample) GetBefore(context.Context) error { return nil }
func (*readHookSample) ListAfter(context.Context) error { return nil }

// hookFreeIntermediate embeds the framework base without overriding anything,
// so models embedding it still carry only no-op hooks.
type hookFreeIntermediate struct {
	model.Base
}

// promotedPlainSample promotes its hooks through an intermediate type that
// never overrides them; the chain still ends at the framework no-ops.
type promotedPlainSample struct {
	hookFreeIntermediate
}

// hookedIntermediate overrides a create hook on an intermediate type that
// models then embed. The override is promoted into the model, so the model
// must count as overriding even though its own wrapper is compiler-generated.
type hookedIntermediate struct {
	model.Base
}

func (*hookedIntermediate) CreateBefore(context.Context) error { return nil }

// promotedHookSample promotes a real create-hook implementation from an
// intermediate type rather than declaring one itself.
type promotedHookSample struct {
	hookedIntermediate
}

// autoBaseSample embeds AutoBase instead of Base; its hooks are the promoted
// framework no-ops all the same.
type autoBaseSample struct {
	model.AutoBase
}

// modelHookPhases derives the hook phases from the lifecycle hooks the model
// contract declares, mirroring Phase.Name in the other direction, so
// the tracked set is checked against the contract itself rather than against
// a second copy of the phase list.
func modelHookPhases(t *testing.T) []apiaction.Phase {
	t.Helper()
	contract := reflect.TypeFor[types.Model]()
	phases := make([]apiaction.Phase, 0, contract.NumMethod())
	for method := range contract.Methods() {
		if !strings.HasSuffix(method.Name, "Before") && !strings.HasSuffix(method.Name, "After") {
			continue
		}
		phase := apiaction.Phase(strcase.SnakeCase(method.Name))
		require.Equal(t, method.Name, phase.Name(), "phase derived from %s must round-trip", method.Name)
		phases = append(phases, phase)
	}
	require.NotEmpty(t, phases, "the model contract must declare lifecycle hooks")
	return phases
}

// requireOverridesNoHook asserts that neither the pair-level nor the per-hook
// detection reports an override for m.
func requireOverridesNoHook(t *testing.T, m any) {
	t.Helper()
	require.False(t, model.OverridesCreateHooks(m))
	require.False(t, model.OverridesUpdateHooks(m))
	require.False(t, model.OverridesDeleteHooks(m))
	for _, phase := range modelHookPhases(t) {
		require.False(t, model.OverridesHook(m, phase), "hook %s", phase.Name())
	}
}

func TestOverridesHooks(t *testing.T) {
	t.Run("base only model overrides nothing", func(t *testing.T) {
		requireOverridesNoHook(t, &hookFreeSample{})
	})

	t.Run("one overridden hook marks its pair and only itself", func(t *testing.T) {
		m := &createHookSample{}
		require.True(t, model.OverridesCreateHooks(m))
		require.False(t, model.OverridesUpdateHooks(m))
		require.False(t, model.OverridesDeleteHooks(m))
		require.True(t, model.OverridesHook(m, apiaction.CreateBefore))
		require.False(t, model.OverridesHook(m, apiaction.CreateAfter))
	})

	t.Run("mixed overrides mark each pair and each hook independently", func(t *testing.T) {
		m := &updateDeleteHookSample{}
		require.False(t, model.OverridesCreateHooks(m))
		require.True(t, model.OverridesUpdateHooks(m))
		require.True(t, model.OverridesDeleteHooks(m))
		require.False(t, model.OverridesHook(m, apiaction.UpdateBefore))
		require.True(t, model.OverridesHook(m, apiaction.UpdateAfter))
		require.True(t, model.OverridesHook(m, apiaction.DeleteBefore))
		require.False(t, model.OverridesHook(m, apiaction.DeleteAfter))
	})

	t.Run("read hooks are tracked one by one", func(t *testing.T) {
		m := &readHookSample{}
		require.False(t, model.OverridesCreateHooks(m))
		require.False(t, model.OverridesUpdateHooks(m))
		require.False(t, model.OverridesDeleteHooks(m))
		require.True(t, model.OverridesHook(m, apiaction.GetBefore))
		require.False(t, model.OverridesHook(m, apiaction.GetAfter))
		require.False(t, model.OverridesHook(m, apiaction.ListBefore))
		require.True(t, model.OverridesHook(m, apiaction.ListAfter))
	})

	t.Run("promotion through a hook free intermediate overrides nothing", func(t *testing.T) {
		requireOverridesNoHook(t, &promotedPlainSample{})
	})

	t.Run("hook promoted from an intermediate type counts as overridden", func(t *testing.T) {
		m := &promotedHookSample{}
		require.True(t, model.OverridesCreateHooks(m))
		require.False(t, model.OverridesUpdateHooks(m))
		require.False(t, model.OverridesDeleteHooks(m))
		require.True(t, model.OverridesHook(m, apiaction.CreateBefore))
		require.False(t, model.OverridesHook(m, apiaction.CreateAfter))
	})

	t.Run("auto base model overrides nothing", func(t *testing.T) {
		requireOverridesNoHook(t, &autoBaseSample{})
	})

	t.Run("nil fails closed as overridden", func(t *testing.T) {
		require.True(t, model.OverridesCreateHooks(nil))
		require.True(t, model.OverridesUpdateHooks(nil))
		require.True(t, model.OverridesDeleteHooks(nil))
		for _, phase := range modelHookPhases(t) {
			require.True(t, model.OverridesHook(nil, phase), "hook %s", phase.Name())
		}
	})

	t.Run("typed nil resolves through its type", func(t *testing.T) {
		requireOverridesNoHook(t, (*hookFreeSample)(nil))
	})

	t.Run("type without hooks overrides nothing", func(t *testing.T) {
		require.False(t, model.OverridesCreateHooks(42))
		require.False(t, model.OverridesUpdateHooks("plain"))
		require.False(t, model.OverridesDeleteHooks(struct{}{}))
		require.False(t, model.OverridesHook(42, apiaction.GetBefore))
	})

	t.Run("a phase outside the hook set fails closed as overridden", func(t *testing.T) {
		require.True(t, model.OverridesHook(&hookFreeSample{}, apiaction.Create))
	})
}
