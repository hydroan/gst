package ts

import (
	"cmp"
	"fmt"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
)

// generator walks the types reachable from the roots and renders their
// declarations.
type generator struct {
	cfg        Config
	dir        string // absolute Config.Dir
	fset       *token.FileSet
	pkgs       map[string]*packages.Package // the project packages, keyed by import path
	sources    map[string]*sourceIndex      // the syntax index of each project package, keyed by import path
	methodSets typeutil.MethodSetCache      // caches the method sets g.method searches
	// preludeFile is the file the framework prelude goes to, named after the
	// application.
	preludeFile string

	// decls holds an entry for every project type queued so far; the entry
	// stays nil until the type is rendered.
	decls map[*types.TypeName]*declaration
	queue []*types.TypeName             // the queued types not rendered yet
	enums map[*types.TypeName]*enumType // enumOf results, nil for a type that is no enum
	// inlining holds the types from outside the project being spelled out,
	// to stop at one that refers to itself.
	inlining map[string]bool

	diags    []Diagnostic
	reported map[string]bool // the text of every diagnostic in diags, to report each once
}

// declaration is the rendered TypeScript declaration of one project type.
type declaration struct {
	obj     *types.TypeName
	text    string          // the declaration under its doc comment, empty for a type reported instead
	imports map[string]bool // paths of the packages the text refers to
}

// fileContext collects the imports of the file a declaration goes to.
type fileContext struct {
	pkgPath string
	imports map[string]bool
}

// newGenerator prepares a generation run of cfg over the loaded packages: it
// indexes the syntax of every project package (see newSourceIndex) and names
// the prelude file after the application (see preludeFileName).
func newGenerator(cfg Config, l *loaded) *generator {
	dir, err := filepath.Abs(cfg.Dir)
	if err != nil {
		dir = cfg.Dir
	}
	g := &generator{
		cfg:      cfg,
		dir:      dir,
		fset:     l.fset,
		pkgs:     l.pkgs,
		sources:  make(map[string]*sourceIndex, len(l.pkgs)),
		decls:    make(map[*types.TypeName]*declaration),
		enums:    make(map[*types.TypeName]*enumType),
		inlining: make(map[string]bool),
		reported: make(map[string]bool),

		preludeFile: preludeFileName(cfg.AppName),
	}
	for pkgPath, pkg := range l.pkgs {
		g.sources[pkgPath] = newSourceIndex(l.fset, pkg)
	}
	return g
}

// generate renders the declarations reachable from the roots, or reports every
// diagnostic found on the way.
func (g *generator) generate() ([]File, error) {
	for _, ref := range g.cfg.Roots {
		g.declareRoot(ref)
	}
	for len(g.queue) > 0 {
		obj := g.queue[0]
		g.queue = g.queue[1:]
		g.decls[obj] = g.render(obj)
	}
	g.checkForeignConstants()
	files := g.files()
	if len(g.diags) > 0 {
		slices.SortFunc(g.diags, func(a, b Diagnostic) int {
			return cmp.Or(
				strings.Compare(a.Pos.Filename, b.Pos.Filename),
				cmp.Compare(a.Pos.Line, b.Pos.Line),
				cmp.Compare(a.Pos.Column, b.Pos.Column),
				strings.Compare(a.Subject, b.Subject),
				strings.Compare(a.Message, b.Message),
			)
		})
		return nil, &DiagnosticsError{Diagnostics: g.diags}
	}
	return files, nil
}

// declareRoot queues the declaration of a root type.
func (g *generator) declareRoot(ref TypeRef) {
	s := site{subject: ref.PkgPath + "." + ref.Name}
	pkg := g.pkgs[ref.PkgPath]
	if pkg == nil || pkg.Types == nil {
		g.report(s, "the package is not part of module %s", g.cfg.ModulePath)
		return
	}
	obj, ok := pkg.Types.Scope().Lookup(ref.Name).(*types.TypeName)
	if !ok {
		g.report(s, "the package declares no type %s", ref.Name)
		return
	}
	g.enqueue(obj)
}

// enqueue queues the declaration of a project type, once.
func (g *generator) enqueue(obj *types.TypeName) {
	if _, queued := g.decls[obj]; queued {
		return
	}
	g.decls[obj] = nil
	g.queue = append(g.queue, obj)
}

// declares reports whether obj gets a declaration of its own: a type declared
// at package scope in a project package.
func (g *generator) declares(obj *types.TypeName) bool {
	return obj.Pkg() != nil && g.sources[obj.Pkg().Path()] != nil && obj.Parent() == obj.Pkg().Scope()
}

// site locates the subject of a diagnostic.
type site struct {
	subject string
	pos     token.Pos
}

// report records a diagnostic, once.
func (g *generator) report(s site, format string, args ...any) {
	d := Diagnostic{Subject: s.subject, Message: fmt.Sprintf(format, args...)}
	if s.pos.IsValid() {
		d.Pos = g.fset.Position(s.pos)
		if rel, err := filepath.Rel(g.dir, d.Pos.Filename); err == nil && filepath.IsLocal(rel) {
			d.Pos.Filename = rel
		}
	}
	if key := d.String(); !g.reported[key] {
		g.reported[key] = true
		g.diags = append(g.diags, d)
	}
}
