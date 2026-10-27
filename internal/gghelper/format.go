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

// FormatNode prints node as Go source in the go/format style.
//
//gst:api the counterpart of FormatNodeExtra in the plain go/format style
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

// FormatNodeExtra prints node like FormatNode, but in the stricter gofumpt
// style (https://github.com/mvdan/gofumpt) with its GroupParams,
// ClotheReturns and BalanceCalls rules, as a file of the module modulePath
// (see FormatNodeExtraWithFileSet). The generated registration and service
// files are printed through it.
func FormatNodeExtra(node ast.Node, modulePath string) (string, error) {
	return FormatNodeExtraWithFileSet(node, nil, modulePath)
}

// FormatNodeExtraWithFileSet is FormatNodeExtra printing node through fset,
// the FileSet the positions of node refer to, which lays it out by them: the
// comments of a parsed file stay in place, and a file built from scratch
// breaks lines where its fabricated positions say (see goast.LineSet). A nil
// fset stands for a new one. The imports are grouped the way gopls organizes
// them on save and gofumpt then formats the file for the module modulePath,
// the order a gopls running gofumpt saves a file in, so saving a file gg
// wrote leaves its import groups and its layout as they are: for the module
// demo, demo/model/tool and github.com/hydroan/gst are two groups, and with
// fmt imported too, fmt leads in a group of its own.
func FormatNodeExtraWithFileSet(node ast.Node, fset *token.FileSet, modulePath string) (string, error) {
	var buf bytes.Buffer
	// create a new FileSet if none was provided
	if fset == nil {
		fset = token.NewFileSet()
	}

	if err := format.Node(&buf, fset, node); err != nil {
		return "", err
	}
	grouped, err := imports.Process("", buf.Bytes(), formatOnlyImports)
	if err != nil {
		return "", err
	}

	formatted, err := gofumpt.Source(grouped, gofumpt.Options{
		LangVersion: "",
		ModulePath:  modulePath,
		Extra:       gofumpt.Extra{GroupParams: true, ClotheReturns: true, BalanceCalls: true},
	})
	if err != nil {
		return "", err
	}
	return string(formatted), nil
}
