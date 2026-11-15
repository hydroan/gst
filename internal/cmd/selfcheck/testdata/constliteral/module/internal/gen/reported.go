package gen

import (
	"bytes"
	"go/ast"
	"path/filepath"
	"strings"

	"example.com/module/internal/ggconst"
)

// reported holds, one a line, the strings that spell a constant of ggconst.
func reported(root, name string, src []byte) []any {
	return []any{
		ast.NewIdent("gst"),
		&ast.Ident{Name: "gst"},
		strings.HasSuffix(name, ".go"),
		bytes.Contains(src, []byte("init")),
		filepath.Join(root, "model"),
		name == "model",
		name + ".go",
		[]string{"gst"},
		local("init"),
		ggconst.PkgGst,
	}
}

// local stands for a function of the module that takes a name.
func local(name string) string { return name }
