// Command selfcheck holds the framework's own source to the rules
// golangci-lint cannot express, and prints each violation it finds.
//
// Run it from the repository root through `make check`. The rules bind the
// framework alone: make check runs them over the framework, and a project is
// held to none of them.
//
// # Test placement
//
// golangci-lint's testpackage makes a test file that joins the package it
// tests say so: its name ends in _internal_test.go. That settles the name but
// not the need, and this check makes the suffix true. A file can carry the
// suffix and still use nothing unexported of its package, or even declare the
// external test package; the check reports both, so an internal test is always
// one that could not be written from outside.
package main

import (
	"fmt"
	"go/types"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"golang.org/x/tools/go/packages"
)

// violation is a breach of one of the rules, found in one file.
type violation struct {
	// File is the path of the file, relative to the checked directory.
	File string
	// Message says what is wrong and how to fix it.
	Message string
}

// check is one of the rules selfcheck holds the framework to.
type check struct {
	// name prefixes the errors the check returns.
	name string
	// run reports the violations of the rule among pkgs, the packages under
	// root as load returns them.
	run func(root string, pkgs []*packages.Package) ([]violation, error)
}

// checks lists the rules in the order they run and report.
var checks = []check{
	{name: "testplacement", run: checkTestPlacement},
}

func main() {
	violations, err := run(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "selfcheck:", err)
		os.Exit(1)
	}
	for _, v := range violations {
		fmt.Println(v.Message)
	}
	if len(violations) > 0 {
		os.Exit(1)
	}
}

// run loads the packages under dir once, tests included, and runs every check
// over them, returning what the checks find in the order they run. A package
// that does not type-check stops the run: no check can judge code the
// compiler rejects.
func run(dir string) ([]violation, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, errors.Wrap(err, "resolve the checked directory")
	}
	pkgs, err := load(root, nil, "./...")
	if err != nil {
		return nil, err
	}
	for _, p := range pkgs {
		if len(p.Errors) > 0 {
			return nil, errors.Newf("package %s does not type-check: %v", p.ID, p.Errors[0])
		}
	}

	var violations []violation
	for _, c := range checks {
		found, err := c.run(root, pkgs)
		if err != nil {
			return nil, errors.Wrap(err, c.name)
		}
		violations = append(violations, found...)
	}
	return violations, nil
}

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
