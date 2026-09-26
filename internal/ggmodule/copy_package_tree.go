package ggmodule

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strconv"

	"golang.org/x/tools/go/packages"
)

// moduleCopyPackageTree is the type-checked view of one module source tree
// (every package under one root, loaded in a single packages.Load call). The
// load shares one token.FileSet across all packages, so the file declaring
// any used object is always fset.Position(obj.Pos()).Filename, no matter
// which package of the tree the use appears in — this is what lets reference
// walks cross package boundaries inside the tree.
type moduleCopyPackageTree struct {
	fset *token.FileSet
	// files maps the canonical path of every non-test Go source file in the
	// tree to its syntax and owning package. Membership in this map is the
	// tree-membership test for reference targets.
	files map[string]moduleCopyTreeFile
	// packages maps the import path of every package in the tree to the
	// package, which is how a file outside the tree importing one is read
	// (see referencedByImporter).
	packages map[string]*packages.Package
}

type moduleCopyTreeFile struct {
	pkg    *packages.Package
	syntax *ast.File
}

// loadModuleCopyPackageTree type-checks every package under root. The tree
// must compile: copy-time reference analysis is only trustworthy on a source
// tree the framework itself builds.
func loadModuleCopyPackageTree(root string) (*moduleCopyPackageTree, error) {
	fset := token.NewFileSet()
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  root,
		Fset: fset,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, err
	}
	if packages.PrintErrors(pkgs) > 0 {
		return nil, fmt.Errorf("failed to load module source packages under %s", root)
	}

	tree := &moduleCopyPackageTree{fset: fset, files: make(map[string]moduleCopyTreeFile), packages: make(map[string]*packages.Package, len(pkgs))}
	for _, pkg := range pkgs {
		tree.packages[pkg.PkgPath] = pkg
		for idx, file := range pkg.CompiledGoFiles {
			if !isGoSourceFile(filepath.Base(file)) || idx >= len(pkg.Syntax) {
				continue
			}
			abs, absErr := canonicalModuleCopyPath(file)
			if absErr != nil {
				return nil, absErr
			}
			tree.files[abs] = moduleCopyTreeFile{pkg: pkg, syntax: pkg.Syntax[idx]}
		}
	}
	return tree, nil
}

// declFile returns the canonical path of the tree file declaring obj, or ""
// when obj is declared outside the tree (or has no position, like the
// predeclared universe objects).
func (t *moduleCopyPackageTree) declFile(obj types.Object) string {
	if obj == nil || !obj.Pos().IsValid() {
		return ""
	}
	abs, err := canonicalModuleCopyPath(t.fset.Position(obj.Pos()).Filename)
	if err != nil {
		return ""
	}
	if _, ok := t.files[abs]; !ok {
		return ""
	}
	return abs
}

// referencedTreeFiles returns the other tree files declaring objects that the
// given file uses, sorted for determinism.
func (t *moduleCopyPackageTree) referencedTreeFiles(path string) []string {
	file, ok := t.files[path]
	if !ok {
		return nil
	}
	seen := make(map[string]bool)
	ast.Inspect(file.syntax, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		declFile := t.declFile(file.pkg.TypesInfo.Uses[ident])
		if declFile != "" && declFile != path {
			seen[declFile] = true
		}
		return true
	})
	return sortedPaths(seen)
}

// referencedByImporter returns the tree files declaring the package-level
// objects that the Go file at path uses through its imports of tree
// packages, sorted for determinism. The file is one outside the tree, a
// middleware or interceptor of the module, so it is not type-checked with
// it: it is parsed alone, and a use is a selector on the name an import of
// a tree package goes by in the file, resolved in that package's scope. A
// file importing no tree package uses nothing of it.
func (t *moduleCopyPackageTree) referencedByImporter(path string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	imported := make(map[string]*packages.Package)
	for _, imp := range file.Imports {
		importPath, unquoteErr := strconv.Unquote(imp.Path.Value)
		if unquoteErr != nil {
			continue
		}
		pkg, ok := t.packages[importPath]
		if !ok {
			continue
		}
		name := pkg.Name
		if imp.Name != nil {
			name = imp.Name.Name
		}
		imported[name] = pkg
	}
	if len(imported) == 0 {
		return nil, nil
	}
	seen := make(map[string]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		pkg, ok := imported[ident.Name]
		if !ok {
			return true
		}
		if declFile := t.declFile(pkg.Types.Scope().Lookup(selector.Sel.Name)); declFile != "" {
			seen[declFile] = true
		}
		return true
	})
	return sortedPaths(seen), nil
}

// sortedPaths returns the paths of set, sorted.
func sortedPaths(set map[string]bool) []string {
	paths := make([]string, 0, len(set))
	for path := range set {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

// filesInDir returns the tree files that sit directly in dir, sorted.
func (t *moduleCopyPackageTree) filesInDir(dir string) []string {
	files := make([]string, 0)
	for path := range t.files {
		if filepath.Dir(path) == dir {
			files = append(files, path)
		}
	}
	sort.Strings(files)
	return files
}
