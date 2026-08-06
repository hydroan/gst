package ggmodule

import (
	"bytes"
	"go/ast"
	goformat "go/format"
	"go/parser"
	"go/token"
	"os"

	gofumpt "mvdan.cc/gofumpt/format"
)

func parseGoFile(path string) (*token.FileSet, *ast.File, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	return fset, file, err
}

func formatGoFile(fset *token.FileSet, file *ast.File) ([]byte, error) {
	var buf bytes.Buffer
	if err := goformat.Node(&buf, fset, file); err != nil {
		return nil, err
	}
	return gofumpt.Source(buf.Bytes(), gofumpt.Options{})
}

func writeGoFile(path string, fset *token.FileSet, file *ast.File) error {
	formatted, err := formatGoFile(fset, file)
	if err != nil {
		return err
	}
	if err := ensureParentDir(path); err != nil {
		return err
	}
	return os.WriteFile(path, formatted, 0o600)
}

func identName(ident *ast.Ident) string {
	if ident == nil {
		return ""
	}
	return ident.Name
}
