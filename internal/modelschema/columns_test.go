package modelschema_test

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/internal/modelschema"
	"github.com/stretchr/testify/require"
)

// TestBindingRules pins the examples of BindingRules: the rules of a field's
// own are the ones its binding tag declares before a dive, in order, and a
// tag declaring none has none.
func TestBindingRules(t *testing.T) {
	require.Equal(t, []string{"required", "max=8"}, modelschema.BindingRules(`binding:"required,max=8"`))
	require.Equal(t, []string{"omitempty"}, modelschema.BindingRules(`binding:"omitempty,dive,required"`))
	require.Empty(t, modelschema.BindingRules(`binding:"dive,required"`))
	require.Nil(t, modelschema.BindingRules(`json:"name"`))
	require.Nil(t, modelschema.BindingRules(``))
}

// TestIsReadOnly pins which struct tags declare a field read-only to
// clients: the ones whose own binding rules hold readonly, the rules after a
// dive being the elements' of a slice or a map, and no other tag, whatever
// else names the word.
func TestIsReadOnly(t *testing.T) {
	for _, tt := range []struct {
		tag  reflect.StructTag
		want bool
	}{
		{tag: `binding:"readonly"`, want: true},
		{tag: `json:"stage" binding:"readonly" pb:"13"`, want: true},
		{tag: `binding:"omitempty,readonly"`, want: true},
		{tag: `binding:"readonly,dive,required"`, want: true},
		{tag: `binding:"dive,readonly"`, want: false},
		{tag: `binding:"required"`, want: false},
		{tag: `binding:"readonlyish"`, want: false},
		{tag: `binding:"READONLY"`, want: false},
		{tag: `json:"readonly"`, want: false},
		{tag: `readonly:"true"`, want: false},
		{tag: ``, want: false},
	} {
		require.Equal(t, tt.want, modelschema.IsReadOnly(tt.tag), "%s", tt.tag)
	}
}
