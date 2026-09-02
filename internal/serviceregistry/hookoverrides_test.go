package serviceregistry_test

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

type hookSampleModel struct {
	modelregistry.Base
}

type hookSampleBase = serviceregistry.Base[*hookSampleModel, *hookSampleModel, *hookSampleModel]

// hookFreeService embeds the base and overrides nothing.
type hookFreeService struct {
	hookSampleBase
}

// listBeforeService overrides one hook with a pointer receiver.
type listBeforeService struct {
	hookSampleBase
}

func (*listBeforeService) ListBefore(*types.ServiceContext, *[]*hookSampleModel) error { return nil }

// listAfterValueService overrides one hook with a value receiver, the shape
// the base itself uses.
type listAfterValueService struct {
	hookSampleBase
}

func (listAfterValueService) ListAfter(*types.ServiceContext, *[]*hookSampleModel) error { return nil }

// hookedIntermediate overrides a hook on a type services then embed.
type hookedIntermediate struct {
	hookSampleBase
}

func (*hookedIntermediate) CreateBefore(*types.ServiceContext, *hookSampleModel) error { return nil }

// promotedHookService promotes the override of hookedIntermediate.
type promotedHookService struct {
	hookedIntermediate
}

// baseMethodNames lists every method the service base declares, read off the
// type itself so the assertions cover the whole hook set without a copy of it.
// The methods promoted from the logger Base embeds are not hooks and are left
// out.
func baseMethodNames() []string {
	typ := reflect.TypeFor[*hookSampleBase]()
	logger := reflect.TypeFor[types.Logger]()
	names := make([]string, 0, typ.NumMethod())
	for method := range typ.Methods() {
		if _, promoted := logger.MethodByName(method.Name); promoted {
			continue
		}
		names = append(names, method.Name)
	}
	return names
}

func TestOverridesHook(t *testing.T) {
	t.Run("base only service overrides nothing", func(t *testing.T) {
		svc := &hookFreeService{}
		for _, name := range baseMethodNames() {
			require.False(t, serviceregistry.OverridesHook(svc, name), "method %s", name)
		}
	})

	t.Run("the default service overrides nothing", func(t *testing.T) {
		svc := serviceregistry.Resolve[*hookSampleModel, *hookSampleModel, *hookSampleModel]("hookoverrides/unregistered")
		for _, name := range baseMethodNames() {
			require.False(t, serviceregistry.OverridesHook(svc, name), "method %s", name)
		}
	})

	t.Run("pointer receiver override marks only its hook", func(t *testing.T) {
		svc := &listBeforeService{}
		require.True(t, serviceregistry.OverridesHook(svc, "ListBefore"))
		require.False(t, serviceregistry.OverridesHook(svc, "ListAfter"))
		require.False(t, serviceregistry.OverridesHook(svc, "CreateBefore"))
	})

	t.Run("value receiver override marks only its hook", func(t *testing.T) {
		svc := &listAfterValueService{}
		require.True(t, serviceregistry.OverridesHook(svc, "ListAfter"))
		require.False(t, serviceregistry.OverridesHook(svc, "ListBefore"))
	})

	t.Run("hook promoted from an intermediate type counts as overridden", func(t *testing.T) {
		svc := &promotedHookService{}
		require.True(t, serviceregistry.OverridesHook(svc, "CreateBefore"))
		require.False(t, serviceregistry.OverridesHook(svc, "CreateAfter"))
	})

	t.Run("nil fails closed as overridden", func(t *testing.T) {
		require.True(t, serviceregistry.OverridesHook(nil, "ListBefore"))
	})

	t.Run("a method outside the base set fails closed as overridden", func(t *testing.T) {
		require.True(t, serviceregistry.OverridesHook(&hookFreeService{}, "Bogus"))
	})
}
