package modelregistry_test

import (
	"context"
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
)

// writeHookFreeSample carries no write-hook overrides: every hook is the
// promoted framework no-op.
type writeHookFreeSample struct {
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

func TestOverridesWriteHooks(t *testing.T) {
	t.Run("base only model overrides nothing", func(t *testing.T) {
		m := &writeHookFreeSample{}
		require.False(t, modelregistry.OverridesCreateHooks(m))
		require.False(t, modelregistry.OverridesUpdateHooks(m))
		require.False(t, modelregistry.OverridesDeleteHooks(m))
	})

	t.Run("one overridden hook marks only its pair", func(t *testing.T) {
		m := &createHookSample{}
		require.True(t, modelregistry.OverridesCreateHooks(m))
		require.False(t, modelregistry.OverridesUpdateHooks(m))
		require.False(t, modelregistry.OverridesDeleteHooks(m))
	})

	t.Run("mixed overrides mark each pair independently", func(t *testing.T) {
		m := &updateDeleteHookSample{}
		require.False(t, modelregistry.OverridesCreateHooks(m))
		require.True(t, modelregistry.OverridesUpdateHooks(m))
		require.True(t, modelregistry.OverridesDeleteHooks(m))
	})

	t.Run("promotion through a hook free intermediate overrides nothing", func(t *testing.T) {
		m := &promotedPlainSample{}
		require.False(t, modelregistry.OverridesCreateHooks(m))
		require.False(t, modelregistry.OverridesUpdateHooks(m))
		require.False(t, modelregistry.OverridesDeleteHooks(m))
	})

	t.Run("hook promoted from an intermediate type counts as overridden", func(t *testing.T) {
		m := &promotedHookSample{}
		require.True(t, modelregistry.OverridesCreateHooks(m))
		require.False(t, modelregistry.OverridesUpdateHooks(m))
		require.False(t, modelregistry.OverridesDeleteHooks(m))
	})

	t.Run("auto base model overrides nothing", func(t *testing.T) {
		m := &autoBaseSample{}
		require.False(t, modelregistry.OverridesCreateHooks(m))
		require.False(t, modelregistry.OverridesUpdateHooks(m))
		require.False(t, modelregistry.OverridesDeleteHooks(m))
	})

	t.Run("nil fails closed as overridden", func(t *testing.T) {
		require.True(t, modelregistry.OverridesCreateHooks(nil))
		require.True(t, modelregistry.OverridesUpdateHooks(nil))
		require.True(t, modelregistry.OverridesDeleteHooks(nil))
	})

	t.Run("typed nil resolves through its type", func(t *testing.T) {
		m := (*writeHookFreeSample)(nil)
		require.False(t, modelregistry.OverridesCreateHooks(m))
		require.False(t, modelregistry.OverridesUpdateHooks(m))
		require.False(t, modelregistry.OverridesDeleteHooks(m))
	})

	t.Run("type without hooks overrides nothing", func(t *testing.T) {
		require.False(t, modelregistry.OverridesCreateHooks(42))
		require.False(t, modelregistry.OverridesUpdateHooks("plain"))
		require.False(t, modelregistry.OverridesDeleteHooks(struct{}{}))
	})
}
