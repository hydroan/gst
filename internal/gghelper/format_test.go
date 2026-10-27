package gghelper_test

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/hydroan/gst/internal/gghelper"
)

// TestFormatFileGroupsImportsTheWayGoplsSaves pins how FormatFile groups the
// imports of a file: as a gopls running gofumpt saves them, so saving a file
// gg wrote leaves its import groups as they are. In the module demo the
// project's packages make a group of their own, after the standard library's
// and before the others; a module path with a dot in it is grouped with the
// others, as goimports groups it.
func TestFormatFileGroupsImportsTheWayGoplsSaves(t *testing.T) {
	tests := []struct {
		name       string
		modulePath string
		src        string
		want       string
	}{
		{
			name:       "the project's package and the framework's",
			modulePath: "demo",
			src: `package router

import (
	"github.com/hydroan/gst"
	"demo/model/tool"
)
`,
			want: `package router

import (
	"demo/model/tool"

	"github.com/hydroan/gst"
)
`,
		},
		{
			name:       "with a package of the standard library",
			modulePath: "demo",
			src: `package router

import (
	"demo/model/tool"
	"fmt"
	"github.com/hydroan/gst"
)
`,
			want: `package router

import (
	"fmt"

	"demo/model/tool"

	"github.com/hydroan/gst"
)
`,
		},
		{
			name:       "a module path with a dot",
			modulePath: "example.com/demo",
			src: `package router

import (
	"github.com/hydroan/gst"
	"example.com/demo/model/tool"
)
`,
			want: `package router

import (
	"example.com/demo/model/tool"
	"github.com/hydroan/gst"
)
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "router.gen.go", tt.src, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			got, err := gghelper.FormatFile(file, fset, tt.modulePath)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("FormatFile() =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

// TestFormatFileLeavesTheExtraRulesOff pins the example of FormatFile: the
// imports of a single group come out in three, as a gopls running gofumpt
// saves them, and the extra rules of gofumpt stay off, as gopls runs it, so
// two parameters of one type keep both types and a bare return stays bare.
func TestFormatFileLeavesTheExtraRulesOff(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "sample.go", `package sample

import (
	"fmt"
	"demo/model/tool"
	"github.com/hydroan/gst"
)

func join(a string, b string) (s string) {
	s = a + b
	return
}
`, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	got, err := gghelper.FormatFile(file, fset, "demo")
	if err != nil {
		t.Fatal(err)
	}
	want := `package sample

import (
	"fmt"

	"demo/model/tool"

	"github.com/hydroan/gst"
)

func join(a string, b string) (s string) {
	s = a + b
	return
}
`
	if got != want {
		t.Fatalf("FormatFile() =\n%s\nwant\n%s", got, want)
	}
}
