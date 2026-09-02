package modelregistry_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/stoewer/go-strcase"
	"github.com/stretchr/testify/require"
)

// hookFreeSample carries no hook overrides: every hook is the promoted
// framework no-op.
type hookFreeSample struct {
	Name string `json:"name,omitempty"`

	modelregistry.Base
}

// createHookSample overrides one hook of the create pair only.
type createHookSample struct {
	modelregistry.Base
}

func (*createHookSample) CreateBefore(context.Context) error { return nil }

// updateDeleteHookSample overrides one hook of the update pair and one hook
// of the delete pair, leaving the create pair promoted.
type updateDeleteHookSample struct {
	modelregistry.Base
}

func (*updateDeleteHookSample) UpdateAfter(context.Context) error  { return nil }
func (*updateDeleteHookSample) DeleteBefore(context.Context) error { return nil }

// readHookSample overrides one hook of the get pair and one hook of the list
// pair, leaving every write hook promoted.
type readHookSample struct {
	modelregistry.Base
}

func (*readHookSample) GetBefore(context.Context) error { return nil }
func (*readHookSample) ListAfter(context.Context) error { return nil }

// hookFreeIntermediate embeds the framework base without overriding anything,
// so models embedding it still carry only no-op hooks.
type hookFreeIntermediate struct {
	modelregistry.Base
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
	modelregistry.Base
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
	modelregistry.AutoBase
}

// modelHookPhases derives the hook phases from the lifecycle hooks the model
// contract declares, mirroring Phase.MethodName in the other direction, so
// the tracked set is checked against the contract itself rather than against
// a second copy of the phase list.
func modelHookPhases(t *testing.T) []consts.Phase {
	t.Helper()
	contract := reflect.TypeFor[types.Model]()
	phases := make([]consts.Phase, 0, contract.NumMethod())
	for method := range contract.Methods() {
		if !strings.HasSuffix(method.Name, "Before") && !strings.HasSuffix(method.Name, "After") {
			continue
		}
		phase := consts.Phase(strcase.SnakeCase(method.Name))
		require.Equal(t, method.Name, phase.MethodName(), "phase derived from %s must round-trip", method.Name)
		phases = append(phases, phase)
	}
	require.NotEmpty(t, phases, "the model contract must declare lifecycle hooks")
	return phases
}

// requireOverridesNoHook asserts that neither the pair-level nor the per-hook
// detection reports an override for m.
func requireOverridesNoHook(t *testing.T, m any) {
	t.Helper()
	require.False(t, modelregistry.OverridesCreateHooks(m))
	require.False(t, modelregistry.OverridesUpdateHooks(m))
	require.False(t, modelregistry.OverridesDeleteHooks(m))
	for _, phase := range modelHookPhases(t) {
		require.False(t, modelregistry.OverridesHook(m, phase), "hook %s", phase.MethodName())
	}
}

func TestOverridesHooks(t *testing.T) {
	t.Run("base only model overrides nothing", func(t *testing.T) {
		requireOverridesNoHook(t, &hookFreeSample{})
	})

	t.Run("one overridden hook marks its pair and only itself", func(t *testing.T) {
		m := &createHookSample{}
		require.True(t, modelregistry.OverridesCreateHooks(m))
		require.False(t, modelregistry.OverridesUpdateHooks(m))
		require.False(t, modelregistry.OverridesDeleteHooks(m))
		require.True(t, modelregistry.OverridesHook(m, consts.PHASE_CREATE_BEFORE))
		require.False(t, modelregistry.OverridesHook(m, consts.PHASE_CREATE_AFTER))
	})

	t.Run("mixed overrides mark each pair and each hook independently", func(t *testing.T) {
		m := &updateDeleteHookSample{}
		require.False(t, modelregistry.OverridesCreateHooks(m))
		require.True(t, modelregistry.OverridesUpdateHooks(m))
		require.True(t, modelregistry.OverridesDeleteHooks(m))
		require.False(t, modelregistry.OverridesHook(m, consts.PHASE_UPDATE_BEFORE))
		require.True(t, modelregistry.OverridesHook(m, consts.PHASE_UPDATE_AFTER))
		require.True(t, modelregistry.OverridesHook(m, consts.PHASE_DELETE_BEFORE))
		require.False(t, modelregistry.OverridesHook(m, consts.PHASE_DELETE_AFTER))
	})

	t.Run("read hooks are tracked one by one", func(t *testing.T) {
		m := &readHookSample{}
		require.False(t, modelregistry.OverridesCreateHooks(m))
		require.False(t, modelregistry.OverridesUpdateHooks(m))
		require.False(t, modelregistry.OverridesDeleteHooks(m))
		require.True(t, modelregistry.OverridesHook(m, consts.PHASE_GET_BEFORE))
		require.False(t, modelregistry.OverridesHook(m, consts.PHASE_GET_AFTER))
		require.False(t, modelregistry.OverridesHook(m, consts.PHASE_LIST_BEFORE))
		require.True(t, modelregistry.OverridesHook(m, consts.PHASE_LIST_AFTER))
	})

	t.Run("promotion through a hook free intermediate overrides nothing", func(t *testing.T) {
		requireOverridesNoHook(t, &promotedPlainSample{})
	})

	t.Run("hook promoted from an intermediate type counts as overridden", func(t *testing.T) {
		m := &promotedHookSample{}
		require.True(t, modelregistry.OverridesCreateHooks(m))
		require.False(t, modelregistry.OverridesUpdateHooks(m))
		require.False(t, modelregistry.OverridesDeleteHooks(m))
		require.True(t, modelregistry.OverridesHook(m, consts.PHASE_CREATE_BEFORE))
		require.False(t, modelregistry.OverridesHook(m, consts.PHASE_CREATE_AFTER))
	})

	t.Run("auto base model overrides nothing", func(t *testing.T) {
		requireOverridesNoHook(t, &autoBaseSample{})
	})

	t.Run("nil fails closed as overridden", func(t *testing.T) {
		require.True(t, modelregistry.OverridesCreateHooks(nil))
		require.True(t, modelregistry.OverridesUpdateHooks(nil))
		require.True(t, modelregistry.OverridesDeleteHooks(nil))
		for _, phase := range modelHookPhases(t) {
			require.True(t, modelregistry.OverridesHook(nil, phase), "hook %s", phase.MethodName())
		}
	})

	t.Run("typed nil resolves through its type", func(t *testing.T) {
		requireOverridesNoHook(t, (*hookFreeSample)(nil))
	})

	t.Run("type without hooks overrides nothing", func(t *testing.T) {
		require.False(t, modelregistry.OverridesCreateHooks(42))
		require.False(t, modelregistry.OverridesUpdateHooks("plain"))
		require.False(t, modelregistry.OverridesDeleteHooks(struct{}{}))
		require.False(t, modelregistry.OverridesHook(42, consts.PHASE_GET_BEFORE))
	})

	t.Run("a phase outside the hook set fails closed as overridden", func(t *testing.T) {
		require.True(t, modelregistry.OverridesHook(&hookFreeSample{}, consts.PHASE_CREATE))
	})
}
