package modelschema_test

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/internal/modelschema"
	"github.com/stretchr/testify/require"
)

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
