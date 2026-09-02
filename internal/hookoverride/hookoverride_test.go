package hookoverride_test

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/internal/hookoverride"
	"github.com/stretchr/testify/require"
)

// valueBase stands in for a framework base declaring its hook with a value
// receiver, as the service base does.
type valueBase struct{}

func (valueBase) ValueHook() error { return nil }

// pointerBase stands in for a framework base declaring its hook with a
// pointer receiver, as the model bases do.
type pointerBase struct{}

func (*pointerBase) PointerHook() error { return nil }

func isFrameworkBase(typ reflect.Type) bool {
	return typ == reflect.TypeFor[valueBase]() || typ == reflect.TypeFor[pointerBase]()
}

// plainSample embeds both bases and overrides nothing.
type plainSample struct {
	valueBase
	pointerBase
}

// pointerOverrideSample declares the pointer-receiver hook itself and takes
// the other from its base.
type pointerOverrideSample struct {
	valueBase
}

func (*pointerOverrideSample) PointerHook() error { return nil }

// valueOverrideSample declares the value-receiver hook itself, so a pointer
// to it reaches the declaration through a compiler wrapper, and takes the
// other hook from its base.
type valueOverrideSample struct {
	pointerBase
}

func (valueOverrideSample) ValueHook() error { return nil }

// pointerEmbedSample embeds the bases through pointers.
type pointerEmbedSample struct {
	*valueBase
	*pointerBase
}

// hookedIntermediate declares a hook on a type that samples embed in turn.
type hookedIntermediate struct {
	valueBase
}

func (*hookedIntermediate) PointerHook() error { return nil }

// promotedOverrideSample promotes the override of hookedIntermediate.
type promotedOverrideSample struct {
	hookedIntermediate
}

// plainIntermediate embeds the bases without overriding anything.
type plainIntermediate struct {
	valueBase
	pointerBase
}

// promotedPlainSample promotes only framework no-ops through plainIntermediate.
type promotedPlainSample struct {
	plainIntermediate
}

func TestOverridden(t *testing.T) {
	t.Run("base only type overrides nothing", func(t *testing.T) {
		typ := reflect.TypeFor[*plainSample]()
		require.False(t, hookoverride.Overridden(typ, "ValueHook", isFrameworkBase))
		require.False(t, hookoverride.Overridden(typ, "PointerHook", isFrameworkBase))
	})

	t.Run("the framework bases themselves override nothing", func(t *testing.T) {
		require.False(t, hookoverride.Overridden(reflect.TypeFor[*valueBase](), "ValueHook", isFrameworkBase))
		require.False(t, hookoverride.Overridden(reflect.TypeFor[valueBase](), "ValueHook", isFrameworkBase))
		require.False(t, hookoverride.Overridden(reflect.TypeFor[*pointerBase](), "PointerHook", isFrameworkBase))
	})

	t.Run("pointer receiver override counts for that hook only", func(t *testing.T) {
		typ := reflect.TypeFor[*pointerOverrideSample]()
		require.True(t, hookoverride.Overridden(typ, "PointerHook", isFrameworkBase))
		require.False(t, hookoverride.Overridden(typ, "ValueHook", isFrameworkBase))
	})

	t.Run("value receiver override is seen through the pointer wrapper", func(t *testing.T) {
		require.True(t, hookoverride.Overridden(reflect.TypeFor[*valueOverrideSample](), "ValueHook", isFrameworkBase))
		require.True(t, hookoverride.Overridden(reflect.TypeFor[valueOverrideSample](), "ValueHook", isFrameworkBase))
		require.False(t, hookoverride.Overridden(reflect.TypeFor[*valueOverrideSample](), "PointerHook", isFrameworkBase))
	})

	t.Run("bases embedded through pointers override nothing", func(t *testing.T) {
		typ := reflect.TypeFor[*pointerEmbedSample]()
		require.False(t, hookoverride.Overridden(typ, "ValueHook", isFrameworkBase))
		require.False(t, hookoverride.Overridden(typ, "PointerHook", isFrameworkBase))
	})

	t.Run("override promoted from an intermediate type counts", func(t *testing.T) {
		typ := reflect.TypeFor[*promotedOverrideSample]()
		require.True(t, hookoverride.Overridden(typ, "PointerHook", isFrameworkBase))
		require.False(t, hookoverride.Overridden(typ, "ValueHook", isFrameworkBase))
	})

	t.Run("promotion through a plain intermediate overrides nothing", func(t *testing.T) {
		typ := reflect.TypeFor[*promotedPlainSample]()
		require.False(t, hookoverride.Overridden(typ, "ValueHook", isFrameworkBase))
		require.False(t, hookoverride.Overridden(typ, "PointerHook", isFrameworkBase))
	})

	t.Run("a method the type lacks overrides nothing", func(t *testing.T) {
		require.False(t, hookoverride.Overridden(reflect.TypeFor[*plainSample](), "Missing", isFrameworkBase))
		require.False(t, hookoverride.Overridden(reflect.TypeFor[int](), "ValueHook", isFrameworkBase))
	})
}
