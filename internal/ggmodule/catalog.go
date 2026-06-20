package ggmodule

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// Module describes one built-in framework module that gg can manage.
//
// Name is the directory under the framework's module/ tree. PackageName is the
// actual Go package declared by register.go; those can differ, for example
// module/version uses package versionmod. ImportPath is the framework import
// path a project module/module.go should import when it wants to use the module.
// Addable is deliberately separate from discovery: gg module list should show
// every discoverable framework module, while gg module add can only automate
// modules whose Register function can be called without project-specific args.
type Module struct {
	Name        string
	PackageName string
	ImportPath  string
	Addable     bool
}

// ListModules returns built-in framework modules discovered from module/*/register.go.
//
// The catalog is intentionally derived from source instead of maintained as a
// static list. New framework modules become visible to gg as soon as they expose
// a register.go file, and tests cannot drift from the actual module tree.
func ListModules() ([]Module, error) {
	frameworkRoot, err := findFrameworkRoot()
	if err != nil {
		return nil, err
	}
	return listModulesFromRoot(frameworkRoot)
}

func listModulesFromRoot(frameworkRoot string) ([]Module, error) {
	moduleRoot := filepath.Join(frameworkRoot, "module")
	entries, err := os.ReadDir(moduleRoot)
	if err != nil {
		return nil, err
	}

	modules := make([]Module, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		registerFile := filepath.Join(moduleRoot, entry.Name(), "register.go")
		info, err := inspectModuleRegister(registerFile)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		info.Name = entry.Name()
		info.ImportPath = filepath.ToSlash(filepath.Join(frameworkModulePath, "module", entry.Name()))
		modules = append(modules, info)
	}

	sort.Slice(modules, func(i, j int) bool {
		return modules[i].Name < modules[j].Name
	})
	return modules, nil
}

func inspectModuleRegister(path string) (Module, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return Module{}, err
	}

	info := Module{PackageName: file.Name.Name}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name == nil || fn.Name.Name != "Register" {
			continue
		}
		info.Addable = registerCanBeCalledWithoutArgs(fn)
		return info, nil
	}
	return info, nil
}

// registerCanBeCalledWithoutArgs decides whether gg module add can emit a plain
// pkg.Register() call. A no-arg Register is obviously safe. A single variadic
// parameter is also safe because Go permits calling it with no values; this lets
// optional module configuration remain a code-level choice without blocking
// automatic registration. Required parameters are not guessed by the CLI because
// the right values are project decisions, not framework defaults.
func registerCanBeCalledWithoutArgs(fn *ast.FuncDecl) bool {
	if fn.Type == nil || fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
		return true
	}
	if len(fn.Type.Params.List) != 1 {
		return false
	}
	_, ok := fn.Type.Params.List[0].Type.(*ast.Ellipsis)
	return ok
}

func moduleByName(name string) (Module, error) {
	modules, err := ListModules()
	if err != nil {
		return Module{}, err
	}
	for _, module := range modules {
		if module.Name == name {
			return module, nil
		}
	}
	return Module{}, os.ErrNotExist
}

// importAliasForModule returns the explicit import alias required by Go when the
// module directory name and declared package name differ. Returning nil lets
// astutil add a normal import for the common case.
func importAliasForModule(module Module) *ast.Ident {
	if module.PackageName == "" || module.PackageName == module.Name {
		return nil
	}
	return ast.NewIdent(module.PackageName)
}

// moduleImportAlias maps a project's import spec back to the identifier used in
// code. RemoveModule needs this because users may have kept the default package
// name or an explicit alias that came from an earlier add.
func moduleImportAlias(spec *ast.ImportSpec, module Module) (string, bool) {
	path, err := strconv.Unquote(spec.Path.Value)
	if err != nil || path != module.ImportPath {
		return "", false
	}
	if spec.Name != nil && spec.Name.Name != "." && spec.Name.Name != "_" {
		return spec.Name.Name, true
	}
	return module.PackageName, true
}
