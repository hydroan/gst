package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCheckSourceFormat runs the check over a fixture module. Two calls are
// reported: a generator under internal/codegen (render) and one under cmd/gg
// (heal) formatting source text. Nothing is reported for the formatting path
// itself (internal/codegen/gen/helper.go), for a package outside the
// generators (other) or for a test file (render_test.go).
func TestCheckSourceFormat(t *testing.T) {
	root, pkgs := loadFixture(t, "testdata/sourceformat/module")
	violations, err := checkSourceFormat(root, pkgs)
	require.NoError(t, err)
	require.Equal(t, []violation{
		{
			File:    "cmd/gg/heal.go",
			Message: "Call to go/format.Source at cmd/gg/heal.go:8 formats source text: build the generated file as a syntax tree and print it through the one formatting path, internal/codegen/gen/helper.go",
		},
		{
			File:    "internal/codegen/gen/render.go",
			Message: "Call to go/format.Source at internal/codegen/gen/render.go:7 formats source text: build the generated file as a syntax tree and print it through the one formatting path, internal/codegen/gen/helper.go",
		},
	}, violations)
}
