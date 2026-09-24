package main

import (
	"fmt"
	"go/ast"
	"go/types"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
)

// sourceFormatters are the functions that turn source text into formatted Go
// source. A generator calling one of them built its output as text.
var sourceFormatters = map[[2]string]bool{
	{"go/format", "Source"}:                   true,
	{"mvdan.cc/gofumpt/format", "Source"}:     true,
	{"golang.org/x/tools/imports", "Process"}: true,
}

// sourceFormatScope lists the directories under the root whose packages
// generate Go code, and sourceFormatHome names the one file among them that
// may format source text: the printing path every generator's syntax tree
// goes through.
var (
	sourceFormatScope = []string{filepath.Join("internal", "codegen"), filepath.Join("cmd", "gg")}
	sourceFormatHome  = filepath.Join("internal", "codegen", "gen", "helper.go")
)

// checkSourceFormat reports the calls that format source text in the code
// generators: every call to one of sourceFormatters from a file under
// sourceFormatScope other than sourceFormatHome. Generated Go code is built
// as a syntax tree and printed, so a generator has no formatted text to
// produce but through that one printing path; a call anywhere else means the
// generator assembled its output as a string. Test files are left alone.
func checkSourceFormat(root string, pkgs []*packages.Package) ([]violation, error) {
	var found []violation
	for _, p := range pkgs {
		if p.ForTest != "" {
			continue
		}
		for _, file := range p.Syntax {
			path := p.Fset.Position(file.Pos()).Filename
			rel, err := filepath.Rel(root, path)
			if err != nil || strings.HasSuffix(rel, "_test.go") || rel == sourceFormatHome || !inSourceFormatScope(rel) {
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				fn, ok := p.TypesInfo.Uses[sel.Sel].(*types.Func)
				if !ok || fn.Pkg() == nil || !sourceFormatters[[2]string{fn.Pkg().Path(), fn.Name()}] {
					return true
				}
				pos := p.Fset.Position(call.Pos())
				found = append(found, violation{
					File: rel,
					Message: fmt.Sprintf("Call to %s.%s at %s:%d formats source text: build the generated file as a syntax tree and print it through the one formatting path, %s",
						fn.Pkg().Path(), fn.Name(), rel, pos.Line, sourceFormatHome),
				})
				return true
			})
		}
	}
	slices.SortFunc(found, func(a, b violation) int {
		return strings.Compare(a.Message, b.Message)
	})
	return found, nil
}

// inSourceFormatScope reports whether rel, a path relative to the root, lies
// under one of the sourceFormatScope directories.
func inSourceFormatScope(rel string) bool {
	for _, dir := range sourceFormatScope {
		if rel == dir || strings.HasPrefix(rel, dir+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
