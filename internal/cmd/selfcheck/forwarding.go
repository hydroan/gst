package main

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"golang.org/x/tools/go/packages"
)

// maxForwardingLead is how many statements a function may run before it
// forwards to a function nothing else uses, and still be a shell to merge with
// that function; thinLead says which statements qualify.
const maxForwardingLead = 1

// forwarding is a thin forwarding function the check reports.
type forwarding struct {
	// pos is where the forwarding function is declared.
	pos token.Position
	// message says what is wrong and how to fix it.
	message string
}

// checkForwarding reports the thin forwarding functions under root. A
// function forwards when its last statement passes its own parameters,
// unchanged and in order, to another function that takes and returns exactly
// the types it does; a method may pass its receiver along too, as the receiver
// of the call or its first argument. The forwarding layer can go, and is
// reported, in two cases:
//
//   - the forwarding function does nothing else, is unexported and has a
//     single use, which can call the other function itself;
//   - the function it forwards to is unexported, declared in the same package
//     and used by nothing else, so its body can move into the forwarding
//     function, which may first run a statement of its own as long as that
//     is straight-line work (see thinLead).
//
// Only uses inside the package are counted, tests included, which is why only
// unexported functions are judged. Left alone are a method whose name an
// interface of its package declares, since a call through the interface is a
// use the check cannot count; a function whose name appears in a file the
// build leaves out; and anything declared in a generated file.
func checkForwarding(root string, pkgs []*packages.Package) ([]violation, error) {
	withTests := make(map[string]bool)
	for _, p := range pkgs {
		if isInternalTestVariant(p) {
			withTests[p.PkgPath] = true
		}
	}

	var found []forwarding
	for _, p := range pkgs {
		if p.ForTest == "" && withTests[p.PkgPath] {
			// The internal test variant holds the same files and the tests.
			continue
		}
		inPkg, err := forwardingIn(root, p)
		if err != nil {
			return nil, err
		}
		found = append(found, inPkg...)
	}
	slices.SortFunc(found, func(a, b forwarding) int {
		return cmp.Or(cmp.Compare(a.pos.Filename, b.pos.Filename), cmp.Compare(a.pos.Line, b.pos.Line))
	})

	var violations []violation
	for _, f := range found {
		violations = append(violations, violation{File: relative(root, f.pos.Filename), Message: f.message})
	}
	return violations, nil
}

// forwardingIn finds the thin forwarding functions declared in p.
func forwardingIn(root string, p *packages.Package) ([]forwarding, error) {
	uses := make(map[*types.Func][]token.Pos)
	for id, obj := range p.TypesInfo.Uses {
		if fn, ok := obj.(*types.Func); ok && fn.Pkg() == p.Types {
			uses[fn.Origin()] = append(uses[fn.Origin()], id.Pos())
		}
	}
	ignored, err := ignoredNames(p)
	if err != nil {
		return nil, err
	}
	viaInterface := interfaceMethods(p)
	generated := make(map[string]bool)
	for i, f := range p.Syntax {
		if ast.IsGenerated(f) {
			generated[p.CompiledGoFiles[i]] = true
		}
	}
	// judged reports whether every use of fn is one the check counts.
	judged := func(fn *types.Func) bool {
		if fn.Exported() || ignored[fn.Name()] {
			return false
		}
		return fn.Signature().Recv() == nil || !viaInterface[fn.Name()]
	}
	at := func(pos token.Pos) string {
		position := p.Fset.Position(pos)
		return fmt.Sprintf("%s:%d", relative(root, position.Filename), position.Line)
	}

	var found []forwarding
	for i, f := range p.Syntax {
		if generated[p.CompiledGoFiles[i]] {
			continue
		}
		for _, decl := range f.Decls {
			d, ok := decl.(*ast.FuncDecl)
			if !ok || d.Body == nil {
				continue
			}
			fn, ok := p.TypesInfo.Defs[d.Name].(*types.Func)
			if !ok {
				continue
			}
			to, lead, ok := forwardedTo(p, d, fn)
			if !ok {
				continue
			}
			kind := "Function"
			if fn.Signature().Recv() != nil {
				kind = "Method"
			}
			toUses := uses[to]
			switch {
			case len(lead) == 0 && judged(fn) && len(uses[fn]) == 1:
				found = append(found, forwarding{
					pos: p.Fset.Position(d.Name.Pos()),
					message: fmt.Sprintf("%s '%s' at %s only forwards to %s and has one use, at %s: call %s there instead",
						kind, funcName(p, fn), at(d.Name.Pos()), funcName(p, to), at(uses[fn][0]), funcName(p, to)),
				})
			case thinLead(lead) && to.Pkg() == p.Types && judged(to) &&
				!generated[p.Fset.Position(to.Pos()).Filename] &&
				len(toUses) == 1 && d.Pos() <= toUses[0] && toUses[0] < d.End():
				found = append(found, forwarding{
					pos: p.Fset.Position(d.Name.Pos()),
					message: fmt.Sprintf("%s '%s' at %s forwards to %s, which nothing else uses: merge %s into %s",
						kind, funcName(p, fn), at(d.Name.Pos()), funcName(p, to), funcName(p, to), funcName(p, fn)),
				})
			}
		}
	}
	return found, nil
}

// forwardedTo reports the function d forwards to, and the statements d runs
// before it does. d forwards when its last statement calls that function with
// d's own parameters, unchanged and in order, and gives back what the call
// returns, and the call takes and returns exactly the types d does, so taking
// d away changes no conversion. A method may also pass its receiver, as the
// receiver of the call or its first argument.
func forwardedTo(p *packages.Package, d *ast.FuncDecl, fn *types.Func) (*types.Func, []ast.Stmt, bool) {
	stmts := d.Body.List
	if len(stmts) == 0 {
		return nil, nil, false
	}
	sig := fn.Signature()
	var call *ast.CallExpr
	switch s := stmts[len(stmts)-1].(type) {
	case *ast.ReturnStmt:
		if len(s.Results) == 1 {
			call, _ = ast.Unparen(s.Results[0]).(*ast.CallExpr)
		}
	case *ast.ExprStmt:
		if sig.Results().Len() == 0 {
			call, _ = ast.Unparen(s.X).(*ast.CallExpr)
		}
	}
	if call == nil {
		return nil, nil, false
	}
	recv := sig.Recv()
	to, onRecv := callee(p, call.Fun, recv)
	if to == nil || to.Origin() == fn {
		return nil, nil, false
	}

	// want holds the parameter types the call has to take.
	var want []types.Type
	args := call.Args
	if recv != nil && !onRecv && len(args) == sig.Params().Len()+1 && usesVar(p, args[0], recv) {
		want = append(want, recv.Type())
		args = args[1:]
	}
	if len(args) != sig.Params().Len() {
		return nil, nil, false
	}
	for i, arg := range args {
		param := sig.Params().At(i)
		if !usesVar(p, arg, param) {
			return nil, nil, false
		}
		want = append(want, param.Type())
	}

	called, ok := p.TypesInfo.TypeOf(call.Fun).(*types.Signature)
	if !ok || called.Variadic() != sig.Variadic() || call.Ellipsis.IsValid() != sig.Variadic() ||
		called.Params().Len() != len(want) || !types.Identical(called.Results(), sig.Results()) {
		return nil, nil, false
	}
	for i, t := range want {
		if !types.Identical(called.Params().At(i).Type(), t) {
			return nil, nil, false
		}
	}
	if onRecv && !types.Identical(to.Signature().Recv().Type(), recv.Type()) {
		return nil, nil, false
	}
	return to.Origin(), stmts[:len(stmts)-1], true
}

// thinLead reports whether the statements a function runs before it forwards
// leave it a thin shell around the function it forwards to: no more than
// maxForwardingLead of them, each straight-line work, which is an assignment,
// an increment or decrement, or a plain call, with no function literal inside.
// A branch, a loop or a closure is work of its own, which the function it
// forwards to does not need to absorb.
func thinLead(lead []ast.Stmt) bool {
	if len(lead) > maxForwardingLead {
		return false
	}
	for _, s := range lead {
		switch s := s.(type) {
		case *ast.AssignStmt, *ast.IncDecStmt:
		case *ast.ExprStmt:
			if _, ok := ast.Unparen(s.X).(*ast.CallExpr); !ok {
				return false
			}
		default:
			return false
		}
		closure := false
		ast.Inspect(s, func(n ast.Node) bool {
			_, isLiteral := n.(*ast.FuncLit)
			closure = closure || isLiteral
			return !closure
		})
		if closure {
			return false
		}
	}
	return true
}

// callee returns the function or method fun names, and whether it is a method
// called on recv, the receiver of the calling method. It returns nil for
// anything else: a function value, a conversion, a builtin, a method of some
// other value, or an instantiation spelled out in the call.
func callee(p *packages.Package, fun ast.Expr, recv *types.Var) (*types.Func, bool) {
	switch fun := ast.Unparen(fun).(type) {
	case *ast.Ident:
		if fn, ok := p.TypesInfo.Uses[fun].(*types.Func); ok && fn.Signature().Recv() == nil {
			return fn, false
		}
	case *ast.SelectorExpr:
		x, ok := fun.X.(*ast.Ident)
		if !ok {
			return nil, false
		}
		switch obj := p.TypesInfo.Uses[x].(type) {
		case *types.PkgName:
			if fn, ok := p.TypesInfo.Uses[fun.Sel].(*types.Func); ok {
				return fn, false
			}
		case *types.Var:
			if recv == nil || obj != recv {
				return nil, false
			}
			if sel := p.TypesInfo.Selections[fun]; sel != nil && sel.Kind() == types.MethodVal {
				if fn, ok := sel.Obj().(*types.Func); ok {
					return fn, true
				}
			}
		}
	}
	return nil, false
}

// usesVar reports whether e is nothing but a use of v.
func usesVar(p *packages.Package, e ast.Expr, v *types.Var) bool {
	id, ok := ast.Unparen(e).(*ast.Ident)
	return ok && p.TypesInfo.Uses[id] == v
}

// interfaceMethods returns the names of the methods the interfaces in p
// declare: a method of p named like one of them may be called through an
// interface, a use no identifier records.
func interfaceMethods(p *packages.Package) map[string]bool {
	names := make(map[string]bool)
	for _, tv := range p.TypesInfo.Types {
		if tv.Type == nil {
			continue
		}
		if iface, ok := tv.Type.Underlying().(*types.Interface); ok {
			for m := range iface.Methods() {
				names[m.Name()] = true
			}
		}
	}
	return names
}

// ignoredNames returns every identifier in the Go files of p's directory that
// the build leaves out: a use there is one the check cannot see, so a function
// named like any of them is not judged.
func ignoredNames(p *packages.Package) (map[string]bool, error) {
	names := make(map[string]bool)
	fset := token.NewFileSet()
	for _, path := range p.IgnoredFiles {
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, errors.Wrapf(err, "parse %s", path)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				names[id.Name] = true
			}
			return true
		})
	}
	return names, nil
}

// funcName names fn the way a report refers to it from p: Type.name for a
// method, pkg.Name for a function of another package, and Name for a function
// of p.
func funcName(p *packages.Package, fn *types.Func) string {
	if recv := fn.Signature().Recv(); recv != nil {
		if named, ok := derefNamed(recv.Type()); ok {
			return named.Obj().Name() + "." + fn.Name()
		}
		return fn.Name()
	}
	if fn.Pkg() != p.Types {
		return fn.Pkg().Name() + "." + fn.Name()
	}
	return fn.Name()
}
