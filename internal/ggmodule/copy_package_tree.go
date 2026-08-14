package ggmodule

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"

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

	tree := &moduleCopyPackageTree{fset: fset, files: make(map[string]moduleCopyTreeFile)}
	for _, pkg := range pkgs {
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
	referenced := make([]string, 0, len(seen))
	for declFile := range seen {
		referenced = append(referenced, declFile)
	}
	sort.Strings(referenced)
	return referenced
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
