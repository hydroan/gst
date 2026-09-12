package main

import (
	"bytes"
	"cmp"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/codegen/constants"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/types/consts"
)

// columnInspectionPanic is what the inspection reports when it runs code that
// needs the very column references it is resolving.
const columnInspectionPanic = "gg gen: this function depends on generated column references, which do not exist while gg gen resolves columns"

// columnInspectionPlaceholder is the body a function depending on generated
// column references gets in the inspection build.
const columnInspectionPlaceholder = `panic("` + columnInspectionPanic + `")`

// columnInspectionOverlay returns the build overlay the inspection program
// compiles the project with.
//
// The inspection compiles the model packages before the column references the
// run writes exist. Previously generated column files are replaced by stubs
// holding only their package clause: they may carry an API shape older than
// the running gg, and the build must not choke on the very files the run
// rewrites. A first run has no column files at all. Either way, handwritten
// model code that reads the references, such as a hook updating its model
// through the Cols var, would fail the build that produces them.
//
// The overlay breaks that cycle the way a type checker skipping function
// bodies does: every declaration that depends on a generated column
// reference, directly or through other package-level declarations, is left
// out of the inspection build. Leaving it out cannot change the resolved
// columns, which would otherwise depend on themselves. A function keeps its
// signature and gets a body that panics should the inspection call it; an
// init function gets an empty body, since the runtime runs every init; a
// package-level var is dropped, which leaves out whatever reads it in turn.
// Imports only the left-out code used become blank imports, keeping their
// initialization side effects. Every rewrite keeps the line breaks it
// replaces, so compiler messages about the remaining code point at the right
// lines.
//
// Dependence is decided by name, without type information. A local variable
// that happens to share an omitted name leaves its function out too, which
// costs nothing: the inspection calls project code only through the few model
// methods it reaches through interfaces. Methods are never omitted by name,
// since a method is selected through a value whose type is unknown here, so
// a package-level var or an init function calling a left-out method reaches
// its placeholder. Only the model directory is rewritten, the same scope the
// inspection cache key covers.
func columnInspectionOverlay(module string, modelDir string, models []*gen.ModelInfo) (map[string]string, error) {
	files, err := scanColumnInspectionFiles(module, modelDir)
	if err != nil {
		return nil, err
	}
	// The scan only sees the column vars an earlier run declared; a first
	// run, or a model added since, has none on disk yet.
	for _, m := range models {
		files.omit(modelPkgPath(m), columnVarName(m.ModelName))
	}
	rewrites, err := leaveOutColumnDependents(files)
	if err != nil {
		return nil, err
	}
	overlay := files.stubs
	maps.Copy(overlay, rewrites)
	return overlay, nil
}

// columnInspectionFiles is the model directory as the inspection build sees
// it.
type columnInspectionFiles struct {
	fset *token.FileSet

	// stubs maps every framework-owned generated column file to a stub
	// holding only its package clause.
	stubs map[string]string

	// sources are the other Go files the build compiles, kept as written
	// unless they depend on generated column references.
	sources []*columnInspectionSource

	// packageNames lists, per package import path, the package names its
	// sources declare: one, unless a file excluded from the build declares
	// another.
	packageNames map[string][]string

	// omitted holds, per package import path, the package-level names the
	// inspection build goes without: the column vars first, then every
	// function and var found to depend on them.
	omitted map[string]map[string]bool
}

// columnInspectionSource is one parsed Go file of a model package.
type columnInspectionSource struct {
	path    string
	pkg     string // Import path of the package the file belongs to.
	content []byte
	file    *ast.File
}

// scanColumnInspectionFiles reads the Go files the inspection build compiles
// from the model directory. Tests are skipped, since the inspection program
// does not build them, and so are the directories and files the go command
// ignores.
func scanColumnInspectionFiles(module string, modelDir string) (*columnInspectionFiles, error) {
	files := &columnInspectionFiles{
		fset:         token.NewFileSet(),
		stubs:        make(map[string]string),
		packageNames: make(map[string][]string),
		omitted:      make(map[string]map[string]bool),
	}
	err := filepath.WalkDir(modelDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := entry.Name()
		ignored := strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
		if entry.IsDir() {
			if path != modelDir && (ignored || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if ignored || !strings.HasSuffix(name, constants.ExtensionGo) || strings.HasSuffix(name, constants.PatternTestFile) {
			return nil
		}
		content, readErr := os.ReadFile(path) //nolint:gosec // path comes from the model directory walk.
		if readErr != nil {
			return errors.Wrapf(readErr, "read %s", path)
		}
		pkg := packageImportPath(module, filepath.Dir(path))
		// A file without the generated header is hand-written, whatever its
		// name, and keeps participating in the build.
		if isColumnFileCandidate(path) && bytes.HasPrefix(content, []byte(consts.CodeGeneratedComment())) {
			return files.addColumnFile(path, pkg, content)
		}
		files.addSource(path, pkg, content)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

// addColumnFile stubs out a generated column file, omitting every name it
// declares from the inspection build.
func (f *columnInspectionFiles) addColumnFile(path string, pkg string, content []byte) error {
	clause, err := parser.ParseFile(token.NewFileSet(), path, content, parser.PackageClauseOnly)
	if err != nil {
		return errors.Wrapf(err, "parse package clause of %s", path)
	}
	f.stubs[path] = "package " + clause.Name.Name + "\n"
	// The declared names also cover a model renamed or deleted since the
	// file was written. A file that no longer parses, such as one left with
	// merge conflict markers, still stubs out and just contributes no names.
	if parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, content, parser.SkipObjectResolution); parseErr == nil {
		for _, name := range packageLevelNames(parsed) {
			f.omit(pkg, name)
		}
	}
	return nil
}

// addSource records a file the inspection build compiles. A file that does
// not parse is left to the compiler, which reports it on its own.
func (f *columnInspectionFiles) addSource(path string, pkg string, content []byte) {
	file, err := parser.ParseFile(f.fset, path, content, parser.SkipObjectResolution)
	if err != nil {
		return
	}
	f.sources = append(f.sources, &columnInspectionSource{path: path, pkg: pkg, content: content, file: file})
	if !slices.Contains(f.packageNames[pkg], file.Name.Name) {
		f.packageNames[pkg] = append(f.packageNames[pkg], file.Name.Name)
	}
}

// omit records a package-level name the inspection build goes without and
// reports whether it was not omitted already.
func (f *columnInspectionFiles) omit(pkg string, name string) bool {
	if name == "_" || f.omitted[pkg][name] {
		return false
	}
	if f.omitted[pkg] == nil {
		f.omitted[pkg] = make(map[string]bool)
	}
	f.omitted[pkg][name] = true
	return true
}

// modelImportsOf resolves how src refers to the model packages it imports.
func (f *columnInspectionFiles) modelImportsOf(src *columnInspectionSource) modelImports {
	imports := modelImports{qualifiers: make(map[string][]string)}
	for _, spec := range src.file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		names, isModelPackage := f.packageNames[path]
		if !isModelPackage {
			continue
		}
		switch {
		case spec.Name == nil:
			for _, name := range names {
				imports.qualifiers[name] = append(imports.qualifiers[name], path)
			}
		case spec.Name.Name == ".":
			imports.dotted = append(imports.dotted, path)
		case spec.Name.Name != "_":
			imports.qualifiers[spec.Name.Name] = append(imports.qualifiers[spec.Name.Name], path)
		}
	}
	return imports
}

// modelImports is how one file refers to the model packages it imports.
type modelImports struct {
	// qualifiers maps each name the file may qualify an identifier with to
	// the model packages it stands for. A plain import is keyed by every
	// package name the package's sources declare.
	qualifiers map[string][]string

	// dotted lists the model packages the file imports with a dot.
	dotted []string
}

// columnDependents is the analysis that leaves the declarations depending on
// omitted names out of the inspection build.
type columnDependents struct {
	files   *columnInspectionFiles
	imports map[*columnInspectionSource]modelImports

	// bodies are the functions whose body is left out.
	bodies map[*ast.FuncDecl]bool

	// vars are the package-level var specs left out.
	vars map[*ast.ValueSpec]bool

	// exports caches the exported names of dot-imported packages, nil for a
	// package whose names cannot be read.
	exports map[string]map[string]bool
}

// leaveOutColumnDependents leaves out every declaration that depends on an
// omitted name and returns the content each affected file compiles with,
// keyed by path.
func leaveOutColumnDependents(files *columnInspectionFiles) (map[string]string, error) {
	d := &columnDependents{
		files:   files,
		imports: make(map[*columnInspectionSource]modelImports, len(files.sources)),
		bodies:  make(map[*ast.FuncDecl]bool),
		vars:    make(map[*ast.ValueSpec]bool),
		exports: make(map[string]map[string]bool),
	}
	for _, src := range files.sources {
		d.imports[src] = files.modelImportsOf(src)
	}
	d.resolve()

	affected := slices.DeleteFunc(slices.Clone(files.sources), func(src *columnInspectionSource) bool {
		return !d.leavesOutAnyOf(src)
	})
	rewrites := make(map[string]string, len(affected))
	if len(affected) == 0 {
		return rewrites, nil
	}
	// Telling which imports turn unused takes the name a plain import is
	// referred to by, which need not match the last element of its path, and
	// the names a dot import brings into scope. The scan knows both for model
	// packages; the go command reports the rest in one run.
	listed, err := listProjectPackages(d.unknownImports(affected))
	if err != nil {
		return nil, err
	}
	for _, src := range affected {
		rewrites[src.path] = d.rewrite(src, listed)
	}
	return rewrites, nil
}

// resolve leaves out every declaration that depends on an omitted name. It
// repeats until a pass omits no new name, since leaving out a plain function
// or a var omits its name in turn, which can make a declaration an earlier
// pass kept depend on it.
func (d *columnDependents) resolve() {
	for changed := true; changed; {
		changed = false
		for _, src := range d.files.sources {
			for _, decl := range src.file.Decls {
				switch x := decl.(type) {
				case *ast.FuncDecl:
					if x.Body == nil || d.bodies[x] || !d.references(src, x.Body) {
						continue
					}
					d.bodies[x] = true
					// A method is selected through a value, which cannot be
					// matched by name, and an init function cannot be
					// referred to at all.
					if x.Recv == nil && x.Name.Name != "init" {
						changed = d.files.omit(src.pkg, x.Name.Name) || changed
					}
				case *ast.GenDecl:
					if x.Tok != token.VAR {
						continue
					}
					for _, spec := range x.Specs {
						value, ok := spec.(*ast.ValueSpec)
						if !ok || d.vars[value] || !slices.ContainsFunc(value.Values, func(expr ast.Expr) bool {
							return d.references(src, expr)
						}) {
							continue
						}
						d.vars[value] = true
						for _, name := range value.Names {
							changed = d.files.omit(src.pkg, name.Name) || changed
						}
					}
				}
			}
		}
	}
}

// references reports whether node mentions an omitted name: one of src's own
// package, one of a dot-imported model package, or one selected from an
// imported model package.
func (d *columnDependents) references(src *columnInspectionSource, node ast.Node) bool {
	imports := d.imports[src]
	omitted := d.files.omitted
	found := false
	var visit func(ast.Node) bool
	visit = func(n ast.Node) bool {
		if found {
			return false
		}
		switch x := n.(type) {
		case *ast.SelectorExpr:
			if operand, ok := x.X.(*ast.Ident); ok {
				for _, pkg := range imports.qualifiers[operand.Name] {
					if omitted[pkg][x.Sel.Name] {
						found = true
						return false
					}
				}
			}
			// The selected name is a field, a method or a member of another
			// package: only the operand can mention a name of this package.
			ast.Inspect(x.X, visit)
			return false
		case *ast.Ident:
			if omitted[src.pkg][x.Name] || slices.ContainsFunc(imports.dotted, func(pkg string) bool {
				return omitted[pkg][x.Name]
			}) {
				found = true
				return false
			}
		}
		return true
	}
	ast.Inspect(node, visit)
	return found
}

// leavesOutAnyOf reports whether src declares a left-out function body or
// var.
func (d *columnDependents) leavesOutAnyOf(src *columnInspectionSource) bool {
	for _, decl := range src.file.Decls {
		switch x := decl.(type) {
		case *ast.FuncDecl:
			if d.bodies[x] {
				return true
			}
		case *ast.GenDecl:
			for _, spec := range x.Specs {
				if value, ok := spec.(*ast.ValueSpec); ok && d.vars[value] {
					return true
				}
			}
		}
	}
	return false
}

// unknownImports returns the import paths of the affected files whose package
// name, or for a dot import whose exported names, the scan does not know.
func (d *columnDependents) unknownImports(affected []*columnInspectionSource) []string {
	var paths []string
	for _, src := range affected {
		for _, spec := range src.file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil || path == "C" || slices.Contains(paths, path) {
				continue
			}
			names, isModelPackage := d.files.packageNames[path]
			switch {
			case spec.Name == nil && len(names) != 1:
				paths = append(paths, path)
			case spec.Name != nil && spec.Name.Name == "." && !isModelPackage:
				paths = append(paths, path)
			}
		}
	}
	slices.Sort(paths)
	return paths
}

// rewrite returns the content src compiles with once its left-out
// declarations are gone.
func (d *columnDependents) rewrite(src *columnInspectionSource, listed map[string]listedPackage) string {
	tokenFile := d.files.fset.File(src.file.Package)
	var edits []sourceEdit
	for _, decl := range src.file.Decls {
		switch x := decl.(type) {
		case *ast.FuncDecl:
			if !d.bodies[x] {
				continue
			}
			start, end := tokenFile.Offset(x.Body.Lbrace), tokenFile.Offset(x.Body.Rbrace)+1
			body := columnInspectionPlaceholder
			if x.Recv == nil && x.Name.Name == "init" {
				body = ""
			}
			edits = append(edits, sourceEdit{start: start, end: end, text: "{" + body + lineBreaks(src.content[start:end]) + "}"})
		case *ast.GenDecl:
			for _, spec := range x.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok || !d.vars[value] {
					continue
				}
				// An ungrouped declaration holds this one spec, and removing
				// the spec alone would leave a bare var keyword behind.
				if !x.Lparen.IsValid() {
					edits = append(edits, removalEdit(src.content, tokenFile.Offset(x.Pos()), tokenFile.Offset(x.End())))
					continue
				}
				edits = append(edits, removalEdit(src.content, tokenFile.Offset(value.Pos()), tokenFile.Offset(value.End())))
			}
		}
	}

	remaining := d.remainingNamesOf(src)
	for _, spec := range src.file.Imports {
		if d.importUsed(spec, remaining, listed) {
			continue
		}
		if spec.Name == nil {
			at := tokenFile.Offset(spec.Path.Pos())
			edits = append(edits, sourceEdit{start: at, end: at, text: "_ "})
			continue
		}
		edits = append(edits, sourceEdit{start: tokenFile.Offset(spec.Name.Pos()), end: tokenFile.Offset(spec.Name.End()), text: "_"})
	}
	return applySourceEdits(src.content, edits)
}

// remainingNames is what the code a file keeps still mentions.
type remainingNames struct {
	// qualifiers are the identifiers selector expressions select from, which
	// is where a package name is used.
	qualifiers map[string]bool

	// bare are the identifiers used on their own, which is how a
	// dot-imported name is used.
	bare map[string]bool
}

// remainingNamesOf collects the names the code src keeps still mentions: every
// declaration that is not left out, and the signature of every function whose
// body is.
func (d *columnDependents) remainingNamesOf(src *columnInspectionSource) remainingNames {
	remaining := remainingNames{qualifiers: make(map[string]bool), bare: make(map[string]bool)}
	var visit func(ast.Node) bool
	visit = func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			if operand, ok := x.X.(*ast.Ident); ok {
				remaining.qualifiers[operand.Name] = true
			}
			ast.Inspect(x.X, visit)
			return false
		case *ast.Ident:
			remaining.bare[x.Name] = true
		}
		return true
	}
	for _, decl := range src.file.Decls {
		switch x := decl.(type) {
		case *ast.FuncDecl:
			if !d.bodies[x] {
				ast.Inspect(x, visit)
				continue
			}
			if x.Recv != nil {
				ast.Inspect(x.Recv, visit)
			}
			ast.Inspect(x.Type, visit)
		case *ast.GenDecl:
			if x.Tok == token.IMPORT {
				continue
			}
			for _, spec := range x.Specs {
				if value, ok := spec.(*ast.ValueSpec); ok && d.vars[value] {
					continue
				}
				ast.Inspect(spec, visit)
			}
		}
	}
	return remaining
}

// importUsed reports whether the code a file keeps still uses an import. An
// import whose names cannot be resolved counts as used, leaving the compiler
// to judge it.
func (d *columnDependents) importUsed(spec *ast.ImportSpec, remaining remainingNames, listed map[string]listedPackage) bool {
	path, err := strconv.Unquote(spec.Path.Value)
	if err != nil || path == "C" {
		return true
	}
	switch {
	case spec.Name == nil:
		name := d.packageName(path, listed)
		return name == "" || remaining.qualifiers[name]
	case spec.Name.Name == "_":
		return true
	case spec.Name.Name == ".":
		exported := d.exportedNames(path, listed)
		if exported == nil {
			return true
		}
		for name := range remaining.bare {
			if exported[name] {
				return true
			}
		}
		return false
	default:
		return remaining.qualifiers[spec.Name.Name]
	}
}

// packageName returns the name a plain import of path is referred to by, or
// "" when it cannot be resolved.
func (d *columnDependents) packageName(path string, listed map[string]listedPackage) string {
	if names := d.files.packageNames[path]; len(names) == 1 {
		return names[0]
	}
	return listed[path].Name
}

// exportedNames returns the names a dot import of path brings into scope, or
// nil when they cannot be read.
func (d *columnDependents) exportedNames(path string, listed map[string]listedPackage) map[string]bool {
	if exported, cached := d.exports[path]; cached {
		return exported
	}
	var exported map[string]bool
	if files, ok := d.packageFiles(path, listed); ok {
		exported = make(map[string]bool)
		for _, file := range files {
			for _, name := range packageLevelNames(file) {
				if ast.IsExported(name) {
					exported[name] = true
				}
			}
		}
	}
	d.exports[path] = exported
	return exported
}

// packageFiles returns the parsed files of the package at path: the scanned
// sources of a model package, or the files the go command selected for any
// other package.
func (d *columnDependents) packageFiles(path string, listed map[string]listedPackage) ([]*ast.File, bool) {
	if _, isModelPackage := d.files.packageNames[path]; isModelPackage {
		var files []*ast.File
		for _, src := range d.files.sources {
			if src.pkg == path {
				files = append(files, src.file)
			}
		}
		return files, true
	}
	pkg, ok := listed[path]
	if !ok || pkg.Dir == "" {
		return nil, false
	}
	names := slices.Concat(pkg.GoFiles, pkg.CgoFiles)
	files := make([]*ast.File, 0, len(names))
	for _, name := range names {
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(pkg.Dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, false
		}
		files = append(files, file)
	}
	return files, true
}

// packageLevelNames returns the names a file declares at package level:
// functions, types, vars and constants, but not methods.
func packageLevelNames(file *ast.File) []string {
	var names []string
	for _, decl := range file.Decls {
		switch x := decl.(type) {
		case *ast.FuncDecl:
			if x.Recv == nil {
				names = append(names, x.Name.Name)
			}
		case *ast.GenDecl:
			for _, spec := range x.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					names = append(names, s.Name.Name)
				case *ast.ValueSpec:
					for _, name := range s.Names {
						names = append(names, name.Name)
					}
				}
			}
		}
	}
	return names
}

// sourceEdit replaces content[start:end] with text.
type sourceEdit struct {
	start int
	end   int
	text  string
}

// applySourceEdits returns content with non-overlapping edits applied.
func applySourceEdits(content []byte, edits []sourceEdit) string {
	slices.SortFunc(edits, func(a, b sourceEdit) int { return cmp.Compare(a.start, b.start) })
	var out strings.Builder
	offset := 0
	for _, edit := range edits {
		out.Write(content[offset:edit.start])
		out.WriteString(edit.text)
		offset = edit.end
	}
	out.Write(content[offset:])
	return out.String()
}

// removalEdit blanks content[start:end] out, together with a semicolon that
// directly follows on the same line: a declaration or spec separated by an
// explicit semicolon would otherwise leave an empty one behind, which does not
// parse.
func removalEdit(content []byte, start int, end int) sourceEdit {
	rest := content[end:]
	if trimmed := bytes.TrimLeft(rest, " \t"); len(trimmed) > 0 && trimmed[0] == ';' {
		end += len(rest) - len(trimmed) + 1
	}
	return sourceEdit{start: start, end: end, text: lineBreaks(content[start:end])}
}

// lineBreaks returns as many line breaks as text holds, which is what a
// replacement needs to keep every later line where it was.
func lineBreaks(text []byte) string {
	return strings.Repeat("\n", bytes.Count(text, []byte("\n")))
}
