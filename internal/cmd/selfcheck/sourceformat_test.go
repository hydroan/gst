package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCheckSourceFormat runs the check over a fixture module. Three things
// are reported: a generator under internal/gggen (render) and one under
// cmd/gg (heal) formatting source text, and a generator importing
// text/template (tmpl). Nothing is reported for the formatting path itself
// (internal/gggen/helper.go), for a package outside the generators
// (other) or for a test file (render_test.go).
func TestCheckSourceFormat(t *testing.T) {
	root, pkgs := loadFixture(t, "testdata/sourceformat/module")
	violations, err := checkSourceFormat(root, pkgs)
	require.NoError(t, err)
	require.Equal(t, []violation{
		{
			File:    "cmd/gg/heal.go",
			Message: "Call to go/format.Source at cmd/gg/heal.go:8 formats source text: build the generated file as a syntax tree and print it through the one formatting path, internal/gggen/helper.go",
		},
		{
			File:    "internal/gggen/render.go",
			Message: "Call to go/format.Source at internal/gggen/render.go:7 formats source text: build the generated file as a syntax tree and print it through the one formatting path, internal/gggen/helper.go",
		},
		{
			File:    "internal/gggen/tmpl.go",
			Message: "Import of text/template at internal/gggen/tmpl.go:3 renders generated code from a template: build the generated file as a syntax tree and print it through the one formatting path, internal/gggen/helper.go",
		},
	}, violations)
}
