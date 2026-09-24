package main

import (
	"go/types"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"golang.org/x/tools/go/packages"
)

// The helpers more than one check uses, or a check shares with run. A helper
// only one check uses lives in that check's file.

// load loads the packages matching patterns under dir with their tests,
// syntax and type information, reading the overlay in place of the files it
// names.
func load(dir string, overlay map[string][]byte, patterns ...string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports |
			packages.NeedForTest,
		Tests:   true,
		Dir:     dir,
		Overlay: overlay,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, errors.Wrap(err, "load packages")
	}
	return pkgs, nil
}

// isInternalTestVariant reports whether p is a package compiled together with
// its internal test files, as go test builds it: the variant of a package
// built for its own tests. It asks ForTest rather than reading the package ID,
// whose syntax go/packages leaves to the build system and tells clients not to
// interpret.
func isInternalTestVariant(p *packages.Package) bool {
	return p.ForTest != "" && p.ForTest == p.PkgPath
}

// relative returns path relative to root in slash form, or path itself when it
// lies outside root.
func relative(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// derefNamed returns the named type t is, or points to.
func derefNamed(t types.Type) (*types.Named, bool) {
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	named, ok := t.(*types.Named)
	return named, ok
}
