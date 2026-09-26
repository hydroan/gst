package controller_test

import (
	"testing"

	"github.com/hydroan/gst/internal/controller"
	"github.com/stretchr/testify/require"
)

// sampleBinder is an interface with methods: no request body decodes into
// one, nor into a pointer to one.
type sampleBinder interface{ Bind() }

// TestFactoriesRefuseARequestTypeNoBodyDecodesInto pins that mounting a
// factory whose request type is an interface with methods, or a pointer to
// one, panics at once, as the route registers, and names the route: every
// request to the route would fail to bind, so the declaration is refused
// instead. any and a pointer to it stay accepted, holding whatever JSON value
// the body carries, and so does an interface response type, which is only
// ever encoded.
func TestFactoriesRefuseARequestTypeNoBodyDecodesInto(t *testing.T) {
	cfg := configFor[*sampleRecord]("samples")

	require.PanicsWithValue(t, `controller: route "samples": request type controller_test.sampleBinder is an interface with methods or a pointer to one, which no request body decodes into; declare a concrete type, or any`, func() {
		controller.CreateFactory[*sampleRecord, sampleBinder, *sampleRecord](cfg)
	})
	require.PanicsWithValue(t, `controller: route "samples": request type *controller_test.sampleBinder is an interface with methods or a pointer to one, which no request body decodes into; declare a concrete type, or any`, func() {
		controller.CreateFactory[*sampleRecord, *sampleBinder, *sampleRecord](cfg)
	})
	require.NotPanics(t, func() {
		controller.CreateFactory[*sampleRecord, any, *sampleRecord](cfg)
		controller.CreateFactory[*sampleRecord, *any, *sampleRecord](cfg)
		controller.CreateFactory[*sampleRecord, *sampleRecord, sampleBinder](cfg)
	})
}
