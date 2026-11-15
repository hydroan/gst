package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCheckConstLiteral runs the check over a fixture module. Reported are
// the nine strings of internal/gen/reported.go, one a line, each spelling a
// constant of the module's internal/ggconst: handed to go/ast, twice, to
// strings, to bytes through a conversion and to path/filepath, compared,
// concatenated, listed and handed to a function of the module, the one two
// constants spell naming both; a directive that gives no reason; and two
// directives on lines the check reports no string on, one where no string
// spells a constant and one whose string is handed to os/exec. Nothing is
// reported for the strings of accepted.go, which are a constant's value,
// handed to a package outside the module that is no sink, os/exec, fmt and
// flag, or only contain the spelling of a constant; for the import of
// strings, which a constant spells; for the string a directive speaks for;
// for a test file (gen_test.go); for a package that does not import the
// constants (pkg/other); or for the constants package itself.
func TestCheckConstLiteral(t *testing.T) {
	root, pkgs := loadFixture(t, "testdata/constliteral/module")
	violations, err := checkConstLiteral(root, pkgs)
	require.NoError(t, err)
	require.Equal(t, []violation{
		{
			File:    "internal/gen/directive.go",
			Message: "//gst:literal at internal/gen/directive.go:16 gives no reason: say what the string means after the directive",
		},
		{
			File:    "internal/gen/directive.go",
			Message: "//gst:literal at internal/gen/directive.go:17 speaks for no string the check reports: drop the directive",
		},
		{
			File:    "internal/gen/directive.go",
			Message: "//gst:literal at internal/gen/directive.go:18 speaks for no string the check reports: drop the directive",
		},
		{
			File:    "internal/gen/reported.go",
			Message: `String "gst" at internal/gen/reported.go:15 spells ggconst.PkgGst: use the constant, or close the line with //gst:literal <what the string means> when it means something else`,
		},
		{
			File:    "internal/gen/reported.go",
			Message: `String "gst" at internal/gen/reported.go:16 spells ggconst.PkgGst: use the constant, or close the line with //gst:literal <what the string means> when it means something else`,
		},
		{
			File:    "internal/gen/reported.go",
			Message: `String ".go" at internal/gen/reported.go:17 spells ggconst.ExtensionGo: use the constant, or close the line with //gst:literal <what the string means> when it means something else`,
		},
		{
			File:    "internal/gen/reported.go",
			Message: `String "init" at internal/gen/reported.go:18 spells ggconst.FuncInit: use the constant, or close the line with //gst:literal <what the string means> when it means something else`,
		},
		{
			File:    "internal/gen/reported.go",
			Message: `String "model" at internal/gen/reported.go:19 spells ggconst.DirModel or ggconst.PkgModel: use the one it means, or close the line with //gst:literal <what the string means> when it means something else`,
		},
		{
			File:    "internal/gen/reported.go",
			Message: `String "model" at internal/gen/reported.go:20 spells ggconst.DirModel or ggconst.PkgModel: use the one it means, or close the line with //gst:literal <what the string means> when it means something else`,
		},
		{
			File:    "internal/gen/reported.go",
			Message: `String ".go" at internal/gen/reported.go:21 spells ggconst.ExtensionGo: use the constant, or close the line with //gst:literal <what the string means> when it means something else`,
		},
		{
			File:    "internal/gen/reported.go",
			Message: `String "gst" at internal/gen/reported.go:22 spells ggconst.PkgGst: use the constant, or close the line with //gst:literal <what the string means> when it means something else`,
		},
		{
			File:    "internal/gen/reported.go",
			Message: `String "init" at internal/gen/reported.go:23 spells ggconst.FuncInit: use the constant, or close the line with //gst:literal <what the string means> when it means something else`,
		},
	}, violations)
}
