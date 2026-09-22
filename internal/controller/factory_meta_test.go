package controller_test

import (
	"testing"

	"github.com/hydroan/gst/internal/controller"
	"github.com/stretchr/testify/require"
)

// sampleBinder is an interface with methods: no request body decodes into
// one.
type sampleBinder interface{ Bind() }

// TestFactoriesRefuseARequestInterfaceWithMethods pins that mounting a factory
// whose request type is an interface with methods panics at once, as the
// route registers: every request to the route would fail to bind, so the
// declaration is refused instead. any stays accepted, holding whatever JSON
// value the body carries.
func TestFactoriesRefuseARequestInterfaceWithMethods(t *testing.T) {
	require.PanicsWithValue(t, "controller: request type controller_test.sampleBinder is an interface with methods, which no request body decodes into; declare a concrete type, or any", func() {
		controller.CreateFactory[*sampleRecord, sampleBinder, *sampleRecord]()
	})
	require.NotPanics(t, func() {
		controller.CreateFactory[*sampleRecord, any, *sampleRecord]()
	})
}
