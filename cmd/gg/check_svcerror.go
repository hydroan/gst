package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// gstServiceImportPath is the framework package whose NewError and
// NewErrorWithCause constructors are the only sanctioned way to build errors
// that leave a service method.
const gstServiceImportPath = "github.com/hydroan/gst/service"

// CheckServiceErrorDiscipline checks that every error a service method can
// return is created by service.NewError or service.NewErrorWithCause, either
// directly at the exit or inside a project function the exit's error flows
// from. An error built any other way reaches the client and the logs as-is:
// its message leaks internal wording instead of an operator-facing one, and
// it usually carries no useful stack, so the error_stack log field cannot
// locate the failing service code.
//
// The analysis is purely syntactic, mirroring the other project checks. It
// summarizes, per project function whose last result is error, where the
// returned error values come from, then walks the flow from every service
// method (a method on a struct embedding service.Base) and reports each raw
// source it can reach: framework and third-party calls returned as-is, raw
// cockroachdb constructors, and identifiers whose origin cannot be resolved.
// database.Transaction calls are transparent: their closure exits are
// treated as exits of the enclosing flow. Unresolvable constructs fail
// closed, so an exit the checker cannot prove compliant is a violation.
func CheckServiceErrorDiscipline() []string {
	analysis := &svcErrAnalysis{
		modulePath:  currentProjectModulePath(),
		fset:        token.NewFileSet(),
		summaries:   map[svcErrFuncKey]*svcErrFuncSummary{},
		entryTypes:  map[string]map[string]bool{},
		pkgVarTypes: map[string]map[string]string{},
	}
	if analysis.modulePath == "" {
		return nil
	}

	ignoreMatcher := newProjectIgnoreMatcher()
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if path == "." {
				return nil
			}
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") || base == "vendor" || base == "testdata" {
				return filepath.SkipDir
			}
			// Nested Go modules belong to other projects.
			if _, statErr := os.Stat(filepath.Join(path, "go.mod")); statErr == nil {
				return filepath.SkipDir
			}
			if ignoreMatcher != nil && ignoreMatcher.Match(strings.Split(path, string(filepath.Separator)), true) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		analysis.collectFile(path)
		return nil
	})
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

// svcErrVarObj is the parser-resolved declaration object of a local
// variable. ast.Object is deprecated because syntactic resolution is
// ambiguous without type information (composite literal keys, selector
// fields); this checker resolves only plain local variables in assignments
// and returns, a subset the parser's lexical scoping gets right, and it must
// stay off go/types to keep gg gen fast. Every use of the deprecated API is
// confined to this alias and svcErrDeclObj.
//
//nolint:staticcheck // SA1019: sound for the local-variable subset, see above.
type svcErrVarObj = *ast.Object

// svcErrDeclObj returns the declaration object of an identifier, the single
// accessor for the deprecated field.
func svcErrDeclObj(ident *ast.Ident) svcErrVarObj {
	return ident.Obj
}

// svcErrFuncKey identifies a project function or method: the package
// directory, the receiver type name ("" for package-level functions), and
// the function name.
type svcErrFuncKey struct {
	pkgDir string
	recv   string
	name   string
}

type svcErrSourceKind int

const (
	svcErrSourceNil svcErrSourceKind = iota
	svcErrSourceNewError
	svcErrSourceCall
	svcErrSourceRaw
)

// svcErrSource is one origin an error exit value can flow from.
type svcErrSource struct {
	kind   svcErrSourceKind
	callee svcErrFuncKey  // set for svcErrSourceCall
	pos    token.Position // set for svcErrSourceCall and svcErrSourceRaw
}

// svcErrFuncSummary aggregates the origins of every error exit of one
// function, closure exits of database.Transaction included.
type svcErrFuncSummary struct {
	sources []svcErrSource
}

// svcErrAnalysis carries the whole-project state: per-function summaries and
// the service struct types whose methods are the checked entry points.
type svcErrAnalysis struct {
	modulePath  string
	fset        *token.FileSet
	summaries   map[svcErrFuncKey]*svcErrFuncSummary
	entryTypes  map[string]map[string]bool
	pkgVarTypes map[string]map[string]string
	entries     []svcErrFuncKey
	files       []*svcErrFileCollector
}

// collectFile parses one project file and records service struct types and
// function summaries.
func (a *svcErrAnalysis) collectFile(path string) {
	file, err := parser.ParseFile(a.fset, path, nil, 0)
	if err != nil {
		return
	}

	collector := &svcErrFileCollector{
		analysis:   a,
		pkgDir:     filepath.ToSlash(filepath.Dir(path)),
		projectPkg: map[string]string{},
	}
	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}
		importPath := strings.Trim(imp.Path.Value, `"`)
		name := filepath.Base(importPath)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		switch {
		case importPath == gstServiceImportPath:
			collector.svcAliases = append(collector.svcAliases, name)
		case importPath == gstDatabaseImportPath:
			collector.dbAliases = append(collector.dbAliases, name)
		case importPath == a.modulePath:
			collector.projectPkg[name] = "."
		case strings.HasPrefix(importPath, a.modulePath+"/"):
			collector.projectPkg[name] = strings.TrimPrefix(importPath, a.modulePath+"/")
		}
	}

	collector.file = file
	for _, decl := range file.Decls {
		if decl, ok := decl.(*ast.GenDecl); ok {
			collector.collectServiceTypes(decl)
			collector.collectPackageVars(decl)
		}
	}
	a.files = append(a.files, collector)
}

// svcErrFileCollector is the per-file context: the parsed file plus import
// aliases of the framework packages and of project-internal packages.
type svcErrFileCollector struct {
	analysis   *svcErrAnalysis
	file       *ast.File
	pkgDir     string
	svcAliases []string
	dbAliases  []string
	projectPkg map[string]string
}

// collectPackageVars records the concrete type of package-level variables
// declared as composite literals or with an explicit type, so method calls on
// them (for example a package-level manager singleton) resolve to that type's
// methods instead of failing closed.
func (c *svcErrFileCollector) collectPackageVars(decl *ast.GenDecl) {
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
				typeName = svcErrReceiverTypeName(valueSpec.Type)
			case i < len(valueSpec.Values):
				typeName = svcErrCompositeTypeName(valueSpec.Values[i])
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

// svcErrCompositeTypeName extracts the local type name from a composite
// literal value, with or without a leading address operator.
func svcErrCompositeTypeName(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.UnaryExpr:
		if expr.Op == token.AND {
			return svcErrCompositeTypeName(expr.X)
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
func (c *svcErrFileCollector) collectServiceTypes(decl *ast.GenDecl) {
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
			if len(field.Names) > 0 || !c.isServiceBase(field.Type) {
				continue
			}
			types := c.analysis.entryTypes[c.pkgDir]
			if types == nil {
				types = map[string]bool{}
				c.analysis.entryTypes[c.pkgDir] = types
			}
			types[typeSpec.Name.Name] = true
		}
	}
}

// isServiceBase reports whether expr denotes service.Base under any
// recognized import alias, with or without type arguments.
func (c *svcErrFileCollector) isServiceBase(expr ast.Expr) bool {
	switch expr := expr.(type) {
	case *ast.IndexExpr:
		return c.isServiceBase(expr.X)
	case *ast.IndexListExpr:
		return c.isServiceBase(expr.X)
	case *ast.SelectorExpr:
		ident, ok := expr.X.(*ast.Ident)
		if !ok || expr.Sel == nil || expr.Sel.Name != "Base" {
			return false
		}
		return slices.Contains(c.svcAliases, ident.Name)
	}
	return false
}

// collectFunc summarizes one function whose last result is error.
func (c *svcErrFileCollector) collectFunc(decl *ast.FuncDecl) {
	if decl.Body == nil || decl.Type.Results == nil || len(decl.Type.Results.List) == 0 {
		return
	}
	results := decl.Type.Results.List
	last := results[len(results)-1]
	// A function returning *service.Error is compliant by construction: every
	// non-nil value of that type came from NewError or NewErrorWithCause.
	if c.isServiceErrorPtr(last.Type) {
		key := svcErrFuncKey{pkgDir: c.pkgDir, recv: svcErrReceiverTypeName(receiverType(decl)), name: decl.Name.Name}
		c.analysis.summaries[key] = &svcErrFuncSummary{sources: []svcErrSource{{kind: svcErrSourceNewError}}}
		return
	}
	if ident, ok := last.Type.(*ast.Ident); !ok || ident.Name != "error" {
		return
	}

	scope := &svcErrFuncScope{
		file:       c,
		numResults: 0,
	}
	for _, field := range results {
		n := len(field.Names)
		if n == 0 {
			n = 1
		}
		scope.numResults += n
	}
	if names := last.Names; len(names) > 0 {
		scope.resultObj = svcErrDeclObj(names[len(names)-1])
	}
	if decl.Recv != nil && len(decl.Recv.List) == 1 {
		scope.recvType = svcErrReceiverTypeName(decl.Recv.List[0].Type)
		if names := decl.Recv.List[0].Names; len(names) == 1 {
			scope.recvName = names[0].Name
		}
	}

	scope.collectAssigns(decl.Body)
	summary := &svcErrFuncSummary{}
	scope.collectExits(decl.Body, summary)

	key := svcErrFuncKey{pkgDir: c.pkgDir, recv: scope.recvType, name: decl.Name.Name}
	c.analysis.summaries[key] = summary
	if decl.Recv != nil && decl.Name.IsExported() && c.analysis.entryTypes[c.pkgDir][scope.recvType] {
		c.analysis.entries = append(c.analysis.entries, key)
	}
}

// receiverType returns the receiver type expression of a method declaration,
// or nil for a plain function.
func receiverType(decl *ast.FuncDecl) ast.Expr {
	if decl.Recv == nil || len(decl.Recv.List) != 1 {
		return nil
	}
	return decl.Recv.List[0].Type
}

// isServiceErrorPtr reports whether expr denotes *service.Error under any
// recognized import alias of the framework service package.
func (c *svcErrFileCollector) isServiceErrorPtr(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil || sel.Sel.Name != "Error" {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && slices.Contains(c.svcAliases, ident.Name)
}

// svcErrReceiverTypeName extracts the receiver's type name, unwrapping
// pointers and type parameters.
func svcErrReceiverTypeName(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.StarExpr:
		return svcErrReceiverTypeName(expr.X)
	case *ast.IndexExpr:
		return svcErrReceiverTypeName(expr.X)
	case *ast.IndexListExpr:
		return svcErrReceiverTypeName(expr.X)
	case *ast.Ident:
		return expr.Name
	}
	return ""
}

// svcErrFuncScope carries the per-function state used to resolve where the
// error values returned by the function come from.
type svcErrFuncScope struct {
	file       *svcErrFileCollector
	recvName   string
	recvType   string
	resultObj  svcErrVarObj // named error result, for naked returns
	numResults int
	// assigns maps a declared variable to every expression assigned to it,
	// closures included. Keying by the parser-resolved declaration object
	// keeps same-named variables from different scopes apart, and each entry
	// records its position so a return only pools assignments that happened
	// before it: reusing one err variable for several sources must not let a
	// later raw assignment pollute an earlier compliant exit.
	assigns map[svcErrVarObj][]svcErrAssign
	// windows lists the exclusive visibility windows per variable; a use
	// inside a window sees only the window's own assignment.
	windows map[svcErrVarObj][]svcErrWindow
}

// svcErrAssign is one recorded assignment: the assigned expression, where
// the assignment happens, and where its value stops being visible. killEnd
// is set when the assignment is immediately answered by an
// `if <var> != nil { return ... }` style check: past that check the variable
// no longer holds this value, so later uses must not pool it.
type svcErrAssign struct {
	expr    ast.Expr
	pos     token.Pos
	killEnd token.Pos
}

// svcErrWindow is an exclusive visibility window: inside the body of an
// `if <var> != nil` check that the assignment at assignPos feeds, the
// variable can hold only that value, regardless of what it held before.
type svcErrWindow struct {
	assignPos token.Pos
	bodyStart token.Pos
	bodyEnd   token.Pos
}

// collectAssigns records every assignment in the function body, closures
// included, keyed by the assigned variable's declaration object.
func (s *svcErrFuncScope) collectAssigns(body *ast.BlockStmt) {
	s.assigns = map[svcErrVarObj][]svcErrAssign{}
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		if len(assign.Rhs) == 1 && len(assign.Lhs) > 1 {
			// Multi-value assignment from one call: every variable pools the
			// call as origin; only the error-typed one ever reaches an exit.
			for _, lhs := range assign.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok && svcErrDeclObj(ident) != nil {
					s.assigns[svcErrDeclObj(ident)] = append(s.assigns[svcErrDeclObj(ident)], svcErrAssign{expr: assign.Rhs[0], pos: assign.Pos()})
				}
			}
			return true
		}
		if len(assign.Rhs) != len(assign.Lhs) {
			return true
		}
		for i, lhs := range assign.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok && svcErrDeclObj(ident) != nil {
				s.assigns[svcErrDeclObj(ident)] = append(s.assigns[svcErrDeclObj(ident)], svcErrAssign{expr: assign.Rhs[i], pos: assign.Pos()})
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
func (s *svcErrFuncScope) markKilledAssigns(body *ast.BlockStmt) {
	s.windows = map[svcErrVarObj][]svcErrWindow{}
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
			checkedObj := svcErrNilCheckedObj(ifStmt)
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
			s.windows[checkedObj] = append(s.windows[checkedObj], svcErrWindow{
				assignPos: assign.Pos(),
				bodyStart: ifStmt.Body.Pos(),
				bodyEnd:   ifStmt.Body.End(),
			})
			// A check whose body always leaves also consumes the value for
			// everything after the check.
			if svcErrStmtsAlwaysLeave(ifStmt.Body.List) {
				s.killAssign(checkedObj, assign, ifStmt.End())
			}
		}
		return true
	})
}

// svcErrNilCheckedObj returns the declaration object of v when cond is a
// plain `v != nil` comparison, nil otherwise.
func svcErrNilCheckedObj(ifStmt *ast.IfStmt) svcErrVarObj {
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
	return svcErrDeclObj(ident)
}

// svcErrStmtsAlwaysLeave reports whether a statement list ends by leaving
// the surrounding flow: a return, a branch statement, or a panic call.
func svcErrStmtsAlwaysLeave(stmts []ast.Stmt) bool {
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
func (s *svcErrFuncScope) killAssign(obj svcErrVarObj, assign *ast.AssignStmt, killEnd token.Pos) {
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
func (s *svcErrFuncScope) collectExits(body *ast.BlockStmt, summary *svcErrFuncSummary) {
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
func (s *svcErrFuncScope) resolveReturn(ret *ast.ReturnStmt) []svcErrSource {
	visiting := map[svcErrVarObj]bool{}
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
func (s *svcErrFuncScope) resolveExpr(expr ast.Expr, visiting map[svcErrVarObj]bool) []svcErrSource {
	switch expr := expr.(type) {
	case *ast.Ident:
		if expr.Name == "nil" {
			return []svcErrSource{{kind: svcErrSourceNil}}
		}
		if svcErrDeclObj(expr) == nil {
			// Unresolved identifier: a package-level error variable (a raw
			// sentinel) or a cross-file symbol; fail closed.
			return []svcErrSource{s.raw(expr)}
		}
		return s.resolveObj(svcErrDeclObj(expr), expr, visiting)
	case *ast.CallExpr:
		return s.resolveCall(expr, visiting)
	case *ast.ParenExpr:
		return s.resolveExpr(expr.X, visiting)
	default:
		return []svcErrSource{s.raw(expr)}
	}
}

// resolveObj resolves the origins of the value held by one declared
// variable, pooling the assignments recorded for its declaration object that
// happen before the use site. at names the use — the expression or statement
// to blame when nothing was recorded.
func (s *svcErrFuncScope) resolveObj(obj svcErrVarObj, at ast.Node, visiting map[svcErrVarObj]bool) []svcErrSource {
	if obj == nil || visiting[obj] {
		return nil
	}
	visiting[obj] = true
	defer delete(visiting, obj)
	var sources []svcErrSource
	found := false
	// A use inside an exclusive window sees only the window's own
	// assignment: the check's init just overwrote the variable. Nested
	// windows pick the innermost one, the most recent overwrite.
	var window *svcErrWindow
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
		return []svcErrSource{{kind: svcErrSourceRaw, pos: s.file.analysis.fset.Position(at.Pos())}}
	}
	return sources
}

// resolveCall resolves the origins of the error produced by one call.
func (s *svcErrFuncScope) resolveCall(call *ast.CallExpr, visiting map[svcErrVarObj]bool) []svcErrSource {
	fun := call.Fun
	// A generic call instantiates its function first; the instantiation
	// wrapper is transparent for resolving who is called.
	for {
		switch instantiated := fun.(type) {
		case *ast.IndexExpr:
			fun = instantiated.X
			continue
		case *ast.IndexListExpr:
			fun = instantiated.X
			continue
		}
		break
	}
	switch fun := fun.(type) {
	case *ast.Ident:
		// A same-package call; whether it is compliant is the callee
		// summary's business.
		return []svcErrSource{{
			kind:   svcErrSourceCall,
			callee: svcErrFuncKey{pkgDir: s.file.pkgDir, name: fun.Name},
			pos:    s.file.analysis.fset.Position(call.Pos()),
		}}
	case *ast.SelectorExpr:
		if fun.Sel == nil {
			return []svcErrSource{s.raw(call)}
		}
		// A method call on another project package's package-level variable
		// (pkg.Manager.Method) resolves through that package's recorded
		// variable types.
		if varSel, ok := fun.X.(*ast.SelectorExpr); ok {
			if pkgIdent, ok := varSel.X.(*ast.Ident); ok && varSel.Sel != nil {
				if pkgDir, ok := s.file.projectPkg[pkgIdent.Name]; ok {
					if typeName, ok := s.file.analysis.pkgVarTypes[pkgDir][varSel.Sel.Name]; ok {
						return []svcErrSource{{
							kind:   svcErrSourceCall,
							callee: svcErrFuncKey{pkgDir: pkgDir, recv: typeName, name: fun.Sel.Name},
							pos:    s.file.analysis.fset.Position(call.Pos()),
						}}
					}
				}
			}
			return []svcErrSource{s.raw(call)}
		}
		ident, ok := fun.X.(*ast.Ident)
		if !ok {
			return []svcErrSource{s.raw(call)}
		}
		if slices.Contains(s.file.svcAliases, ident.Name) && (fun.Sel.Name == "NewError" || fun.Sel.Name == "NewErrorWithCause") {
			return []svcErrSource{{kind: svcErrSourceNewError}}
		}
		if slices.Contains(s.file.dbAliases, ident.Name) && fun.Sel.Name == "Transaction" {
			return s.resolveTransaction(call, visiting)
		}
		if s.recvName != "" && ident.Name == s.recvName {
			return []svcErrSource{{
				kind:   svcErrSourceCall,
				callee: svcErrFuncKey{pkgDir: s.file.pkgDir, recv: s.recvType, name: fun.Sel.Name},
				pos:    s.file.analysis.fset.Position(call.Pos()),
			}}
		}
		if pkgDir, ok := s.file.projectPkg[ident.Name]; ok {
			return []svcErrSource{{
				kind:   svcErrSourceCall,
				callee: svcErrFuncKey{pkgDir: pkgDir, name: fun.Sel.Name},
				pos:    s.file.analysis.fset.Position(call.Pos()),
			}}
		}
		// A method call on a same-package package-level variable (a manager
		// singleton) resolves through the variable's recorded concrete type.
		// Local variables never register there, so the lookup alone decides;
		// a same-named local shadowing the package variable is not a shape
		// this project uses.
		if typeName, ok := s.file.analysis.pkgVarTypes[s.file.pkgDir][ident.Name]; ok {
			return []svcErrSource{{
				kind:   svcErrSourceCall,
				callee: svcErrFuncKey{pkgDir: s.file.pkgDir, recv: typeName, name: fun.Sel.Name},
				pos:    s.file.analysis.fset.Position(call.Pos()),
			}}
		}
		return []svcErrSource{s.raw(call)}
	default:
		return []svcErrSource{s.raw(call)}
	}
}

// resolveTransaction expands a database.Transaction call: the error it
// returns is whatever the closure exits return, so those exits join the
// enclosing flow. A non-literal transaction function cannot be followed and
// fails closed.
func (s *svcErrFuncScope) resolveTransaction(call *ast.CallExpr, visiting map[svcErrVarObj]bool) []svcErrSource {
	if len(call.Args) != 2 {
		return []svcErrSource{s.raw(call)}
	}
	closure, ok := call.Args[1].(*ast.FuncLit)
	if !ok || closure.Body == nil {
		return []svcErrSource{s.raw(call)}
	}
	var sources []svcErrSource
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
func (s *svcErrFuncScope) raw(expr ast.Expr) svcErrSource {
	return svcErrSource{kind: svcErrSourceRaw, pos: s.file.analysis.fset.Position(expr.Pos())}
}

// report walks the error flow from every service entry method and lists each
// reachable raw source once, ordered by position.
func (a *svcErrAnalysis) report() []string {
	seen := map[string]bool{}
	var positions []token.Position
	visited := map[svcErrFuncKey]bool{}
	for _, entry := range a.entries {
		a.collectRawSources(entry, visited, seen, &positions)
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
			"%s:%d: error on a service exit path is created outside service.NewError/service.NewErrorWithCause; construct it here (or in the project function it flows through) so the client gets a curated status and message and the log gets a service-level stack",
			filepath.ToSlash(pos.Filename), pos.Line,
		))
	}
	return violations
}

// collectRawSources accumulates the raw sources reachable from one function's
// error exits, following project calls and deduplicating by position.
func (a *svcErrAnalysis) collectRawSources(key svcErrFuncKey, visited map[svcErrFuncKey]bool, seen map[string]bool, out *[]token.Position) {
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
		case svcErrSourceRaw:
			a.recordRaw(source.pos, seen, out)
		case svcErrSourceCall:
			if _, ok := a.summaries[source.callee]; !ok {
				// A call the checker cannot follow (builtin, embedded method,
				// unresolved package): fail closed at the call site.
				a.recordRaw(source.pos, seen, out)
				continue
			}
			a.collectRawSources(source.callee, visited, seen, out)
		}
	}
}

// recordRaw appends one raw-source position, deduplicated across entries.
func (a *svcErrAnalysis) recordRaw(pos token.Position, seen map[string]bool, out *[]token.Position) {
	id := fmt.Sprintf("%s:%d", pos.Filename, pos.Line)
	if seen[id] {
		return
	}
	seen[id] = true
	*out = append(*out, pos)
}
