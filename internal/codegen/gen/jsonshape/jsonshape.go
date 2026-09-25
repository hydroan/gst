// Package jsonshape reads what the types of a gst project look like on the
// wire: the JSON encoding/json writes for them, which is the codec of the
// framework's response envelope and request binding. Every generator that
// describes the API in another language reads the shape from here -- the
// TypeScript declarations do -- so all of them describe one and the same
// shape.
//
// Load type-checks the project packages the API's types are declared in,
// from source, and indexes their syntax for the doc comments and constants a
// description carries over. The Project it returns then answers, for any type
// reached from those packages, which keys a struct encodes to (Fields), which
// types decide their encoding through methods of their own (MarshalMethod),
// which named types are enums (Enum), and which values may be null or absent
// (Nullable, Nilable). A shape that cannot be read off a declaration is
// reported on the Project as a Diagnostic, at the position the project
// declares the subject.
package jsonshape

import (
	"go/token"
	"go/types"
	"path/filepath"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
)

// Config describes one load.
type Config struct {
	// Dir is the directory the Go packages are loaded from, normally the root
	// of the project module. Diagnostic file names are relative to it.
	Dir string
	// ModulePath is the import path prefix of the project packages: the ones
	// type-checked from source, whose types get descriptions of their own.
	ModulePath string
	// Roots are the import paths of the packages declaring the types the API
	// sends and receives. The project packages they reach are loaded.
	Roots []string
}

// Project is a loaded project: its type-checked packages, the doc comments
// and constants read off their syntax, and the diagnostics of one description
// run over them. It serves that one run: the diagnostics it collects are the
// run's, and a generator reads them once its run is over.
type Project struct {
	dir        string // absolute Config.Dir
	fset       *token.FileSet
	pkgs       map[string]*packages.Package // the project packages, keyed by import path
	sources    map[string]*sourceIndex      // the syntax index of each project package, keyed by import path
	methodSets typeutil.MethodSetCache      // caches the method sets MarshalMethod searches
	enums      map[*types.TypeName]*Enum    // Enum results, nil for a type that is no enum

	diags    []Diagnostic
	reported map[string]bool // the text of every diagnostic in diags, to report each once
}

// Load type-checks the project packages the roots reach (see load) and
// indexes their syntax (see newSourceIndex).
func Load(cfg Config) (*Project, error) {
	fset, pkgs, err := load(cfg)
	if err != nil {
		return nil, err
	}
	p := &Project{
		dir:      absoluteDir(cfg.Dir),
		fset:     fset,
		pkgs:     pkgs,
		sources:  make(map[string]*sourceIndex, len(pkgs)),
		enums:    make(map[*types.TypeName]*Enum),
		reported: make(map[string]bool),
	}
	for pkgPath, pkg := range pkgs {
		p.sources[pkgPath] = newSourceIndex(fset, pkg)
	}
	return p, nil
}

// absoluteDir returns dir absolute, with its symbolic links resolved, the form
// the loader reports file names in; a dir that cannot be resolved is kept as
// given.
func absoluteDir(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

// RelativeFile returns the name of file relative to the directory the
// packages were loaded from, or file itself when it lies outside. File names
// come from the loader, with symbolic links resolved on both sides.
func (p *Project) RelativeFile(file string) string {
	if resolved, err := filepath.EvalSymlinks(file); err == nil {
		file = resolved
	}
	if rel, err := filepath.Rel(p.dir, file); err == nil && filepath.IsLocal(rel) {
		return rel
	}
	return file
}

// Package returns the project package at pkgPath, or nil for an import path
// that names no project package.
func (p *Project) Package(pkgPath string) *packages.Package { return p.pkgs[pkgPath] }

// FileSet returns the file set the packages were loaded into, which the
// positions of their syntax and types resolve against.
func (p *Project) FileSet() *token.FileSet { return p.fset }

// Declares reports whether obj gets a description of its own: a type declared
// at package scope in a project package.
func (p *Project) Declares(obj *types.TypeName) bool {
	return obj.Pkg() != nil && p.sources[obj.Pkg().Path()] != nil && obj.Parent() == obj.Pkg().Scope()
}

// TypeDoc returns the doc comment of a project type, picked the way the
// OpenAPI document picks it: the type's own comment, or that of its
// declaration group. A type from outside the project has none.
func (p *Project) TypeDoc(obj *types.TypeName) string {
	if obj.Pkg() == nil {
		return ""
	}
	if source := p.sources[obj.Pkg().Path()]; source != nil {
		return source.typeDocs[obj]
	}
	return ""
}

// FieldDoc returns the doc comment of a project struct field, or its trailing
// comment when it has none. A field of a type from outside the project has
// none.
func (p *Project) FieldDoc(v *types.Var) string {
	if v.Pkg() == nil {
		return ""
	}
	if source := p.sources[v.Pkg().Path()]; source != nil {
		return source.fieldDocs[v]
	}
	return ""
}
