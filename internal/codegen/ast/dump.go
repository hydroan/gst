// Package codegenast provides AST helpers for codegen; the name avoids
// conflicting with the standard library "go/ast" package.
package codegenast

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
)

func Dump(filename string, source any) (f *ast.File, dump string, err error) {
	fset := token.NewFileSet()
	if f, err = parser.ParseFile(fset, filename, source, parser.ParseComments); err != nil {
		return nil, "", err
	}

	var buf bytes.Buffer
	ast.Fprint(&buf, fset, f, func(string, reflect.Value) bool {
		return true
	})

	return f, buf.String(), nil
}

func DumpNode(node ast.Node) (dump string, err error) {
	fset := token.NewFileSet()
	var buf bytes.Buffer
	ast.Fprint(&buf, fset, node, func(string, reflect.Value) bool {
		return true
	})

	return buf.String(), nil
}
