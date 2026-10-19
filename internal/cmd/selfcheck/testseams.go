package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	"golang.org/x/tools/go/packages"
)

// testHooksFile names the one file of an internal package that declares its
// test seams: the exported functions and variables that exist for the tests
// of other packages, and that no path the framework runs calls.
const testHooksFile = "testhooks.go"

// apiDirective opens the doc comment line that marks an export only tests
// use today as part of an API by design, with the reason after it: a
// constructor of a type production uses through its methods, the counterpart
// of a function production does use. The line reads //gst:api <reason>.
const apiDirective = "//gst:api"

// projectSourceDirs lists the directories under the module root whose
// packages are written as project code and exported for it: the module
// sources gg module copy copies into a project, and the test support the
// public testutil package builds on, which a project's tests reach through
// it.
var projectSourceDirs = []string{"internal/model/", "internal/service/", "internal/testutil/"}

// checkTestSeams reports the exported package-level functions and variables
// of the internal packages under root that only test files use anywhere in
// the module while being declared outside the package's testhooks.go, and
// those declared in a testhooks.go that a non-test file uses, unless the
// export carries an apiDirective. The directive is held to its word: one
// without a reason is reported, and so is one on an export that non-test
// code does use, which the directive then misdescribes. Judged are the
// packages a non-test file of the module imports: in one nothing imports
// outside tests, a package kept for later or a backend the framework keeps
// as an alternative, every export is used by tests alone without being a
// seam. A main package is left alone too, since nothing can import it; so is
// a test support package, one with a non-test file importing testing, whose
// exports exist for tests by design; and so are the packages of
// projectSourceDirs. A use in a file the build leaves out, one behind a build
// constraint, counts as a use outside tests, read from its syntax by name;
// the test binary's main package go/packages synthesizes, which imports the
// package it tests, counts for nothing.
func checkTestSeams(root string, pkgs []*packages.Package) ([]violation, error) {
	if len(pkgs) == 0 || pkgs[0].Module == nil {
		return nil, nil
	}
	module := pkgs[0].Module.Path

	type decl struct {
		kind      string
		pkgPath   string
		pkgDir    string
		file      string
		directive *string // the apiDirective's reason, nil without one
	}
	decls := make(map[string]decl)
	testUse := make(map[string]bool)
	otherUse := make(map[string]string)
	ignoredNames := make(map[string]bool)
	importedOutsideTests := make(map[string]bool)

	for _, p := range pkgs {
		if strings.HasSuffix(p.PkgPath, ".test") {
			continue
		}
		for i, f := range p.Syntax {
			if isTestFile(p.CompiledGoFiles[i]) {
				continue
			}
			for _, imp := range f.Imports {
				importedOutsideTests[strings.Trim(imp.Path.Value, `"`)] = true
			}
		}
		if p.Name == "main" || !isInternalPackage(module, p.PkgPath) || isProjectSourcePackage(module, p) || isTestSupportPackage(p) {
			// Its uses of other packages' exports still count below.
		} else {
			directives := apiDirectives(p)
			for _, obj := range packageLevelExports(p) {
				key := p.PkgPath + "." + obj.Name()
				file := p.Fset.Position(obj.Pos()).Filename
				if isTestFile(file) {
					continue
				}
				kind := "function"
				if _, ok := obj.(*types.Var); ok {
					kind = "variable"
				}
				d := decl{kind: kind, pkgPath: p.PkgPath, pkgDir: filepath.Dir(file), file: file}
				if reason, ok := directives[obj.Name()]; ok {
					d.directive = &reason
				}
				decls[key] = d
			}
		}
		for id, obj := range p.TypesInfo.Uses {
			if !isPackageLevelExport(obj) || obj.Pkg() == nil {
				continue
			}
			key := obj.Pkg().Path() + "." + obj.Name()
			file := p.Fset.Position(id.Pos()).Filename
			if isTestFile(file) {
				testUse[key] = true
			} else if _, seen := otherUse[key]; !seen {
				otherUse[key] = file
			}
		}
		if err := collectIgnoredNames(p, ignoredNames, importedOutsideTests); err != nil {
			return nil, err
		}
	}

	var violations []violation
	report := func(d decl, format string, args ...any) {
		violations = append(violations, violation{File: relative(root, d.file), Message: fmt.Sprintf(format, args...)})
	}
	for key, d := range decls {
		if !importedOutsideTests[d.pkgPath] {
			continue
		}
		name := key[strings.LastIndex(key, ".")+1:]
		pkg := relative(root, d.pkgDir)
		inHooks := filepath.Base(d.file) == testHooksFile
		user, used := otherUse[key]
		switch {
		case used && inHooks:
			report(d, "Exported %s '%s' declared in %s is used outside tests, at %s: it is no test seam, declare it beside the code it serves", d.kind, name, relative(root, d.file), relative(root, user))
		case used && d.directive != nil:
			report(d, "%s on %s '%s' of %s is stale: non-test code uses it at %s; drop the directive", apiDirective, d.kind, name, pkg, relative(root, user))
		case used, inHooks, !testUse[key], ignoredNames[name]:
		case d.directive == nil:
			report(d, "Exported %s '%s' of %s is used by tests alone: declare it in %s/%s, the file of the package's test seams, mark it %s <why it is an API> in its doc comment, or drop it", d.kind, name, pkg, pkg, testHooksFile, apiDirective)
		case *d.directive == "":
			report(d, "%s on %s '%s' of %s gives no reason: say why it is an API after the directive", apiDirective, d.kind, name, pkg)
		}
	}
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].File != violations[j].File {
			return violations[i].File < violations[j].File
		}
		return violations[i].Message < violations[j].Message
	})
	return violations, nil
}

// apiDirectives returns the reasons the apiDirective lines in the doc
// comments of p's non-test files give, by the name of the function or
// variable they are on; a directive with nothing after it maps to "".
func apiDirectives(p *packages.Package) map[string]string {
	reasons := make(map[string]string)
	record := func(doc *ast.CommentGroup, names ...*ast.Ident) {
		if doc == nil {
			return
		}
		for _, c := range doc.List {
			rest, ok := strings.CutPrefix(c.Text, apiDirective)
			if !ok || (rest != "" && !strings.HasPrefix(rest, " ")) {
				continue
			}
			for _, name := range names {
				reasons[name.Name] = strings.TrimSpace(rest)
			}
		}
	}
	for i, f := range p.Syntax {
		if isTestFile(p.CompiledGoFiles[i]) {
			continue
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil {
					record(d.Doc, d.Name)
				}
			case *ast.GenDecl:
				if d.Tok != token.VAR {
					continue
				}
				for _, spec := range d.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						record(d.Doc, vs.Names...)
						record(vs.Doc, vs.Names...)
					}
				}
			}
		}
	}
	return reasons
}

// isInternalPackage reports whether pkgPath is an internal package of module.
func isInternalPackage(module, pkgPath string) bool {
	rel, ok := strings.CutPrefix(pkgPath, module+"/")
	return ok && (rel == "internal" || strings.HasPrefix(rel, "internal/") || strings.Contains(rel, "/internal/") || strings.HasSuffix(rel, "/internal"))
}

// isProjectSourcePackage reports whether p lies in a directory of
// projectSourceDirs.
func isProjectSourcePackage(module string, p *packages.Package) bool {
	rel, _ := strings.CutPrefix(p.PkgPath, module+"/")
	for _, dir := range projectSourceDirs {
		if strings.HasPrefix(rel+"/", dir) {
			return true
		}
	}
	return false
}

// isTestSupportPackage reports whether p is a test support package, one with
// a non-test file importing testing.
func isTestSupportPackage(p *packages.Package) bool {
	for i, f := range p.Syntax {
		if isTestFile(p.CompiledGoFiles[i]) {
			continue
		}
		for _, imp := range f.Imports {
			if imp.Path.Value == `"testing"` {
				return true
			}
		}
	}
	return false
}

// packageLevelExports returns the exported functions and variables declared
// at package level in p.
func packageLevelExports(p *packages.Package) []types.Object {
	scope := p.Types.Scope()
	var objs []types.Object
	for _, name := range scope.Names() {
		if obj := scope.Lookup(name); isPackageLevelExport(obj) {
			objs = append(objs, obj)
		}
	}
	return objs
}

// isPackageLevelExport reports whether obj is an exported function or
// variable declared at package level.
func isPackageLevelExport(obj types.Object) bool {
	if obj == nil || !obj.Exported() || obj.Pkg() == nil || obj.Parent() != obj.Pkg().Scope() {
		return false
	}
	switch obj.(type) {
	case *types.Func, *types.Var:
		return true
	}
	return false
}

// collectIgnoredNames adds to names every identifier, and to imported every
// import path, of the non-test Go files of p's directory that the build left
// out, the files behind a build constraint the load did not satisfy: a use
// among them cannot be typed, so the check reads it by name.
func collectIgnoredNames(p *packages.Package, names map[string]bool, imported map[string]bool) error {
	fset := token.NewFileSet()
	for _, path := range p.IgnoredFiles {
		if !strings.HasSuffix(path, ".go") || isTestFile(path) {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return errors.Wrapf(err, "parse %s", path)
		}
		for _, imp := range f.Imports {
			imported[strings.Trim(imp.Path.Value, `"`)] = true
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				names[id.Name] = true
			}
			return true
		})
	}
	return nil
}
