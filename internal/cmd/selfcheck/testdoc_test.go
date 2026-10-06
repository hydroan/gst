package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCheckTestDoc runs the check over a fixture module whose test files
// hold one case each of what the check reports and leaves alone: a test
// whose doc opens with another test's name, a variable whose doc opens with
// an article and then no name, a grouped specification with such a doc, and
// a helper of an internal test file; left alone are a doc opening with its
// own name, the comment on a parenthesized group, init, an Example, a
// deprecated declaration and a blank identifier, and the non-test file whose
// doc opens with another name, which the lint configuration holds instead.
func TestCheckTestDoc(t *testing.T) {
	root, pkgs := loadFixture(t, "testdata/testdoc/module")
	violations, err := checkTestDoc(root, pkgs)
	require.NoError(t, err)
	require.Equal(t, []violation{
		{
			File:    "sample/sample_internal_test.go",
			Message: "Test file 'sample/sample_internal_test.go:3': the doc comment of helper opens with \"helperValue is what the helper returns; the comment opens with another name.\"; a doc comment opens with the name of what it documents",
		},
		{
			File:    "sample/sample_test.go",
			Message: "Test file 'sample/sample_test.go:5': the doc comment of TestWrongName opens with \"TestOther pins what another test pins: the comment of a neighbor, left on\"; a doc comment opens with the name of what it documents",
		},
		{
			File:    "sample/sample_test.go",
			Message: "Test file 'sample/sample_test.go:12': the doc comment of sharedFixture opens with \"A fixture the tests share, whose comment names no declaration.\"; a doc comment opens with the name of what it documents",
		},
		{
			File:    "sample/sample_test.go",
			Message: "Test file 'sample/sample_test.go:21': the doc comment of third opens with \"Third fixture, the comment of a spec that does not open with its name.\"; a doc comment opens with the name of what it documents",
		},
	}, violations)
}
