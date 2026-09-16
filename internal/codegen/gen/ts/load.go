package ts

import (
	"go/token"
	"os"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"golang.org/x/tools/go/packages"
)

// loaded holds the project packages generation reads, type-checked from
// source together with their syntax: declarations take their doc comments and
// enum constants from it. The packages outside the project are read from
// compiled export data.
type loaded struct {
	fset *token.FileSet
	pkgs map[string]*packages.Package
}

// load type-checks the project packages the root packages reach, in two
// passes. The first lists the import graph only, to find which project
// packages are reachable; the second type-checks just those from source.
// Loading the whole graph from source instead would parse and check every
// dependency, the framework and the standard library included.
func load(cfg Config) (*loaded, error) {
	roots := make([]string, 0, len(cfg.Roots))
	for _, ref := range cfg.Roots {
		roots = append(roots, ref.PkgPath)
	}
	slices.Sort(roots)
	roots = slices.Compact(roots)

	result := &loaded{fset: token.NewFileSet(), pkgs: make(map[string]*packages.Package)}
	if len(roots) == 0 {
		return result, nil
	}

	// A go.work above the project would pull every workspace module into the
	// load, while the routes build against the project's own go.mod.
	env := append(os.Environ(), "GOWORK=off")
	graph, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedImports | packages.NeedDeps,
		Dir:  cfg.Dir,
		Env:  env,
	}, roots...)
	if err != nil {
		return nil, errors.Wrap(err, "list packages")
	}
	if err = packageErrors(graph); err != nil {
		return nil, err
	}
	var project []string
	packages.Visit(graph, nil, func(pkg *packages.Package) {
		if inModule(pkg.PkgPath, cfg.ModulePath) {
			project = append(project, pkg.PkgPath)
		}
	})
	slices.Sort(project)

	typed, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports |
			packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  cfg.Dir,
		Env:  env,
		Fset: result.fset,
	}, project...)
	if err != nil {
		return nil, errors.Wrap(err, "load packages")
	}
	if err = packageErrors(typed); err != nil {
		return nil, err
	}
	for _, pkg := range typed {
		result.pkgs[pkg.PkgPath] = pkg
	}
	return result, nil
}

// packageErrors reports the errors of the loaded packages and their
// dependencies. Types read off a package that does not compile could describe
// anything, so any error stops generation.
func packageErrors(pkgs []*packages.Package) error {
	var lines []string
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		for _, e := range pkg.Errors {
			lines = append(lines, "  "+e.Error())
		}
	})
	if len(lines) == 0 {
		return nil
	}
	return errors.Newf("the packages do not type-check:\n%s", strings.Join(lines, "\n"))
}

// inModule reports whether pkgPath is modulePath or a package below it.
func inModule(pkgPath, modulePath string) bool {
	return pkgPath == modulePath || strings.HasPrefix(pkgPath, modulePath+"/")
}
