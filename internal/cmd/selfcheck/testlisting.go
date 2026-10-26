package main

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// listingFuncs are the calls that list a directory, by package path and
// function name: os.ReadDir and os.DirFS read the directory their argument
// names, filepath.WalkDir and filepath.Walk every directory below it, and
// filepath.Glob the directories its pattern spans.
var listingFuncs = map[string]map[string]bool{
	"os":            {"ReadDir": true, "DirFS": true},
	"path/filepath": {"WalkDir": true, "Walk": true, "Glob": true},
}

// checkTestListing reports the directory listings of the test files whose
// directory the check cannot place inside the test's own package or under a
// directory the test made, t.TempDir() or os.MkdirTemp. go test keys a
// cached result on every entry of every directory the test lists, its name,
// size and modification time, so a listing of another package's directory,
// or of the repository's, reruns the test whenever anything there moves, an
// editor saving a file and git writing .git among it, and the cache never
// holds. A test reads another package's files through golist.Packages of
// internal/testutil/golist instead, which lists no directory.
//
// The path is placed by where it comes from, followed through the package's
// own code: a literal or a constant that is relative and has no ".." element
// stays inside, and so does the file runtime.Caller reports and its
// directory; t.TempDir(), b.TempDir(), os.MkdirTemp and os.TempDir name a
// directory outside the repository; filepath.Join, Dir, Clean, Abs,
// FromSlash, ToSlash and string concatenation keep what their parts are, the
// parent of the package's own directory excepted; a variable is what every
// value assigned to it is, the elements of what a range runs over among them;
// a parameter is what every call of its function in the package passes; and
// a call of a function of the package is what every one of its return
// statements gives back. Any other path, one go list or an environment
// variable produced among them, is reported, as is an absolute one spelt out
// or one with a ".." element anywhere.
func checkTestListing(root string, pkgs []*packages.Package) ([]violation, error) {
	type found struct {
		file, message string
		line          int
	}
	var all []found
	seen := make(map[string]bool)
	for _, p := range pkgs {
		var flow *pathFlow
		for i, file := range p.Syntax {
			path := relative(root, p.CompiledGoFiles[i])
			if filepath.IsAbs(path) || !strings.HasSuffix(path, "_test.go") || seen[path] {
				continue
			}
			seen[path] = true
			if flow == nil {
				flow = newPathFlow(p)
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				name, listing := flow.listingCall(call)
				if !listing || flow.place(call.Args[0], 0) != placeElsewhere {
					return true
				}
				pos := p.Fset.Position(call.Pos())
				all = append(all, found{
					file: path,
					line: pos.Line,
					message: fmt.Sprintf("Test file '%s:%d': %s lists a directory the check cannot place inside the package or under t.TempDir(): "+
						"go test keys the cached result on every entry of a listed directory, so a listing elsewhere reruns the test whenever anything there moves; "+
						"read the files of another package through golist.Packages of internal/testutil/golist, which lists no directory", path, pos.Line, name),
				})
				return true
			})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].file != all[j].file {
			return all[i].file < all[j].file
		}
		return all[i].line < all[j].line
	})
	violations := make([]violation, 0, len(all))
	for _, f := range all {
		violations = append(violations, violation{File: f.file, Message: f.message})
	}
	return violations, nil
}

// place is where a path a test lists lies, as far as the check can tell.
type place int

const (
	// placeKept is a path inside the package's own directory, relative or
	// absolute, or under a directory the test made.
	placeKept place = iota
	// placeCallerFile is the absolute path of the test file runtime.Caller
	// reports, whose directory is the package's.
	placeCallerFile
	// placeOwnDir is the absolute path of the package's directory, whose
	// parent is outside it.
	placeOwnDir
	// placeElsewhere is a path outside the package or one the check cannot
	// place.
	placeElsewhere
	// placeUnset is no place yet, what either starts from.
	placeUnset
)

// maxFlowDepth bounds how many steps the check follows a path back, through
// assignments, parameters and returns, before giving up on placing it.
const maxFlowDepth = 12

// pathFlow follows the paths of one package back to where they come from.
type pathFlow struct {
	info *types.Info
	// assigned holds the expressions assigned to each variable, the
	// expression a range runs over standing for its element.
	assigned map[*types.Var][]ast.Expr
	// ranged marks the expressions in assigned that a range runs over.
	ranged map[ast.Expr]bool
	// params locates each parameter of a function the package declares.
	params map[*types.Var]paramSlot
	// calls holds the calls of each function the package declares.
	calls map[*types.Func][]*ast.CallExpr
	// returns holds the return statements of each function the package
	// declares.
	returns map[*types.Func][]*ast.ReturnStmt
}

// paramSlot is the position of a parameter in the signature of fn.
type paramSlot struct {
	fn    *types.Func
	index int
}

func newPathFlow(p *packages.Package) *pathFlow {
	flow := &pathFlow{
		info:     p.TypesInfo,
		assigned: make(map[*types.Var][]ast.Expr),
		ranged:   make(map[ast.Expr]bool),
		params:   make(map[*types.Var]paramSlot),
		calls:    make(map[*types.Func][]*ast.CallExpr),
		returns:  make(map[*types.Func][]*ast.ReturnStmt),
	}
	for _, file := range p.Syntax {
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			fn, _ := p.TypesInfo.Defs[fd.Name].(*types.Func)
			if fn == nil {
				continue
			}
			index := 0
			for _, field := range fd.Type.Params.List {
				for _, name := range field.Names {
					if v, ok := p.TypesInfo.Defs[name].(*types.Var); ok {
						flow.params[v] = paramSlot{fn: fn, index: index}
					}
					index++
				}
				if len(field.Names) == 0 {
					index++
				}
			}
			if fd.Body != nil {
				ast.Inspect(fd.Body, func(n ast.Node) bool {
					switch n := n.(type) {
					case *ast.FuncLit:
						return false
					case *ast.ReturnStmt:
						flow.returns[fn] = append(flow.returns[fn], n)
					}
					return true
				})
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				if fn, ok := flow.callee(n).(*types.Func); ok {
					flow.calls[fn] = append(flow.calls[fn], n)
				}
			case *ast.AssignStmt:
				flow.recordAssign(n.Lhs, n.Rhs)
			case *ast.ValueSpec:
				lhs := make([]ast.Expr, len(n.Names))
				for i, name := range n.Names {
					lhs[i] = name
				}
				flow.recordAssign(lhs, n.Values)
			case *ast.RangeStmt:
				if n.Value != nil {
					if v := flow.variable(n.Value); v != nil {
						flow.assigned[v] = append(flow.assigned[v], n.X)
						flow.ranged[n.X] = true
					}
				}
				if n.Key != nil {
					if v := flow.variable(n.Key); v != nil {
						// A key indexes the collection; it names no path the
						// collection holds.
						flow.assigned[v] = append(flow.assigned[v], nil)
					}
				}
			}
			return true
		})
	}
	return flow
}

// recordAssign records what each variable of lhs is assigned from rhs: the
// expression at its position, or, for one call giving several results, the
// call itself, which place reads by the position the variable takes.
func (flow *pathFlow) recordAssign(lhs, rhs []ast.Expr) {
	for i, target := range lhs {
		v := flow.variable(target)
		if v == nil {
			continue
		}
		switch {
		case len(rhs) == len(lhs):
			flow.assigned[v] = append(flow.assigned[v], rhs[i])
		case len(rhs) == 1:
			flow.assigned[v] = append(flow.assigned[v], &resultOf{call: rhs[0], index: i})
		default:
			flow.assigned[v] = append(flow.assigned[v], nil)
		}
	}
}

// resultOf stands, among the expressions assigned to a variable, for the
// index-th result of a call giving several.
type resultOf struct {
	ast.Expr
	call  ast.Expr
	index int
}

// variable returns the variable an identifier declares or refers to.
func (flow *pathFlow) variable(e ast.Expr) *types.Var {
	id, ok := e.(*ast.Ident)
	if !ok || id.Name == "_" {
		return nil
	}
	if v, ok := flow.info.Defs[id].(*types.Var); ok {
		return v
	}
	v, _ := flow.info.Uses[id].(*types.Var)
	return v
}

// callee returns the object a call calls: a function, a method or a
// variable holding a function.
func (flow *pathFlow) callee(call *ast.CallExpr) types.Object {
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		return flow.info.Uses[fun]
	case *ast.SelectorExpr:
		return flow.info.Uses[fun.Sel]
	}
	return nil
}

// listingCall reports whether call lists a directory, with the name it is
// reported by.
func (flow *pathFlow) listingCall(call *ast.CallExpr) (string, bool) {
	fn, ok := flow.callee(call).(*types.Func)
	if !ok || fn.Pkg() == nil || !listingFuncs[fn.Pkg().Path()][fn.Name()] {
		return "", false
	}
	return fn.Pkg().Name() + "." + fn.Name(), true
}

// place tells where the path e names lies, following it back depth steps at
// most.
func (flow *pathFlow) place(e ast.Expr, depth int) place {
	if e == nil || depth > maxFlowDepth {
		return placeElsewhere
	}
	if tv, ok := flow.info.Types[e]; ok && tv.Value != nil && tv.Value.Kind() == constant.String {
		return literalPlace(constant.StringVal(tv.Value))
	}
	switch e := e.(type) {
	case *ast.ParenExpr:
		return flow.place(e.X, depth+1)
	case *resultOf:
		return flow.resultPlace(e.call, e.index, depth+1)
	case *ast.BinaryExpr:
		return joinPlaces(flow.fragmentPlace(e.X, depth+1), flow.fragmentPlace(e.Y, depth+1))
	case *ast.Ident, *ast.SelectorExpr:
		v, ok := flow.objectOf(e).(*types.Var)
		if !ok {
			return placeElsewhere
		}
		return flow.variablePlace(v, depth+1)
	case *ast.CallExpr:
		return flow.resultPlace(e, 0, depth+1)
	}
	return placeElsewhere
}

// fragmentPlace places one operand of a string concatenation: a part spelt
// out, "/" + name say, leaves only through a ".." element, and any other is
// placed as a path.
func (flow *pathFlow) fragmentPlace(e ast.Expr, depth int) place {
	if tv, ok := flow.info.Types[e]; ok && tv.Value != nil && tv.Value.Kind() == constant.String {
		if climbsOut(constant.StringVal(tv.Value)) {
			return placeElsewhere
		}
		return placeKept
	}
	return flow.place(e, depth)
}

// objectOf returns the object an identifier or a qualified identifier refers
// to.
func (flow *pathFlow) objectOf(e ast.Expr) types.Object {
	switch e := e.(type) {
	case *ast.Ident:
		return flow.info.ObjectOf(e)
	case *ast.SelectorExpr:
		return flow.info.ObjectOf(e.Sel)
	}
	return nil
}

// variablePlace places what a variable holds: every value the package
// assigns to it, every element of what a range runs over into it, and, for a
// parameter, every argument a call of its function passes.
func (flow *pathFlow) variablePlace(v *types.Var, depth int) place {
	if slot, ok := flow.params[v]; ok {
		calls := flow.calls[slot.fn]
		if len(calls) == 0 {
			return placeElsewhere
		}
		result := placeUnset
		for _, call := range calls {
			if slot.index >= len(call.Args) {
				return placeElsewhere
			}
			result = either(result, flow.place(call.Args[slot.index], depth))
		}
		return result
	}
	values, ok := flow.assigned[v]
	if !ok {
		return placeElsewhere
	}
	result := placeUnset
	for _, value := range values {
		if flow.ranged[value] {
			result = either(result, flow.elementPlace(value, depth))
			continue
		}
		result = either(result, flow.place(value, depth))
	}
	return result
}

// elementPlace places the elements of the slice or array e names: those of
// its composite literal, followed back through the variables holding it.
func (flow *pathFlow) elementPlace(e ast.Expr, depth int) place {
	if depth > maxFlowDepth {
		return placeElsewhere
	}
	switch e := ast.Unparen(e).(type) {
	case *ast.CompositeLit:
		if len(e.Elts) == 0 {
			return placeElsewhere
		}
		result := placeUnset
		for _, element := range e.Elts {
			result = either(result, flow.place(element, depth+1))
		}
		return result
	case *ast.Ident, *ast.SelectorExpr:
		v, ok := flow.objectOf(e).(*types.Var)
		if !ok {
			return placeElsewhere
		}
		values, ok := flow.assigned[v]
		if !ok || len(values) == 0 {
			return placeElsewhere
		}
		result := placeUnset
		for _, value := range values {
			result = either(result, flow.elementPlace(value, depth+1))
		}
		return result
	}
	return placeElsewhere
}

// resultPlace places the index-th result of call: the paths the standard
// library builds from their parts, the directories the test makes, the file
// runtime.Caller reports, and what a function of the package returns.
func (flow *pathFlow) resultPlace(e ast.Expr, index int, depth int) place {
	call, ok := ast.Unparen(e).(*ast.CallExpr)
	if !ok {
		return placeElsewhere
	}
	fn, ok := flow.callee(call).(*types.Func)
	if !ok {
		return placeElsewhere
	}
	if fn.Pkg() != nil {
		switch fn.Pkg().Path() + "." + fn.Name() {
		case "path/filepath.Join", "path.Join":
			result := placeKept
			for i, arg := range call.Args {
				part := flow.place(arg, depth)
				if i == 0 {
					result = part
					continue
				}
				result = joinPlaces(result, part)
			}
			return result
		case "path/filepath.Dir", "path.Dir":
			switch inner := flow.place(call.Args[0], depth); inner {
			case placeCallerFile:
				return placeOwnDir
			case placeOwnDir:
				return placeElsewhere
			default:
				return inner
			}
		case "path/filepath.Clean", "path/filepath.Abs", "path/filepath.FromSlash", "path/filepath.ToSlash", "path.Clean":
			if index != 0 {
				return placeKept
			}
			return flow.place(call.Args[0], depth)
		case "os.MkdirTemp", "os.TempDir":
			return placeKept
		case "runtime.Caller":
			if index == 1 {
				return placeCallerFile
			}
			return placeKept
		}
	}
	if fn.Name() == "TempDir" && isTestingType(fn) {
		return placeKept
	}
	returns, declared := flow.returns[fn]
	if !declared || len(returns) == 0 {
		return placeElsewhere
	}
	result := placeUnset
	for _, ret := range returns {
		switch {
		case index < len(ret.Results) && len(ret.Results) > 1:
			result = either(result, flow.place(ret.Results[index], depth))
		case len(ret.Results) == 1:
			if index == 0 {
				result = either(result, flow.place(ret.Results[0], depth))
			} else {
				result = either(result, flow.resultPlace(ret.Results[0], index, depth))
			}
		default:
			return placeElsewhere
		}
	}
	return result
}

// isTestingType reports whether fn is a method of a type of package testing,
// where TempDir is the directory a test makes.
func isTestingType(fn *types.Func) bool {
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return false
	}
	named, ok := derefNamed(sig.Recv().Type())
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "testing"
}

// literalPlace places a path spelt out: one with a ".." element, or an
// absolute one, can lie anywhere, and any other stays inside the package.
func literalPlace(path string) place {
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") || climbsOut(path) {
		return placeElsewhere
	}
	return placeKept
}

// climbsOut reports whether path has a ".." element, which leaves the
// directory the path starts from, with either separator.
func climbsOut(path string) bool {
	return slices.Contains(strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' }), "..")
}

// joinPlaces places a path one part of which follows another: below the
// package's own directory or the test file's place stays inside, and a part
// that leaves takes the whole path along.
func joinPlaces(head, tail place) place {
	switch {
	case head == placeElsewhere || tail == placeElsewhere:
		return placeElsewhere
	case tail == placeCallerFile || tail == placeOwnDir:
		return placeElsewhere
	default:
		return placeKept
	}
}

// either places a path that may come from a or from b: where both agree,
// that place, and where they differ, the package's own directory when both
// stay inside it, which its parent leaves (see resultPlace), or a place
// anywhere otherwise.
func either(a, b place) place {
	switch {
	case a == placeUnset:
		return b
	case a == b:
		return a
	case a == placeElsewhere || b == placeElsewhere:
		return placeElsewhere
	default:
		return placeOwnDir
	}
}
