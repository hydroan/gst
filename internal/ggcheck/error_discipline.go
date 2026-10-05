package ggcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/goast"
	"github.com/hydroan/gst/internal/modelregistry"
)

// ErrorDiscipline requires the errors leaving service methods and
// model hooks to be built by gst.NewError or gst.NewErrorWithCause.
var ErrorDiscipline = Check{
	Name: "Error discipline",
	Rule: "errors leaving service methods and model hooks must be built by gst.NewError or gst.NewErrorWithCause",
	run:  checkErrorDiscipline,
}

// hookMethods are the lifecycle hooks of a model by method name, the entry
// points of a model type (see collectModelTypes).
var hookMethods = func() map[string]bool {
	set := map[string]bool{}
	for _, name := range modelregistry.HookMethodNames() {
		set[name] = true
	}
	return set
}()

// checkErrorDiscipline checks that every error a service method or a
// model's lifecycle hook can return is created by gst.NewError or
// gst.NewErrorWithCause, either directly at the exit or inside a project
// function the exit's error flows from. An error built any other way is
// answered as the server's own failure, 500 with the generic message,
// instead of the status and message the refusal meant for the client.
//
// The analysis is purely syntactic, mirroring the other project checks. It
// summarizes, per project function whose last result is error, where the
// returned error values come from, then walks the flow from every service
// method (a method on a struct embedding service.Base) and every lifecycle
// hook of a model (a struct embedding model.Base or model.AutoBase) and
// reports each raw source it can reach: framework and third-party calls returned as-is, raw
// cockroachdb constructors, and identifiers whose origin cannot be resolved.
// A model hook may return the error of a framework database call or
// sentinel as it is: the framework answers it by its own mapping, 404 for a
// record that does not exist, 409 for a duplicate, a stale version or a
// foreign key, 400 for a value the table refuses or a missing version or
// id, and 500 for the rest, so a hook that reads or writes records
// before admitting a write is compliant without wrapping; on a service exit
// the same error is raw, since a service answers whatever it returns.
// database.Transaction calls are transparent: their closure exits are
// treated as exits of the enclosing flow. Unresolvable constructs fail
// closed, so an exit the checker cannot prove compliant is a violation.
//
// A method call x.M() is resolved by what x is, and a name the function
// declares itself — its receiver, a parameter, a local variable — comes
// before any package-level meaning of the same name, exactly as the Go
// compiler reads it:
//
//	var mgr = manager{} // package-level
//
//	func (g *Getter) Get(ctx *gst.ServiceContext, req *model.RecordReq) (*model.RecordRsp, error) {
//		mgr := other{}       // shadows the package-level mgr
//		return nil, mgr.Do() // other.Do, not manager.Do
//	}
//
// A local variable resolves to the type its declaration spells out: x :=
// T{}, x := &T{}, var x T, var x = T{}, or a parameter x T. A declaration
// that does not spell the type out, such as x := newT(), a range variable or
// a type switch variable, fails closed at the call, and so does a type of
// another package or an interface, whose method bodies the checker cannot
// see.
func checkErrorDiscipline(ignore gghelper.ProjectIgnore) []string {
	modulePath, err := gghelper.ModulePath()
	if err != nil {
		return []string{fmt.Sprintf("reading the module path: %v", err)}
	}
	analysis, err := collectProject(modulePath, ignore)
	if err != nil {
		return []string{fmt.Sprintf("walking project directory: %v", err)}
	}

	// Function bodies are analyzed only after every file was collected, so
	// cross-file knowledge - service struct types and package-level variable
	// types - is complete regardless of walk order.
	for _, collector := range analysis.files {
		for _, decl := range collector.file.Decls {
			if funcDecl, ok := decl.(*ast.FuncDecl); ok {
				collector.collectFunc(funcDecl)
			}
		}
	}

	return analysis.report()
}

// errDiscFuncKey identifies a project function or method: the package
// directory, the receiver type name ("" for package-level functions), and
// the function name.
type errDiscFuncKey struct {
	pkgDir string
	recv   string
	name   string
}

// errDiscSourceKind classifies where an error exit value comes from: nil, a
// framework error constructor, a project function whose own exits decide, a
// framework database call or sentinel, or a raw source the check reports.
type errDiscSourceKind int

const (
	errDiscSourceNil errDiscSourceKind = iota
	errDiscSourceNewError
	errDiscSourceCall
	// errDiscSourceDatabase is an error of the framework database package,
	// a call rooted at it or one of its sentinels: raw on a service exit,
	// accepted on a model hook's, whose errors the framework maps itself.
	errDiscSourceDatabase
	errDiscSourceRaw
)

// errDiscSource is one origin an error exit value can flow from.
type errDiscSource struct {
	kind   errDiscSourceKind
	callee errDiscFuncKey // set for errDiscSourceCall
	pos    token.Position // set for errDiscSourceCall, errDiscSourceDatabase and errDiscSourceRaw
}

// errDiscFuncSummary aggregates the origins of every error exit of one
// function, closure exits of database.Transaction included.
type errDiscFuncSummary struct {
	sources []errDiscSource
}

// errDiscAnalysis carries the whole-project state: per-function summaries and
// the service struct types whose methods are the checked entry points.
type errDiscAnalysis struct {
	modulePath string
	fset       *token.FileSet
	summaries  map[errDiscFuncKey]*errDiscFuncSummary
	// entryTypes holds, per package directory, the service struct types
	// declared there.
	entryTypes map[string]map[string]bool
	// hookTypes holds, per package directory, the model struct types
	// declared there, whose lifecycle hooks are entry points as well.
	hookTypes map[string]map[string]bool
	// pkgVarTypes maps, per package directory, a package-level variable to
	// the type its declaration spells out, see collectPackageVars.
	pkgVarTypes map[string]map[string]string
	// entries lists the service methods the report walks the error flow
	// from, and hookEntries the model hooks, whose walk accepts the errors
	// of the framework database package.
	entries     []errDiscFuncKey
	hookEntries []errDiscFuncKey
	// files holds the collector of every parsed file, whose functions are
	// summarized once every file was collected.
	files []*errDiscFileCollector
	// packageNames caches the package clause of every project directory an
	// import names, see packageName.
	packageNames map[string]string
	// parseErrors holds, by path, the error of every file the parser
	// refused, which the analysis holds no functions of.
	parseErrors map[string]error
}

// collectProject parses every Go file of the project but the tests, under
// the ignore rules, into the analysis of the project of module path
// modulePath: the files with the service struct types and the package-level
// variable types they declare (see errDiscFileCollector), from which the
// error discipline check summarizes the error exits and the gRPC service
// context check follows the calls. The function bodies are read once every
// file is collected, so that cross-file knowledge is complete regardless of
// walk order.
func collectProject(modulePath string, ignore gghelper.ProjectIgnore) (*errDiscAnalysis, error) {
	analysis := &errDiscAnalysis{
		modulePath:   modulePath,
		fset:         token.NewFileSet(),
		summaries:    map[errDiscFuncKey]*errDiscFuncSummary{},
		entryTypes:   map[string]map[string]bool{},
		hookTypes:    map[string]map[string]bool{},
		pkgVarTypes:  map[string]map[string]string{},
		packageNames: map[string]string{},
		parseErrors:  map[string]error{},
	}
	err := ignore.Walk(".", func(path string, info os.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		analysis.collectFile(path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return analysis, nil
}

// packageName returns the package clause of the project directory dir, the
// name an import of it without an alias goes by. It reads the directory once
// per analysis, however many files import it.
func (a *errDiscAnalysis) packageName(dir string) string {
	if name, ok := a.packageNames[dir]; ok {
		return name
	}
	name := packageNameOf(dir)
	a.packageNames[dir] = name
	return name
}

// collectFile parses one project file and records service struct types and
// package-level variable types; a file the parser refuses is recorded by
// its error.
func (a *errDiscAnalysis) collectFile(path string) {
	file, err := parser.ParseFile(a.fset, path, nil, 0)
	if err != nil {
		a.parseErrors[filepath.ToSlash(path)] = err
		return
	}

	collector := &errDiscFileCollector{
		analysis:   a,
		path:       filepath.ToSlash(path),
		pkgDir:     filepath.ToSlash(filepath.Dir(path)),
		db:         goast.ImportedNames(file, gstDatabaseImportPath, "database"),
		gst:        goast.ImportedNames(file, gstImportPath, "gst"),
		model:      goast.ImportedNames(file, ggconst.ImportPathModel, ggconst.PkgModel),
		projectPkg: map[string]string{},
	}
	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}
		importPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		var dir string
		switch {
		case importPath == a.modulePath:
			dir = "."
		case strings.HasPrefix(importPath, a.modulePath+"/"):
			dir = strings.TrimPrefix(importPath, a.modulePath+"/")
		default:
			continue
		}
		// A dot-imported project package names nothing here: its calls
		// read as same-package calls and fail closed.
		for _, name := range goast.ImportedNames(file, importPath, a.packageName(dir)).Qualifiers {
			collector.projectPkg[name] = dir
		}
	}

	collector.file = file
	for _, decl := range file.Decls {
		if decl, ok := decl.(*ast.GenDecl); ok {
			collector.collectServiceTypes(decl)
			collector.collectModelTypes(decl)
			collector.collectPackageVars(decl)
		}
	}
	a.files = append(a.files, collector)
}

// errDiscFileCollector is the per-file context: the parsed file plus the names
// it knows the framework packages and the project's own packages by.
type errDiscFileCollector struct {
	analysis *errDiscAnalysis
	file     *ast.File
	path     string
	pkgDir   string
	// serviceTypes are the service struct types the file declares, the
	// ones entryTypes records for its package.
	serviceTypes []string
	// db, gst and model are the names of the framework database, root and
	// model packages.
	db    goast.PackageNames
	gst   goast.PackageNames
	model goast.PackageNames
	// projectPkg maps every qualifier of a project package to its directory.
	projectPkg map[string]string
}

// collectPackageVars records the concrete type of package-level variables
// declared as composite literals or with an explicit type, so method calls on
// them (for example a package-level manager singleton) resolve to that type's
// methods instead of failing closed.
func (c *errDiscFileCollector) collectPackageVars(decl *ast.GenDecl) {
	if decl.Tok != token.VAR {
		return
	}
	for _, spec := range decl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, name := range valueSpec.Names {
			typeName := ""
			switch {
			case valueSpec.Type != nil:
				typeName = errDiscReceiverTypeName(valueSpec.Type)
			case i < len(valueSpec.Values):
				typeName = errDiscCompositeTypeName(valueSpec.Values[i])
			}
			if typeName == "" {
				continue
			}
			vars := c.analysis.pkgVarTypes[c.pkgDir]
			if vars == nil {
				vars = map[string]string{}
				c.analysis.pkgVarTypes[c.pkgDir] = vars
			}
			vars[name.Name] = typeName
		}
	}
}

// errDiscCompositeTypeName extracts the local type name from a composite
// literal value, with or without a leading address operator.
func errDiscCompositeTypeName(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.UnaryExpr:
		if expr.Op == token.AND {
			return errDiscCompositeTypeName(expr.X)
		}
	case *ast.CompositeLit:
		if ident, ok := expr.Type.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}

// collectServiceTypes records struct types that embed service.Base; their
// methods are the service entry points of the check.
func (c *errDiscFileCollector) collectServiceTypes(decl *ast.GenDecl) {
	for _, spec := range decl.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok || structType.Fields == nil {
			continue
		}
		for _, field := range structType.Fields.List {
			if !goast.IsServiceBase(c.file, field) {
				continue
			}
			types := c.analysis.entryTypes[c.pkgDir]
			if types == nil {
				types = map[string]bool{}
				c.analysis.entryTypes[c.pkgDir] = types
			}
			types[typeSpec.Name.Name] = true
			c.serviceTypes = append(c.serviceTypes, typeSpec.Name.Name)
		}
	}
}

// collectModelTypes records struct types that embed model.Base or
// model.AutoBase by value; their lifecycle hooks are entry points of the
// check, since the framework runs them inside its own database writes and
// answers their errors the way it answers a service method's. A virtual
// model embedding model.Empty has no hook the framework runs.
func (c *errDiscFileCollector) collectModelTypes(decl *ast.GenDecl) {
	for _, spec := range decl.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok || structType.Fields == nil || embeddedBaseName(structType, c.model) == "" {
			continue
		}
		types := c.analysis.hookTypes[c.pkgDir]
		if types == nil {
			types = map[string]bool{}
			c.analysis.hookTypes[c.pkgDir] = types
		}
		types[typeSpec.Name.Name] = true
	}
}

// collectFunc summarizes one function whose last result is error.
func (c *errDiscFileCollector) collectFunc(decl *ast.FuncDecl) {
	if decl.Body == nil || decl.Type.Results == nil || len(decl.Type.Results.List) == 0 {
		return
	}
	results := decl.Type.Results.List
	last := results[len(results)-1]
	// A function returning *gst.Error is compliant by construction: every
	// non-nil value of that type came from NewError or NewErrorWithCause.
	if c.isServiceErrorPtr(last.Type) {
		key := errDiscFuncKey{pkgDir: c.pkgDir, recv: errDiscReceiverTypeName(receiverType(decl)), name: decl.Name.Name}
		c.analysis.summaries[key] = &errDiscFuncSummary{sources: []errDiscSource{{kind: errDiscSourceNewError}}}
		return
	}
	if ident, ok := last.Type.(*ast.Ident); !ok || ident.Name != "error" {
		return
	}

	scope := c.newScope(decl)
	scope.collectAssigns(decl.Body)
	summary := &errDiscFuncSummary{}
	scope.collectExits(decl.Body, summary)

	key := errDiscFuncKey{pkgDir: c.pkgDir, recv: scope.recvType, name: decl.Name.Name}
	c.analysis.summaries[key] = summary
	if decl.Recv == nil {
		return
	}
	switch {
	case decl.Name.IsExported() && c.analysis.entryTypes[c.pkgDir][scope.recvType]:
		c.analysis.entries = append(c.analysis.entries, key)
	case hookMethods[decl.Name.Name] && c.analysis.hookTypes[c.pkgDir][scope.recvType]:
		c.analysis.hookEntries = append(c.analysis.hookEntries, key)
	}
}

// newScope returns the scope of decl, a function of the file: what the
// function declares itself, its receiver, its *gst.ServiceContext
// parameters and its results, for its calls and exits to be resolved
// against (see errDiscFuncScope).
func (c *errDiscFileCollector) newScope(decl *ast.FuncDecl) *errDiscFuncScope {
	scope := &errDiscFuncScope{file: c, decl: decl, ctxParams: serviceContextParams(decl, c.gst)}
	if decl.Type.Results != nil && len(decl.Type.Results.List) > 0 {
		results := decl.Type.Results.List
		for _, field := range results {
			n := len(field.Names)
			if n == 0 {
				n = 1
			}
			scope.numResults += n
		}
		if names := results[len(results)-1].Names; len(names) > 0 {
			scope.resultObj = declObj(names[len(names)-1])
		}
	}
	if decl.Recv != nil && len(decl.Recv.List) == 1 {
		scope.recvType = errDiscReceiverTypeName(decl.Recv.List[0].Type)
		if names := decl.Recv.List[0].Names; len(names) == 1 {
			scope.recvObj = declObj(names[0])
		}
	}
	return scope
}

// receiverType returns the receiver type expression of a method declaration,
// or nil for a plain function.
func receiverType(decl *ast.FuncDecl) ast.Expr {
	if decl.Recv == nil || len(decl.Recv.List) != 1 {
		return nil
	}
	return decl.Recv.List[0].Type
}

// isServiceErrorPtr reports whether expr denotes *gst.Error under the names
// the file knows the framework root package by.
func (c *errDiscFileCollector) isServiceErrorPtr(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	return ok && c.gst.Refers(star.X, "Error")
}

// errDiscReceiverTypeName extracts the receiver's type name, unwrapping
// pointers and type parameters.
func errDiscReceiverTypeName(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.StarExpr:
		return errDiscReceiverTypeName(expr.X)
	case *ast.IndexExpr:
		return errDiscReceiverTypeName(expr.X)
	case *ast.IndexListExpr:
		return errDiscReceiverTypeName(expr.X)
	case *ast.Ident:
		return expr.Name
	}
	return ""
}

// errDiscFuncScope carries the per-function state used to resolve where the
// error values returned by the function come from.
type errDiscFuncScope struct {
	file *errDiscFileCollector
	// decl is the function the scope summarizes; a name declared inside it
	// is the function's own, see declaresLocally.
	decl       *ast.FuncDecl
	recvObj    varObj // the named receiver, for calls of its methods
	recvType   string
	resultObj  varObj // named error result, for naked returns
	numResults int
	// ctxParams holds the function's *gst.ServiceContext parameters, whose
	// SSE method is a sanctioned error exit.
	ctxParams map[varObj]bool
	// assigns maps a declared variable to every expression assigned to it,
	// closures included. Keying by the parser-resolved declaration object
	// keeps same-named variables from different scopes apart, and each entry
	// records its position so a return only pools assignments that happened
	// before it: reusing one err variable for several sources must not let a
	// later raw assignment pollute an earlier compliant exit.
	assigns map[varObj][]errDiscAssign
	// windows lists the exclusive visibility windows per variable; a use
	// inside a window sees only the window's own assignment.
	windows map[varObj][]errDiscWindow
}

// errDiscAssign is one recorded assignment: the assigned expression, where
// the assignment happens, and where its value stops being visible. killEnd
// is set when the assignment is immediately answered by an
// `if <var> != nil { return ... }` style check: past that check the variable
// no longer holds this value, so later uses must not pool it.
type errDiscAssign struct {
	expr    ast.Expr
	pos     token.Pos
	killEnd token.Pos
}

// errDiscWindow is an exclusive visibility window: inside the body of an
// `if <var> != nil` check that the assignment at assignPos feeds, the
// variable can hold only that value, regardless of what it held before.
type errDiscWindow struct {
	assignPos token.Pos
	bodyStart token.Pos
	bodyEnd   token.Pos
}

// collectAssigns records every assignment in the function body, closures
// included, keyed by the assigned variable's declaration object.
func (s *errDiscFuncScope) collectAssigns(body *ast.BlockStmt) {
	s.assigns = map[varObj][]errDiscAssign{}
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		if len(assign.Rhs) == 1 && len(assign.Lhs) > 1 {
			// Multi-value assignment from one call: every variable pools the
			// call as origin; only the error-typed one ever reaches an exit.
			for _, lhs := range assign.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok && declObj(ident) != nil {
					s.assigns[declObj(ident)] = append(s.assigns[declObj(ident)], errDiscAssign{expr: assign.Rhs[0], pos: assign.Pos()})
				}
			}
			return true
		}
		if len(assign.Rhs) != len(assign.Lhs) {
			return true
		}
		for i, lhs := range assign.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok && declObj(ident) != nil {
				s.assigns[declObj(ident)] = append(s.assigns[declObj(ident)], errDiscAssign{expr: assign.Rhs[i], pos: assign.Pos()})
			}
		}
		return true
	})
	s.markKilledAssigns(body)
}

// markKilledAssigns finds assignments consumed by their own error check —
// `if v = f(); v != nil { return ... }` and the two-statement form
// `v = f()` followed by `if v != nil { return ... }` — and records the end
// of the check as the assignment's kill point. Past a check whose body
// always leaves (return, branch, or panic), the variable no longer carries
// that value, which is exactly how idiomatic Go reuses one err variable.
func (s *errDiscFuncScope) markKilledAssigns(body *ast.BlockStmt) {
	s.windows = map[varObj][]errDiscWindow{}
	ast.Inspect(body, func(n ast.Node) bool {
		var stmts []ast.Stmt
		switch n := n.(type) {
		case *ast.BlockStmt:
			stmts = n.List
		case *ast.CaseClause:
			stmts = n.Body
		case *ast.CommClause:
			stmts = n.Body
		default:
			return true
		}
		for i, stmt := range stmts {
			ifStmt, ok := stmt.(*ast.IfStmt)
			if !ok {
				continue
			}
			checkedObj := errDiscNilCheckedObj(ifStmt)
			if checkedObj == nil {
				continue
			}
			assign, ok := ifStmt.Init.(*ast.AssignStmt)
			if !ok && i > 0 {
				assign, ok = stmts[i-1].(*ast.AssignStmt)
			}
			if !ok || assign == nil {
				continue
			}
			// Inside the check's body the variable holds only the value this
			// assignment just gave it, no matter what it held before.
			s.windows[checkedObj] = append(s.windows[checkedObj], errDiscWindow{
				assignPos: assign.Pos(),
				bodyStart: ifStmt.Body.Pos(),
				bodyEnd:   ifStmt.Body.End(),
			})
			// A check whose body always leaves also consumes the value for
			// everything after the check.
			if errDiscStmtsAlwaysLeave(ifStmt.Body.List) {
				s.killAssign(checkedObj, assign, ifStmt.End())
			}
		}
		return true
	})
}

// errDiscNilCheckedObj returns the declaration object of v when cond is a
// plain `v != nil` comparison, nil otherwise.
func errDiscNilCheckedObj(ifStmt *ast.IfStmt) varObj {
	cond, ok := ifStmt.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.NEQ {
		return nil
	}
	ident, ok := cond.X.(*ast.Ident)
	if !ok {
		return nil
	}
	if right, ok := cond.Y.(*ast.Ident); !ok || right.Name != "nil" {
		return nil
	}
	return declObj(ident)
}

// errDiscStmtsAlwaysLeave reports whether a statement list ends by leaving
// the surrounding flow: a return, a branch statement, or a panic call.
func errDiscStmtsAlwaysLeave(stmts []ast.Stmt) bool {
	if len(stmts) == 0 {
		return false
	}
	switch last := stmts[len(stmts)-1].(type) {
	case *ast.ReturnStmt, *ast.BranchStmt:
		return true
	case *ast.ExprStmt:
		call, ok := last.X.(*ast.CallExpr)
		if !ok {
			return false
		}
		ident, ok := call.Fun.(*ast.Ident)
		return ok && ident.Name == "panic"
	}
	return false
}

// killAssign records the kill point on the recorded entries of one
// assignment statement for the checked variable.
func (s *errDiscFuncScope) killAssign(obj varObj, assign *ast.AssignStmt, killEnd token.Pos) {
	entries := s.assigns[obj]
	for i := range entries {
		if entries[i].pos == assign.Pos() {
			entries[i].killEnd = killEnd
		}
	}
}

// collectExits resolves the error expression of every return statement of
// the function body itself; closures are skipped, since their returns are
// not exits of the enclosing function (database.Transaction closures are
// expanded at their call sites instead).
func (s *errDiscFuncScope) collectExits(body *ast.BlockStmt, summary *errDiscFuncSummary) {
	ast.Inspect(body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		summary.sources = append(summary.sources, s.resolveReturn(ret)...)
		return true
	})
}

// resolveReturn resolves the origins of the error value produced by one
// return statement.
func (s *errDiscFuncScope) resolveReturn(ret *ast.ReturnStmt) []errDiscSource {
	visiting := map[varObj]bool{}
	switch {
	case len(ret.Results) == 0:
		// Naked return: the named error result carries the value.
		return s.resolveObj(s.resultObj, ret, visiting)
	case len(ret.Results) == 1 && s.numResults > 1:
		// A single call expanded into all results; its error output is the
		// call's own error flow.
		return s.resolveExpr(ret.Results[0], visiting)
	default:
		return s.resolveExpr(ret.Results[len(ret.Results)-1], visiting)
	}
}

// resolveExpr resolves the origins of one error-typed expression.
func (s *errDiscFuncScope) resolveExpr(expr ast.Expr, visiting map[varObj]bool) []errDiscSource {
	switch expr := expr.(type) {
	case *ast.Ident:
		if expr.Name == "nil" {
			return []errDiscSource{{kind: errDiscSourceNil}}
		}
		if declObj(expr) == nil {
			// Unresolved identifier: a package-level error variable (a raw
			// sentinel) or a cross-file symbol; fail closed.
			return []errDiscSource{s.raw(expr)}
		}
		return s.resolveObj(declObj(expr), expr, visiting)
	case *ast.CallExpr:
		return s.resolveCall(expr, visiting)
	case *ast.ParenExpr:
		return s.resolveExpr(expr.X, visiting)
	case *ast.SelectorExpr:
		if s.database(expr) {
			return []errDiscSource{{kind: errDiscSourceDatabase, pos: s.file.analysis.fset.Position(expr.Pos())}}
		}
		return []errDiscSource{s.raw(expr)}
	default:
		return []errDiscSource{s.raw(expr)}
	}
}

// database reports whether expr is rooted at the framework database
// package: a call chain starting from one of its functions, such as
// database.Database[*Record](ctx).WithQuery(q).List(&records), or one of
// its exported names, such as the sentinel database.ErrRecordNotFound. A
// name the function declares itself hides the import (see calleeOf).
func (s *errDiscFuncScope) database(expr ast.Expr) bool {
	for {
		switch e := expr.(type) {
		case *ast.SelectorExpr:
			if ident, ok := e.X.(*ast.Ident); ok {
				return !s.declaresLocally(declObj(ident)) && slices.Contains(s.file.db.Qualifiers, ident.Name)
			}
			expr = e.X
		case *ast.CallExpr:
			expr = e.Fun
		case *ast.IndexExpr:
			expr = e.X
		case *ast.IndexListExpr:
			expr = e.X
		case *ast.ParenExpr:
			expr = e.X
		default:
			return false
		}
	}
}

// resolveObj resolves the origins of the value held by one declared
// variable, pooling the assignments recorded for its declaration object that
// happen before the use site. at names the use — the expression or statement
// to blame when nothing was recorded.
func (s *errDiscFuncScope) resolveObj(obj varObj, at ast.Node, visiting map[varObj]bool) []errDiscSource {
	if obj == nil || visiting[obj] {
		return nil
	}
	visiting[obj] = true
	defer delete(visiting, obj)
	var sources []errDiscSource
	found := false
	// A use inside an exclusive window sees only the window's own
	// assignment: the check's init just overwrote the variable. Nested
	// windows pick the innermost one, the most recent overwrite.
	var window *errDiscWindow
	for i := range s.windows[obj] {
		w := &s.windows[obj][i]
		if at.Pos() > w.bodyStart && at.Pos() < w.bodyEnd {
			if window == nil || w.bodyStart > window.bodyStart {
				window = w
			}
		}
	}
	for _, assign := range s.assigns[obj] {
		if window != nil && assign.pos != window.assignPos {
			continue
		}
		// A use always happens after the assignment feeding it, so later
		// assignments cannot be this use's origin; an assignment whose value
		// was consumed by its own error check is dead past that check.
		if assign.pos >= at.Pos() {
			continue
		}
		if assign.killEnd != token.NoPos && at.Pos() > assign.killEnd {
			continue
		}
		found = true
		sources = append(sources, s.resolveExpr(assign.expr, visiting)...)
	}
	if !found {
		// No assignment before the use: a parameter or captured value the
		// checker cannot see through; fail closed.
		return []errDiscSource{{kind: errDiscSourceRaw, pos: s.file.analysis.fset.Position(at.Pos())}}
	}
	return sources
}

// resolveCall resolves the origins of the error produced by one call: the
// framework constructors and the transaction are read for what they are,
// and so is the SSE method of a *gst.ServiceContext parameter, whose
// errors are framework-governed; a project function or method the call
// resolves to (see calleeOf) answers through its own summary, and a call
// the checker cannot follow fails closed.
func (s *errDiscFuncScope) resolveCall(call *ast.CallExpr, visiting map[varObj]bool) []errDiscSource {
	switch fun := instantiated(call.Fun).(type) {
	case *ast.Ident:
		// A dot import names the framework constructors and the
		// transaction without a qualifier; a name the function declares
		// itself hides them (see calleeOf).
		if !s.declaresLocally(declObj(fun)) {
			if s.file.gst.Refers(fun, "NewError", "NewErrorWithCause") {
				return []errDiscSource{{kind: errDiscSourceNewError}}
			}
			if s.file.db.Refers(fun, "Transaction") {
				return s.resolveTransaction(call, visiting)
			}
		}
	case *ast.SelectorExpr:
		ident, ok := fun.X.(*ast.Ident)
		if !ok || fun.Sel == nil {
			break
		}
		// The receiver, a parameter or a local variable hides every
		// package-level meaning of its name, an import included: with
		// gst := other{} in the body, gst.NewError() is other's
		// method, not the framework constructor.
		if obj := declObj(ident); s.declaresLocally(obj) {
			// ServiceContext.SSE errors are framework-governed: a setup
			// failure carries a framework-built message, and an error after
			// the stream opened never reaches the response envelope, so
			// wrapping the call in gst.NewError adds nothing the client
			// could see.
			if s.ctxParams[obj] && fun.Sel.Name == "SSE" {
				return []errDiscSource{{kind: errDiscSourceNewError}}
			}
			break
		}
		if s.file.gst.Refers(fun, "NewError", "NewErrorWithCause") {
			return []errDiscSource{{kind: errDiscSourceNewError}}
		}
		if s.file.db.Refers(fun, "Transaction") {
			return s.resolveTransaction(call, visiting)
		}
	}
	if s.database(call.Fun) {
		return []errDiscSource{{kind: errDiscSourceDatabase, pos: s.file.analysis.fset.Position(call.Pos())}}
	}
	callee, ok := s.calleeOf(call)
	if !ok {
		return []errDiscSource{s.raw(call)}
	}
	// Whether the call is compliant is the callee summary's business.
	return []errDiscSource{{kind: errDiscSourceCall, callee: callee, pos: s.file.analysis.fset.Position(call.Pos())}}
}

// instantiated returns the function a call expression calls, the
// instantiation of a generic function seen through: the wrapper is
// transparent for resolving who is called.
func instantiated(fun ast.Expr) ast.Expr {
	for {
		switch wrapper := fun.(type) {
		case *ast.IndexExpr:
			fun = wrapper.X
		case *ast.IndexListExpr:
			fun = wrapper.X
		default:
			return fun
		}
	}
}

// calleeOf returns the key of the project function or method the call
// calls, resolved the way the Go compiler reads the call: a bare name is a
// function of the package, pkg.F one of the project package pkg names,
// pkg.Var.M and Var.M a method of the type a package-level variable's
// declaration spells out (see collectPackageVars), and x.M on a name x the
// function declares itself a method of the receiver's type, or of the type
// the declaration of x spells out:
//
//	mgr := manager{}   // mgr.Do() is manager.Do
//	var mgr manager    // manager.Do
//	func run(m manager) error { return m.Do() } // manager.Do
//	cli := newClient() // no type spelled out: cli.Do() resolves to nothing
//
// It reports false for a call it cannot follow: a function value the
// function declares itself, such as a closure held in a local variable,
// even when a package-level function of the same name exists; a method of
// a type it cannot see; a call of another package's function the file does
// not import as a project package. A dot-imported project package names
// nothing here: its calls read as same-package calls.
func (s *errDiscFuncScope) calleeOf(call *ast.CallExpr) (errDiscFuncKey, bool) {
	switch fun := instantiated(call.Fun).(type) {
	case *ast.Ident:
		if s.declaresLocally(declObj(fun)) {
			return errDiscFuncKey{}, false
		}
		return errDiscFuncKey{pkgDir: s.file.pkgDir, name: fun.Name}, true
	case *ast.SelectorExpr:
		if fun.Sel == nil {
			return errDiscFuncKey{}, false
		}
		if varSel, ok := fun.X.(*ast.SelectorExpr); ok {
			if pkgIdent, ok := varSel.X.(*ast.Ident); ok && varSel.Sel != nil {
				if pkgDir, ok := s.file.projectPkg[pkgIdent.Name]; ok {
					if typeName, ok := s.file.analysis.pkgVarTypes[pkgDir][varSel.Sel.Name]; ok {
						return errDiscFuncKey{pkgDir: pkgDir, recv: typeName, name: fun.Sel.Name}, true
					}
				}
			}
			return errDiscFuncKey{}, false
		}
		ident, ok := fun.X.(*ast.Ident)
		if !ok {
			return errDiscFuncKey{}, false
		}
		if obj := declObj(ident); s.declaresLocally(obj) {
			recvType := s.recvType
			if obj != s.recvObj {
				recvType = errDiscDeclaredTypeName(obj)
			}
			if recvType == "" {
				return errDiscFuncKey{}, false
			}
			return errDiscFuncKey{pkgDir: s.file.pkgDir, recv: recvType, name: fun.Sel.Name}, true
		}
		if pkgDir, ok := s.file.projectPkg[ident.Name]; ok {
			return errDiscFuncKey{pkgDir: pkgDir, name: fun.Sel.Name}, true
		}
		if typeName, ok := s.file.analysis.pkgVarTypes[s.file.pkgDir][ident.Name]; ok {
			return errDiscFuncKey{pkgDir: s.file.pkgDir, recv: typeName, name: fun.Sel.Name}, true
		}
	}
	return errDiscFuncKey{}, false
}

// declaresLocally reports whether obj, the declaration object of a name used
// in the function, is declared by the function itself: its receiver, a
// parameter or result, or a variable of its body or of a closure inside it.
// A package-level declaration, even one in the same file, is not, and
// neither is the function's own name, which a recursive call uses.
func (s *errDiscFuncScope) declaresLocally(obj varObj) bool {
	if obj == nil {
		return false
	}
	node, ok := declNode(obj).(ast.Node)
	if _, isFunc := node.(*ast.FuncDecl); !ok || isFunc {
		return false
	}
	return node.Pos() >= s.decl.Pos() && node.Pos() < s.decl.End()
}

// errDiscDeclaredTypeName returns the same-package type name a variable's
// declaration spells out, the way collectPackageVars reads a package-level
// one: other for p other and p *other (a parameter), var x other,
// var x = other{} and x := &other{}. It returns "" when the declaration
// names no such type, as for x := newOther() or var x pkg.Other.
func errDiscDeclaredTypeName(obj varObj) string {
	switch decl := declNode(obj).(type) {
	case *ast.Field:
		return errDiscReceiverTypeName(decl.Type)
	case *ast.ValueSpec:
		for i, name := range decl.Names {
			if declObj(name) != obj {
				continue
			}
			switch {
			case decl.Type != nil:
				return errDiscReceiverTypeName(decl.Type)
			case i < len(decl.Values):
				return errDiscCompositeTypeName(decl.Values[i])
			}
		}
	case *ast.AssignStmt:
		if len(decl.Lhs) != len(decl.Rhs) {
			return ""
		}
		for i, lhs := range decl.Lhs {
			if name, ok := lhs.(*ast.Ident); ok && declObj(name) == obj {
				return errDiscCompositeTypeName(decl.Rhs[i])
			}
		}
	}
	return ""
}

// resolveTransaction expands a database.Transaction call: the error it
// returns is whatever the closure exits return, so those exits join the
// enclosing flow. A non-literal transaction function cannot be followed and
// fails closed.
func (s *errDiscFuncScope) resolveTransaction(call *ast.CallExpr, visiting map[varObj]bool) []errDiscSource {
	if len(call.Args) != 2 {
		return []errDiscSource{s.raw(call)}
	}
	closure, ok := call.Args[1].(*ast.FuncLit)
	if !ok || closure.Body == nil {
		return []errDiscSource{s.raw(call)}
	}
	var sources []errDiscSource
	ast.Inspect(closure.Body, func(n ast.Node) bool {
		if inner, ok := n.(*ast.FuncLit); ok && inner != closure {
			return false
		}
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		if len(ret.Results) != 1 {
			return true
		}
		sources = append(sources, s.resolveExpr(ret.Results[0], visiting)...)
		return true
	})
	return sources
}

// raw builds the fail-closed source pointing at the expression itself: that
// is the place to wrap.
func (s *errDiscFuncScope) raw(expr ast.Expr) errDiscSource {
	return errDiscSource{kind: errDiscSourceRaw, pos: s.file.analysis.fset.Position(expr.Pos())}
}

// report walks the error flow from every service method and every model
// hook and lists each reachable raw source once, ordered by position. The
// two walks keep separate visited sets: a function reached from both is
// read once with the database errors raw and once with them accepted.
func (a *errDiscAnalysis) report() []string {
	seen := map[string]bool{}
	var positions []token.Position
	visited := map[errDiscFuncKey]bool{}
	for _, entry := range a.entries {
		a.collectRawSources(entry, visited, seen, &positions, false)
	}
	hookVisited := map[errDiscFuncKey]bool{}
	for _, entry := range a.hookEntries {
		a.collectRawSources(entry, hookVisited, seen, &positions, true)
	}

	sort.Slice(positions, func(i, j int) bool {
		if positions[i].Filename != positions[j].Filename {
			return positions[i].Filename < positions[j].Filename
		}
		return positions[i].Line < positions[j].Line
	})

	violations := make([]string, 0, len(positions))
	for _, pos := range positions {
		violations = append(violations, fmt.Sprintf(
			"%s:%d: error on an exit path of a service method or model hook is created outside gst.NewError/gst.NewErrorWithCause; construct it here (or in the project function it flows through) so the client gets the status and message meant for it",
			filepath.ToSlash(pos.Filename), pos.Line,
		))
	}
	return violations
}

// collectRawSources accumulates the raw sources reachable from one function's
// error exits, following project calls and deduplicating by position; with
// acceptDatabase the errors of the framework database package are not raw.
func (a *errDiscAnalysis) collectRawSources(key errDiscFuncKey, visited map[errDiscFuncKey]bool, seen map[string]bool, out *[]token.Position, acceptDatabase bool) {
	if visited[key] {
		return
	}
	visited[key] = true
	summary, ok := a.summaries[key]
	if !ok {
		return
	}
	for _, source := range summary.sources {
		switch source.kind {
		case errDiscSourceRaw:
			a.recordRaw(source.pos, seen, out)
		case errDiscSourceDatabase:
			if !acceptDatabase {
				a.recordRaw(source.pos, seen, out)
			}
		case errDiscSourceCall:
			if _, ok := a.summaries[source.callee]; !ok {
				// A call the checker cannot follow (builtin, embedded method,
				// unresolved package): fail closed at the call site.
				a.recordRaw(source.pos, seen, out)
				continue
			}
			a.collectRawSources(source.callee, visited, seen, out, acceptDatabase)
		}
	}
}

// recordRaw appends one raw-source position, deduplicated across entries.
func (a *errDiscAnalysis) recordRaw(pos token.Position, seen map[string]bool, out *[]token.Position) {
	id := fmt.Sprintf("%s:%d", pos.Filename, pos.Line)
	if seen[id] {
		return
	}
	seen[id] = true
	*out = append(*out, pos)
}
