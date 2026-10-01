package pb

import (
	"cmp"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"go/types"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/gggen/jsonshape"
	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"
)

// This file holds the type check of the Go files gg gen is about to write
// under pb/: the handlers and registrations it rendered and the files the
// protobuf plugins compiled, checked together as the packages they make up,
// against the project's packages and its dependencies, before any of them
// is written. A generated file the compiler would refuse is a defect of the
// generator; the check turns it into a diagnostic naming the file and the
// line, where the project would otherwise meet it in go build.

// ErrUnchecked is what TypeCheck wraps when the generated files could not
// be checked at all, the packages they import failing to list: a project
// whose module graph the go command cannot resolve yet, before go mod tidy.
// The files are written unchecked then, go build being where they are
// checked next.
var ErrUnchecked = errors.New("the generated Go files could not be type-checked")

// TypeCheck type-checks the Go files among files, the packages gg gen
// writes under pb/ in the project at dir, module modulePath: each package
// is checked from its files the way the go command compiles it once
// written, the project packages and the dependencies it imports read from
// their compiled export data, and a generated package imported by another
// generated one checked first. Every error is reported as a diagnostic at
// the file and line of the generated file, in a *DiagnosticsError; nil
// when the packages type-check; an error wrapping ErrUnchecked when the
// imports could not be listed.
func TypeCheck(dir, modulePath string, files []File) error {
	generated := make(map[string][]File) // the Go files of each generated package, by import path
	for _, f := range files {
		if f.Definition() {
			continue
		}
		pkgPath := path.Join(modulePath, path.Dir(f.Path))
		generated[pkgPath] = append(generated[pkgPath], f)
	}
	if len(generated) == 0 {
		return nil
	}

	fset := token.NewFileSet()
	parsed := make(map[string][]*ast.File, len(generated))
	var diags []jsonshape.Diagnostic
	var external []string
	for pkgPath, pkgFiles := range generated {
		for _, f := range pkgFiles {
			file, err := parser.ParseFile(fset, f.Path, f.Content, parser.SkipObjectResolution)
			if err != nil {
				var list scanner.ErrorList
				if !errors.As(err, &list) {
					return errors.Wrapf(err, "parse %s", f.Path)
				}
				for _, e := range list {
					diags = append(diags, jsonshape.Diagnostic{Pos: e.Pos, Subject: pkgPath, Message: e.Msg})
				}
				continue
			}
			parsed[pkgPath] = append(parsed[pkgPath], file)
			for _, spec := range file.Imports {
				importPath, _ := strconv.Unquote(spec.Path.Value)
				if _, ok := generated[importPath]; !ok && importPath != "unsafe" {
					external = append(external, importPath)
				}
			}
		}
	}
	if len(diags) > 0 {
		return &DiagnosticsError{Diagnostics: sortedDiagnostics(diags)}
	}
	slices.Sort(external)
	external = slices.Compact(external)

	// A go.work above the project would pull every workspace module into
	// the load, while the files build against the project's own go.mod.
	loaded, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedImports | packages.NeedDeps | packages.NeedTypes,
		Dir:  dir,
		Env:  append(os.Environ(), "GOWORK=off"),
	}, external...)
	if err != nil {
		return errors.Wrapf(ErrUnchecked, "list the packages the files import: %v", err)
	}
	imports := make(map[string]*types.Package)
	var listErr error
	packages.Visit(loaded, nil, func(pkg *packages.Package) {
		if listErr == nil && len(pkg.Errors) > 0 {
			listErr = errors.Wrapf(ErrUnchecked, "%s: %v", pkg.PkgPath, pkg.Errors[0])
		}
		if pkg.Types != nil {
			imports[pkg.PkgPath] = pkg.Types
		}
	})
	if listErr != nil {
		return listErr
	}

	// The generated packages are checked on demand, a package imported by
	// another one on the way, so that the importer serves the checked
	// package; nil marks one being checked, which only an import cycle
	// asks for again.
	checked := make(map[string]*types.Package, len(generated))
	goVersion := moduleGoVersion(dir)
	var check func(pkgPath string) (*types.Package, error)
	importer := importerFunc(func(importPath string) (*types.Package, error) {
		if importPath == "unsafe" {
			return types.Unsafe, nil
		}
		if pkg, ok := checked[importPath]; ok {
			if pkg == nil {
				return nil, errors.Newf("import cycle through %s", importPath)
			}
			return pkg, nil
		}
		if _, ok := generated[importPath]; ok {
			return check(importPath)
		}
		if pkg, ok := imports[importPath]; ok {
			return pkg, nil
		}
		return nil, errors.Newf("package %s is not loaded", importPath)
	})
	check = func(pkgPath string) (*types.Package, error) {
		checked[pkgPath] = nil
		conf := types.Config{
			Importer:  importer,
			GoVersion: goVersion,
			Error: func(err error) {
				var typeErr types.Error
				if errors.As(err, &typeErr) {
					diags = append(diags, jsonshape.Diagnostic{Pos: fset.Position(typeErr.Pos), Subject: pkgPath, Message: typeErr.Msg})
				}
			},
		}
		pkg, err := conf.Check(pkgPath, fset, parsed[pkgPath], nil)
		checked[pkgPath] = pkg
		return pkg, err
	}
	for _, pkgPath := range slices.Sorted(maps.Keys(generated)) {
		if _, ok := checked[pkgPath]; !ok {
			_, _ = check(pkgPath)
		}
	}
	if len(diags) > 0 {
		return &DiagnosticsError{Diagnostics: sortedDiagnostics(diags)}
	}
	return nil
}

// importerFunc is a types.Importer made of a function.
type importerFunc func(importPath string) (*types.Package, error)

func (f importerFunc) Import(importPath string) (*types.Package, error) { return f(importPath) }

// moduleGoVersion returns the Go language version the go.mod of the project
// at dir declares, go1.27 for a go 1.27 directive, which the type check
// reads the generated files under; "" for none, the latest version then.
func moduleGoVersion(dir string) string {
	content, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return ""
	}
	file, err := modfile.ParseLax("go.mod", content, nil)
	if err != nil || file.Go == nil {
		return ""
	}
	return "go" + file.Go.Version
}

// sortedDiagnostics returns diags ordered by file, line and column, each
// reported once.
func sortedDiagnostics(diags []jsonshape.Diagnostic) []jsonshape.Diagnostic {
	slices.SortFunc(diags, func(a, b jsonshape.Diagnostic) int {
		return cmp.Or(
			strings.Compare(a.Pos.Filename, b.Pos.Filename),
			cmp.Compare(a.Pos.Line, b.Pos.Line),
			cmp.Compare(a.Pos.Column, b.Pos.Column),
			strings.Compare(a.Message, b.Message),
		)
	})
	return slices.CompactFunc(diags, func(a, b jsonshape.Diagnostic) bool { return a.String() == b.String() })
}
