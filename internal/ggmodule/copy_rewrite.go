package ggmodule

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"

	"github.com/hydroan/gst/internal/codegen/gen"
)

// normalizeModuleModelSource converts framework model files into the current
// project package layout. The model directory name is the package name, so
// internal/model/mfa package modelmfa becomes model/mfa package mfa.
func normalizeModuleModelSource(filename string, src []byte, targetPackage string) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	file.Name.Name = targetPackage

	code, err := gen.FormatNodeExtraWithFileSet(file, fset, true)
	if err != nil {
		return nil, err
	}
	return []byte(code), nil
}

// normalizeModuleServiceSource rewrites helper files into the current service
// package and maps framework internal model imports to the current project's
// model package.
func normalizeModuleServiceSource(filename string, src []byte, moduleName string, targetModelImportPath string) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	sourceModelNames := rewriteModuleServiceFile(file, moduleName, targetModelImportPath)
	rewriteSelectorPackages(file, sourceModelNames, moduleName)

	code, err := gen.FormatNodeExtraWithFileSet(file, fset, true)
	if err != nil {
		return nil, err
	}
	return []byte(code), nil
}

func rewriteModuleServiceFile(file *ast.File, moduleName string, targetModelImportPath string) map[string]bool {
	file.Name.Name = moduleName

	sourceModelNames := make(map[string]bool)
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if path != frameworkModulePath+"/internal/model/"+moduleName {
			continue
		}
		if imp.Name != nil && imp.Name.Name != "." && imp.Name.Name != "_" {
			sourceModelNames[imp.Name.Name] = true
		} else {
			sourceModelNames[moduleName] = true
		}
		imp.Path.Value = strconv.Quote(targetModelImportPath)
		imp.Name = nil
	}
	return sourceModelNames
}

func rewriteSelectorPackages(node ast.Node, oldNames map[string]bool, newName string) {
	if len(oldNames) == 0 {
		return
	}
	ast.Inspect(node, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || !oldNames[ident.Name] {
			return true
		}
		ident.Name = newName
		return true
	})
}
