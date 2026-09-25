package ts

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnumBodyIsAUnionUnlessTheConstantsCombine(t *testing.T) {
	require.Equal(t, `"a" | "b"`, enumBody(&enumType{values: []enumValue{{literal: `"a"`}, {literal: `"b"`}}}))
	require.Equal(t, "number", enumBody(&enumType{bitwise: true, values: []enumValue{{literal: "1"}}}))
	require.Equal(t, "never", enumBody(&enumType{}))
}

func TestEnumDocListsTheConstantsUnderTheTypeComment(t *testing.T) {
	// The examples of the enumDoc doc comment.
	values := &enumType{values: []enumValue{
		{literal: `"active"`, doc: "StatusActive marks a sample\nin use."},
		{literal: `"archived"`},
	}}
	require.Equal(t,
		"Status is the state of a sample.\n\n- \"active\": StatusActive marks a sample in use.\n- \"archived\"",
		enumDoc("Status is the state of a sample.", values))

	flags := &enumType{bitwise: true, values: []enumValue{{literal: "1"}, {literal: "2"}}}
	require.Equal(t, "Any bitwise combination of:\n- 1\n- 2", enumDoc("", flags))
}
