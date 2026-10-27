package gghelper

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/token"

	"golang.org/x/tools/imports"
	gofumpt "mvdan.cc/gofumpt/format"
)

// formatOnlyImports runs goimports as an import formatter only: it sorts and
// groups the imports of a file the way gopls organizes them on save. Its
// import fixing, adding the imports a file misses and dropping the unused
// ones, is left off, since every caller here builds the imports of its file
// itself.
var formatOnlyImports = &imports.Options{Comments: true, TabIndent: true, TabWidth: 8, FormatOnly: true}

// FormatNode prints node, a declaration or another fragment of a file, as Go
// source in the go/format style; gofumpt and the grouping of imports work on
// whole files only (see FormatFile).
//
//gst:api the counterpart of FormatFile for a fragment of a file
func FormatNode(node ast.Node) (string, error) {
	var buf bytes.Buffer
	fset := token.NewFileSet()

	if err := format.Node(&buf, fset, node); err != nil {
		return "", err
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return "", err
	}
	return string(formatted), nil
}

// FormatFile prints file as Go source, the way a gopls running gofumpt saves a
// file of the module modulePath: its imports grouped as gopls organizes them on
// save, then gofumpt, with none of its extra rules, as gopls runs it. The Go
// files gg prints itself go through it, the ones it generates and the project's
// own files it rewrites, so saving one leaves its import groups and its layout
// as they are; the files the protobuf plugins compile are written as the
// plugins print them. For the module demo, the imports fmt, demo/model/tool and
// github.com/hydroan/gst of a single group come out in three, in that order,
// and func join(a string, b string) keeps both types. file is printed through
// fset, the FileSet its positions refer to, which lays it out by them: the
// comments of a parsed file stay in place, and a file built from scratch breaks
// lines where its fabricated positions say (see goast.LineSet). A nil fset
// stands for a new one.
func FormatFile(file *ast.File, fset *token.FileSet, modulePath string) (string, error) {
	var buf bytes.Buffer
	if fset == nil {
		fset = token.NewFileSet()
	}
	if err := format.Node(&buf, fset, file); err != nil {
		return "", err
	}
	grouped, err := imports.Process("", buf.Bytes(), formatOnlyImports)
	if err != nil {
		return "", err
	}
	formatted, err := gofumpt.Source(grouped, gofumpt.Options{LangVersion: "", ModulePath: modulePath})
	if err != nil {
		return "", err
	}
	return string(formatted), nil
}
