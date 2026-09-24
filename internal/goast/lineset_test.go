package goast_test

import (
	"go/token"
	"testing"

	"github.com/hydroan/gst/internal/goast"
	"github.com/stretchr/testify/require"
)

// TestLineSetNextAdvancesOneLineAtATime pins that successive positions of a
// LineSet fall on successive lines of its file, starting at line 1, so a
// node given the next position prints on the next line, and that the lines
// keep coming well past the few thousand a large generated file needs: a
// file whose lines ran out would fold every later position onto its last
// line.
func TestLineSetNextAdvancesOneLineAtATime(t *testing.T) {
	fset := token.NewFileSet()
	lines := goast.NewLineSet(fset)

	for want := 1; want <= 20000; want++ {
		require.Equal(t, want, fset.Position(lines.Next()).Line)
	}
}
