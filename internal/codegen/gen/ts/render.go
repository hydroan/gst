package ts

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"go/token"
	"go/types"
	"maps"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/hydroan/gst/consts"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
)

// generator walks the types reachable from the roots and renders their
// declarations.
type generator struct {
	cfg        Config
	dir        string // absolute Config.Dir
	fset       *token.FileSet
	pkgs       map[string]*packages.Package
	sources    map[string]*sourceIndex
	methodSets typeutil.MethodSetCache
	// preludeFile is the file the framework prelude goes to, named after the
	// application.
	preludeFile string

	// decls holds an entry for every project type queued so far; the entry
	// stays nil until the type is rendered.
	decls map[*types.TypeName]*declaration
	queue []*types.TypeName
	enums map[*types.TypeName]*enumType
	// inlining holds the types from outside the project being spelled out,
	// to stop at one that refers to itself.
	inlining map[string]bool

	diags    []Diagnostic
	reported map[string]bool
}

// declaration is the rendered TypeScript declaration of one project type.
type declaration struct {
	obj     *types.TypeName
	text    string
	imports map[string]bool // paths of the packages the text refers to
}

// site locates the subject of a diagnostic.
type site struct {
	subject string
	pos     token.Pos
}

// fileContext collects the imports of the file a declaration goes to.
type fileContext struct {
	pkgPath string
	imports map[string]bool
}

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

// render renders the declaration of a project type.
func (g *generator) render(obj *types.TypeName) *declaration {
	ctx := &fileContext{pkgPath: obj.Pkg().Path(), imports: make(map[string]bool)}
	s := site{subject: obj.Pkg().Path() + "." + obj.Name(), pos: obj.Pos()}
	d := &declaration{obj: obj, imports: ctx.imports}
	if reservedTypeNames[obj.Name()] {
		g.report(s, "%s is reserved in TypeScript and cannot name a type; rename the Go type", obj.Name())
	}
	doc := g.sources[ctx.pkgPath].typeDocs[obj]

	var b strings.Builder
	switch t := obj.Type().(type) {
	case *types.Alias:
		if t.TypeParams().Len() > 0 {
			g.report(s, "generic type aliases are not supported; declare an alias for each instantiation the API uses")
			return d
		}
		body := g.expr(t.Rhs(), ctx, s)
		// An alias of a slice, map or pointer holds nil like a named type of
		// one does; an alias of a declared project type leaves that to the
		// declaration it refers to.
		if nullable(t.Rhs()) && !g.refersToDeclaration(t.Rhs()) {
			body += " | null"
		}
		writeDoc(&b, "", doc)
		fmt.Fprintf(&b, "export type %s = %s;\n", obj.Name(), body)
	case *types.Named:
		if t.TypeParams().Len() > 0 {
			g.report(s, "generic types of the project are not supported; declare a non-generic type for each instantiation the API uses")
			return d
		}
		if method := g.method(t, marshalMethods); method != "" {
			g.report(s, "the type declares %s, so its JSON shape is decided by code the generator cannot read; drop the method or use a type without one", method)
			return d
		}
		switch u := t.Underlying().(type) {
		case *types.Struct:
			writeDoc(&b, "", doc)
			g.writeInterface(&b, obj.Name(), g.jsonFields(u, s), ctx, s)
		case *types.Basic:
			if e := g.enumOf(obj); e != nil {
				writeDoc(&b, "", enumDoc(doc, e))
				fmt.Fprintf(&b, "export type %s = %s;\n", obj.Name(), enumBody(e))
				break
			}
			writeDoc(&b, "", doc)
			fmt.Fprintf(&b, "export type %s = %s;\n", obj.Name(), g.structural(u, ctx, s))
		default:
			// A nil slice, map or pointer is a value of the type itself, which
			// encodes as null wherever the type is used.
			body := g.structural(u, ctx, s)
			if nullable(u) {
				body += " | null"
			}
			writeDoc(&b, "", doc)
			fmt.Fprintf(&b, "export type %s = %s;\n", obj.Name(), body)
		}
	}
	d.text = b.String()
	return d
}

// writeInterface writes an interface declaration with a property per key.
func (g *generator) writeInterface(b *strings.Builder, name string, fields []jsonField, ctx *fileContext, s site) {
	fmt.Fprintf(b, "export interface %s {\n", name)
	if len(fields) == 0 {
		b.WriteString("  [key: string]: never;\n")
	}
	for _, f := range fields {
		writeDoc(b, "  ", g.fieldDoc(f.field))
		fmt.Fprintf(b, "  %s;\n", g.property(f, ctx, g.fieldSite(s, f.key, f.field)))
	}
	b.WriteString("}\n")
}

// fieldDoc returns the doc comment of a project struct field.
func (g *generator) fieldDoc(v *types.Var) string {
	if v.Pkg() == nil {
		return ""
	}
	if source := g.sources[v.Pkg().Path()]; source != nil {
		return source.fieldDocs[v]
	}
	return ""
}

// fieldSite locates a field of the type at s. The field's own position is
// used when the project declares it; a field of a type from outside the
// project is reported where the project uses that type.
func (g *generator) fieldSite(s site, name string, f *types.Var) site {
	pos := s.pos
	if f.Pkg() != nil && g.sources[f.Pkg().Path()] != nil {
		pos = f.Pos()
	}
	return site{subject: s.subject + "." + name, pos: pos}
}

// property renders the property of one key. It is optional when the key may
// be absent -- an omitempty or omitzero option, a nil-able type, promotion
// through an embedded pointer -- and nullable when a value of the field's type
// may encode as null.
func (g *generator) property(f jsonField, ctx *fileContext, s site) string {
	t := f.field.Type()
	var value string
	if f.quoted {
		value = "string"
	} else {
		value = g.expr(t, ctx, s)
		// An omitted zero value never reaches the client, unless the field is a
		// pointer: a non-nil pointer to the zero value is not omitted.
		_, isPointer := types.Unalias(t).Underlying().(*types.Pointer)
		if zero := g.enumZero(t); zero != "" && (!f.omit || isPointer) {
			value += " | " + zero
		}
	}
	if nullable(t) {
		value += " | null"
	}
	optional := ""
	if f.omit || f.viaPointer || nilable(t) {
		optional = "?"
	}
	return propertyName(f.key) + optional + ": " + value
}

// expr renders the TypeScript type of a value of t that is not null. Whether
// null or the zero value of an enum may stand in for the value is decided where
// the value is used.
func (g *generator) expr(t types.Type, ctx *fileContext, s site) string {
	switch tt := t.(type) {
	case *types.Alias:
		if g.declares(tt.Obj()) {
			if tt.TypeArgs().Len() > 0 {
				g.report(s, "the generic alias %s is not supported; declare an alias for each instantiation the API uses", tt)
				return "unknown"
			}
			return g.ref(tt.Obj(), ctx)
		}
		return g.expr(types.Unalias(tt), ctx, s)
	case *types.Named:
		if kind, ok := builtinOf(tt); ok {
			return g.builtin(kind, tt, ctx, s)
		}
		if g.declares(tt.Obj()) {
			if tt.TypeArgs().Len() > 0 {
				g.report(s, "the generic type %s of the project is not supported; declare a non-generic type for each instantiation the API uses", tt)
				return "unknown"
			}
			return g.ref(tt.Obj(), ctx)
		}
		if method := g.method(tt, marshalMethods); method != "" {
			g.report(s, "type %s declares %s, so its JSON shape is decided by code the generator cannot read; use a type without the method", tt, method)
			return "unknown"
		}
		if _, isStruct := tt.Underlying().(*types.Struct); isStruct {
			key := types.TypeString(tt, nil)
			if g.inlining[key] {
				g.report(s, "type %s refers to itself and is declared outside the project, so it cannot be spelled out inline; use a project type instead", tt)
				return "unknown"
			}
			g.inlining[key] = true
			defer delete(g.inlining, key)
		}
		return g.structural(tt.Underlying(), ctx, s)
	default:
		return g.structural(t, ctx, s)
	}
}

// refersToDeclaration reports whether t renders as a reference to the
// declaration of a project type rather than being spelled out.
func (g *generator) refersToDeclaration(t types.Type) bool {
	switch tt := t.(type) {
	case *types.Alias:
		return g.declares(tt.Obj())
	case *types.Named:
		_, isBuiltin := builtinOf(tt)
		return !isBuiltin && g.declares(tt.Obj())
	default:
		return false
	}
}

// ref queues the declaration of a project type and renders a reference to it
// from the file of ctx.
func (g *generator) ref(obj *types.TypeName, ctx *fileContext) string {
	g.enqueue(obj)
	if obj.Pkg().Path() == ctx.pkgPath {
		return obj.Name()
	}
	ctx.imports[obj.Pkg().Path()] = true
	return g.importAlias(obj.Pkg().Path()) + "." + obj.Name()
}

// builtin renders a type of the builtin table.
func (g *generator) builtin(kind builtinKind, n *types.Named, ctx *fileContext, s site) string {
	switch kind {
	case builtinString, builtinNullableString:
		return "string"
	case builtinNumber:
		return "number"
	case builtinObject:
		return "{ [key: string]: unknown }"
	case builtinWrapper:
		arg := n.TypeArgs().At(0)
		value := g.expr(arg, ctx, s)
		if zero := g.enumZero(arg); zero != "" {
			value += " | " + zero
		}
		return value
	default:
		return "unknown"
	}
}

// structural renders a type by its structure.
func (g *generator) structural(t types.Type, ctx *fileContext, s site) string {
	switch u := t.(type) {
	case *types.Basic:
		info := u.Info()
		switch {
		case info&types.IsBoolean != 0:
			return "boolean"
		case info&types.IsString != 0:
			return "string"
		case info&(types.IsInteger|types.IsFloat) != 0:
			return "number"
		}
		g.report(s, "%s values have no JSON encoding", u)
		return "unknown"
	case *types.Pointer:
		return g.expr(u.Elem(), ctx, s)
	case *types.Slice:
		if g.isByteSlice(u) {
			return "string"
		}
		return arrayOf(g.value(u.Elem(), ctx, s))
	case *types.Array:
		return arrayOf(g.value(u.Elem(), ctx, s))
	case *types.Map:
		g.checkMapKey(u.Key(), s)
		return "{ [key: string]: " + g.value(u.Elem(), ctx, s) + " }"
	case *types.Struct:
		fields := g.jsonFields(u, s)
		if len(fields) == 0 {
			return "{ [key: string]: never }"
		}
		properties := make([]string, len(fields))
		for i, f := range fields {
			properties[i] = g.property(f, ctx, g.fieldSite(s, f.key, f.field))
		}
		return "{ " + strings.Join(properties, "; ") + " }"
	case *types.Interface:
		if u.Empty() {
			return "unknown"
		}
		g.report(s, "an interface with methods has no JSON shape of its own, the dynamic type decides it; use a concrete type")
		return "unknown"
	case *types.Named, *types.Alias:
		return g.expr(u, ctx, s)
	default:
		g.report(s, "%s values have no JSON encoding", t)
		return "unknown"
	}
}

// value renders a value position: the type, the zero value of an enum no
// constant covers, and null when a value of t may encode as null.
func (g *generator) value(t types.Type, ctx *fileContext, s site) string {
	value := g.expr(t, ctx, s)
	if zero := g.enumZero(t); zero != "" {
		value += " | " + zero
	}
	if nullable(t) {
		value += " | null"
	}
	return value
}

// enumZero returns the zero value literal of the project enum t holds,
// directly or through a pointer, when no constant of the enum covers it.
func (g *generator) enumZero(t types.Type) string {
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		t = types.Unalias(p.Elem())
	}
	n, ok := t.(*types.Named)
	if !ok || !g.declares(n.Obj()) {
		return ""
	}
	if e := g.enumOf(n.Obj()); e != nil && !e.bitwise {
		return e.zero
	}
	return ""
}

// isByteSlice reports whether encoding/json encodes s as a base64 string: a
// slice of bytes whose element type has no marshal methods.
func (g *generator) isByteSlice(s *types.Slice) bool {
	elem := types.Unalias(s.Elem())
	b, ok := elem.Underlying().(*types.Basic)
	return ok && b.Kind() == types.Uint8 && g.method(elem, marshalMethods) == ""
}

// checkMapKey reports a map key type without a stable JSON encoding. Keys of
// string and integer types encode as strings; a key type with text marshal
// methods is keyed differently with and without the JSON v2 experiment.
func (g *generator) checkMapKey(key types.Type, s site) {
	key = types.Unalias(key)
	if method := g.method(key, keyMarshalMethods); method != "" {
		g.report(s, "map key type %s declares %s, which encoding/json and the JSON v2 experiment apply to keys differently; use a string or integer key type without it", key, method)
		return
	}
	if b, ok := key.Underlying().(*types.Basic); ok && b.Info()&(types.IsString|types.IsInteger) != 0 {
		return
	}
	g.report(s, "map key type %s has no JSON encoding; use a string or integer key type", key)
}

// arrayOf renders an array of elem, parenthesizing a union.
func arrayOf(elem string) string {
	if strings.Contains(elem, " | ") {
		return "(" + elem + ")[]"
	}
	return elem + "[]"
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

// nonFileName matches a character the prelude file name cannot hold.
var nonFileName = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// preludeFileName returns the file the prelude goes to: the application name,
// or the framework name when the project configured none. Whatever a file name
// and an import specifier cannot hold is replaced, so every configured name
// yields a file a frontend can copy and import.
func preludeFileName(appName string) string {
	return cmp.Or(strings.Trim(nonFileName.ReplaceAllString(appName, "_"), "_-"), consts.FrameworkName) + ".ts"
}

// files assembles the output: the prelude, and a file per package with
// declarations, which appear in source order.
func (g *generator) files() []File {
	// The output mirrors the models: a project whose routes exchange no type
	// has nothing to describe, and the prelude alone would describe nothing.
	if len(g.cfg.Roots) == 0 {
		return nil
	}

	byPackage := make(map[string][]*declaration)
	for obj, d := range g.decls {
		if d != nil && d.text != "" {
			byPackage[obj.Pkg().Path()] = append(byPackage[obj.Pkg().Path()], d)
		}
	}

	files := []File{{Path: g.preludeFile, Content: prelude()}}
	owners := map[string]string{g.preludeFile: "the framework prelude"}
	aliases := make(map[string]string)
	for _, pkgPath := range slices.Sorted(maps.Keys(byPackage)) {
		file := g.filePath(pkgPath)
		if owner, taken := owners[file]; taken {
			g.report(site{subject: pkgPath}, "the package would be written to %s, which %s takes already; rename the package", file, owner)
			continue
		}
		owners[file] = pkgPath

		decls := byPackage[pkgPath]
		slices.SortFunc(decls, func(a, b *declaration) int {
			return comparePositions(g.fset, a.obj.Pos(), b.obj.Pos())
		})
		imports := make(map[string]bool)
		for _, d := range decls {
			maps.Copy(imports, d.imports)
		}

		var b strings.Builder
		b.WriteString(consts.CodeGeneratedComment())
		b.WriteString("\n")
		if len(imports) > 0 {
			b.WriteString("\n")
		}
		for _, imported := range slices.Sorted(maps.Keys(imports)) {
			alias := g.importAlias(imported)
			if other, clash := aliases[alias]; clash && other != imported {
				g.report(site{subject: imported}, "the package and %s are both imported as %s; rename one of them", other, alias)
			}
			aliases[alias] = imported
			fmt.Fprintf(&b, "import type * as %s from %s;\n", alias, quoteString(relativeImport(file, g.filePath(imported))))
		}
		for _, d := range decls {
			b.WriteString("\n")
			b.WriteString(d.text)
		}
		files = append(files, File{Path: file, Content: b.String()})
	}
	slices.SortFunc(files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	return files
}

// relativePackage returns the output path of a package, extension aside. The
// packages of the root tree sit at the output root, so the model directory is
// not repeated in every path; the root package itself keeps that directory's
// name. A package outside the tree keeps its path relative to the module, which
// is what tells a reader it comes from elsewhere.
func (g *generator) relativePackage(pkgPath string) string {
	if root := g.cfg.RootPath; root != "" {
		if pkgPath == root {
			return path.Base(root)
		}
		if rel, inRoot := strings.CutPrefix(pkgPath, root+"/"); inRoot {
			return rel
		}
	}
	return cmp.Or(strings.TrimPrefix(strings.TrimPrefix(pkgPath, g.cfg.ModulePath), "/"), "index")
}

// filePath returns the output file of a package.
func (g *generator) filePath(pkgPath string) string {
	return g.relativePackage(pkgPath) + ".ts"
}

// nonIdentifier matches a character an import name cannot hold.
var nonIdentifier = regexp.MustCompile(`[^A-Za-z0-9_]`)

// importAlias names the namespace a file imports a package under: the relative
// package path with its segments joined by $. Go identifiers never hold $, so
// the name cannot collide with a declaration.
func (g *generator) importAlias(pkgPath string) string {
	segments := strings.Split(g.relativePackage(pkgPath), "/")
	for i, segment := range segments {
		segments[i] = nonIdentifier.ReplaceAllString(segment, "_")
	}
	return "$" + strings.Join(segments, "$")
}

// relativeImport returns the specifier the file at from imports the file at to
// with. The .js extension resolves under every TypeScript module resolution
// mode, including the node modes, which require an extension.
func relativeImport(from, to string) string {
	var fromDirs []string
	if dir := path.Dir(from); dir != "." {
		fromDirs = strings.Split(dir, "/")
	}
	toParts := strings.Split(strings.TrimSuffix(to, ".ts"), "/")
	common := 0
	for common < len(fromDirs) && common < len(toParts)-1 && fromDirs[common] == toParts[common] {
		common++
	}
	parts := make([]string, 0, len(fromDirs)+len(toParts))
	for range fromDirs[common:] {
		parts = append(parts, "..")
	}
	parts = append(parts, toParts[common:]...)
	specifier := strings.Join(parts, "/") + ".js"
	if !strings.HasPrefix(specifier, "../") {
		specifier = "./" + specifier
	}
	return specifier
}

// writeDoc writes doc as a JSDoc comment. A "*/" would end the comment early
// and a line starting with @ would read as a JSDoc tag, so both are escaped.
func writeDoc(b *strings.Builder, indent, doc string) {
	if doc == "" {
		return
	}
	lines := strings.Split(strings.ReplaceAll(doc, "*/", `*\/`), "\n")
	for i, line := range lines {
		if trimmed := strings.TrimLeft(line, " \t"); strings.HasPrefix(trimmed, "@") {
			lines[i] = line[:len(line)-len(trimmed)] + `\` + trimmed
		}
	}
	if len(lines) == 1 {
		fmt.Fprintf(b, "%s/** %s */\n", indent, lines[0])
		return
	}
	fmt.Fprintf(b, "%s/**\n", indent)
	for _, line := range lines {
		if line == "" {
			fmt.Fprintf(b, "%s *\n", indent)
			continue
		}
		fmt.Fprintf(b, "%s * %s\n", indent, line)
	}
	fmt.Fprintf(b, "%s */\n", indent)
}

// identifier matches a property name TypeScript accepts unquoted.
var identifier = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

// propertyName renders a JSON key as a property name, quoted unless it is a
// plain identifier.
func propertyName(key string) string {
	if identifier.MatchString(key) {
		return key
	}
	return quoteString(key)
}

// quoteString renders s as a TypeScript string literal. JSON string syntax is
// valid TypeScript; HTML escaping is turned off to keep the literal readable.
func quoteString(s string) string {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(s); err != nil {
		return strconv.Quote(s)
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

// reservedTypeNames are the names TypeScript refuses for a type: its reserved
// words and predefined type names. Go allows some of them as unexported type
// names.
var reservedTypeNames = map[string]bool{
	"any": true, "bigint": true, "boolean": true, "break": true, "case": true, "catch": true,
	"class": true, "const": true, "continue": true, "debugger": true, "default": true,
	"delete": true, "do": true, "else": true, "enum": true, "export": true, "extends": true,
	"false": true, "finally": true, "for": true, "function": true, "if": true,
	"implements": true, "import": true, "in": true, "instanceof": true, "interface": true,
	"let": true, "never": true, "new": true, "null": true, "number": true, "object": true,
	"package": true, "private": true, "protected": true, "public": true, "return": true,
	"static": true, "string": true, "super": true, "switch": true, "symbol": true,
	"this": true, "throw": true, "true": true, "try": true, "typeof": true,
	"undefined": true, "unknown": true, "var": true, "void": true, "while": true,
	"with": true, "yield": true,
}

// prelude declares the JSON the framework wraps around the project's types:
// the response envelope written by internal/response, and the list data and
// batch request bodies of the default actions in internal/controller.
func prelude() string {
	return consts.CodeGeneratedComment() + `

/**
 * Envelope is the JSON body of every API response. A successful request
 * carries code 0 and the data of the route; a failed one carries another code,
 * the error message and data null.
 */
export interface Envelope<T> {
  code: number;
  data: T;
  msg: string;
  trace_id: string;
}

/** ListResult is the data of the default List action. */
export interface ListResult<T> {
  items: T[];
  total: number;
}

/** ItemsPayload is the request body of the default batch create, update and patch actions. */
export interface ItemsPayload<T> {
  items: T[];
}

/** IDsPayload is the request body of the default batch delete action. */
export interface IDsPayload {
  ids: string[];
}
`
}
