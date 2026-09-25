package ts

import (
	"go/types"

	"github.com/hydroan/gst/internal/codegen/gen/jsonshape"
)

// generator walks the types reachable from the roots and renders their
// declarations.
type generator struct {
	cfg     Config
	project *jsonshape.Project // the loaded project the shapes are read from
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

// newGenerator prepares a generation run of cfg over the loaded project, and
// names the prelude file after the application (see preludeFileName).
func newGenerator(cfg Config, project *jsonshape.Project) *generator {
	return &generator{
		cfg:      cfg,
		project:  project,
		decls:    make(map[*types.TypeName]*declaration),
		enums:    make(map[*types.TypeName]*enumType),
		inlining: make(map[string]bool),

		preludeFile: preludeFileName(cfg.AppName),
	}
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
	g.project.CheckForeignConstants()
	files := g.files()
	if diags := g.project.Diagnostics(); len(diags) > 0 {
		return nil, &DiagnosticsError{Diagnostics: diags}
	}
	return files, nil
}

// declareRoot queues the declaration of a root type.
func (g *generator) declareRoot(ref TypeRef) {
	s := jsonshape.Site{Subject: ref.PkgPath + "." + ref.Name}
	pkg := g.project.Package(ref.PkgPath)
	if pkg == nil || pkg.Types == nil {
		g.project.Report(s, "the package is not part of module %s", g.cfg.ModulePath)
		return
	}
	obj, ok := pkg.Types.Scope().Lookup(ref.Name).(*types.TypeName)
	if !ok {
		g.project.Report(s, "the package declares no type %s", ref.Name)
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
