// Package goast holds the syntax tree helpers the framework's tooling shares:
// the dsl parser, the code generator and the gg commands read Go source
// through them, and none of them generates code. The name keeps clear of the
// standard library "go/ast" package the helpers work on.
package goast

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
)

// Dump parses the Go source file filename, reading source instead when it is
// not nil (see parser.ParseFile), and returns the file with its syntax tree
// as ast.Fprint prints it, positions spelled file:line:column. For
// sample.go holding "package sample\n" the dump starts with the lines below,
// each numbered, the number right-aligned in six columns:
//
//	0  *ast.File {
//	1  .  Doc: nil
//	2  .  Package: sample.go:1:1
//	3  .  Name: *ast.Ident {
//	4  .  .  NamePos: sample.go:1:9
//	5  .  .  Name: "sample"
//	6  .  .  Obj: nil
//	7  .  }
//
// Every field is printed, nil ones included. It is what gg ast dump prints.
func Dump(filename string, source any) (f *ast.File, dump string, err error) {
	fset := token.NewFileSet()
	if f, err = parser.ParseFile(fset, filename, source, parser.ParseComments); err != nil {
		return nil, "", err
	}

	var buf bytes.Buffer
	if err = ast.Fprint(&buf, fset, f, nil); err != nil {
		return nil, "", err
	}
	return f, buf.String(), nil
}
