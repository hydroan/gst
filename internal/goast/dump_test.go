package goast_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/goast"
	"github.com/stretchr/testify/require"
)

// TestDump pins the example in the Dump documentation.
func TestDump(t *testing.T) {
	file, dump, err := goast.Dump("sample.go", "package sample\n")
	require.NoError(t, err)
	require.Equal(t, "sample", file.Name.Name)

	want := []string{
		"0  *ast.File {",
		"1  .  Doc: nil",
		"2  .  Package: sample.go:1:1",
		"3  .  Name: *ast.Ident {",
		"4  .  .  NamePos: sample.go:1:9",
		"5  .  .  Name: \"sample\"",
		"6  .  .  Obj: nil",
		"7  .  }",
	}
	lines := strings.Split(dump, "\n")
	require.GreaterOrEqual(t, len(lines), len(want))
	for i, line := range want {
		// The number is right-aligned in six columns.
		require.Equal(t, fmt.Sprintf("%6s", strings.Fields(line)[0])+line[len(strings.Fields(line)[0]):], lines[i])
	}
}

func TestDumpReportsAParseError(t *testing.T) {
	_, _, err := goast.Dump("sample.go", "package\n")
	require.Error(t, err)
}
