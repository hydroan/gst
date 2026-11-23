package main

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/hydroan/gst/internal/goast"
	"golang.org/x/tools/go/packages"
)

// The transaction of the public database package, by the path of the package
// under the module and the names of its functions, each taking the function
// it runs as its last argument: the error one returns is whatever that
// function returns.
const (
	msgReachDatabasePkg   = "database"
	msgReachTransaction   = "Transaction"
	msgReachTransactionOn = "TransactionOn"
)

// The test of an error against another, which reads nothing an error
// carries, by the package declaring it and its name.
const (
	msgReachErrorsPkg = "github.com/cockroachdb/errors"
	msgReachIs        = "Is"
)

// msgReachFlow is a body an error flows out of: a function declaration, or
// the function literal handed to a transaction, whose error is the one the
// transaction returns to the body calling it.
type msgReachFlow struct {
	pkg *packages.Package
	// name names the body in a report: the function, with its receiver, and
	// for a literal the function declaring it.
	name string
	// built are the service errors the body returns as it builds them: one
	// it keeps in a variable or hands to a call is not followed, and counts
	// as reaching a client.
	built []*ast.CallExpr
	// handed are the bodies the error of this one may leave as it stands:
	// the ones calling it where the error is not seen to stop.
	handed []*msgReachFlow
	// answers reports a body whose error may reach a transport without
	// passing through a body the check sees: one nothing of the module
	// calls, one named as a value, one a project may call, or one called
	// where the check does not follow the error.
	answers bool
}

// checkMsgReach holds every service error the framework builds to a place it
// can reach a client from as it stands. A transport answers a request with
// the outermost service error of the error it is handed (see
// errorConstructors), so one built in a function whose every caller drops
// what it returns, or wraps it in a service error of its own, is never the
// one answered: its status and its sentence are read by nobody, and the
// sentence stands among the ones a client is answered with, translations
// and all, without being one. Such an error is a plain one, kept for the
// log.
//
// The check follows the error a body returns through the calls of the
// module. A body answers a client when nothing of the module calls it, the
// methods and hooks the framework calls among them, when it is named as a
// value, or when it is exported by a package a project imports. A call hands
// the error of the body it calls to the body making it unless the error is
// seen to stop there: the call is assigned to an error variable, the next
// statement tests that variable against nil and always leaves (see
// goast.NilChecked and goast.AlwaysLeaves), and nothing reads what the error
// carries or takes it out of the function as it stands (see msgReachKept).
// The error is dropped, compared, handed to errors.Is, or handed to the
// constructor of a service error as its cause, which the constructor does
// not read. Any other use may read it: errors.As hands the service error
// inside to code that rebuilds the answer from its status and its sentence,
// and a call the check knows nothing of may do the same. The function
// literal a transaction runs is a body of its own, called where the
// transaction is. Whatever the check does not see through, a call made any
// other way and every other function literal, counts as answering: the check
// misses an error that reaches no client rather than report one that
// reaches one.
//
// Reported is each service error a body returns as it builds it when the
// error of that body reaches no answering one. Test files, which only the
// test variants of a package hold, are left alone.
func checkMsgReach(root string, pkgs []*packages.Package) ([]violation, error) {
	if len(pkgs) == 0 || pkgs[0].Module == nil {
		return nil, nil
	}
	module := pkgs[0].Module.Path
	plain := plainPackages(pkgs)
	// constructors holds the constructors of the service error and the
	// variables a package forwards one in. A function handing a sentence of
	// its own on to one is none: what it does with the cause it is handed
	// beside is its own business.
	constructors := make(map[types.Object]bool)
	for obj := range sentenceSinks(module, plain, errorConstructors) {
		if fn, ok := obj.(*types.Func); ok && (fn.Pkg() == nil || fn.Pkg().Path() != module+"/"+sentenceErrorsPkg) {
			continue
		}
		constructors[obj] = true
	}
	if len(constructors) == 0 {
		return nil, nil
	}

	flows := make(map[*types.Func]*msgReachFlow)
	bodies := make(map[*msgReachFlow]*ast.BlockStmt)
	for _, p := range plain {
		for _, file := range p.Syntax {
			for _, decl := range file.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Body == nil {
					continue
				}
				fn, ok := p.TypesInfo.Defs[fd.Name].(*types.Func)
				if !ok {
					continue
				}
				flow := &msgReachFlow{pkg: p, name: fn.Name()}
				if recv := fn.Signature().Recv(); recv != nil {
					if named, ok := derefNamed(recv.Type()); ok {
						flow.name = named.Obj().Name() + "." + fn.Name()
					}
				}
				flows[fn] = flow
				bodies[flow] = fd.Body
			}
		}
	}
	// flowOf returns the body of the module function a call calls, and nil
	// for a call of anything else.
	flowOf := func(p *packages.Package, call *ast.CallExpr) *msgReachFlow {
		fn, ok := usedObject(p.TypesInfo, call.Fun).(*types.Func)
		if !ok {
			return nil
		}
		return flows[fn.Origin()]
	}
	// transactionBody returns the function literal a call hands to a
	// transaction of the database package, and nil for any other call.
	transactionBody := func(p *packages.Package, call *ast.CallExpr) *ast.FuncLit {
		fn, ok := usedObject(p.TypesInfo, call.Fun).(*types.Func)
		if !ok || fn.Pkg() == nil || fn.Pkg().Path() != module+"/"+msgReachDatabasePkg || len(call.Args) == 0 {
			return nil
		}
		if fn.Name() != msgReachTransaction && fn.Name() != msgReachTransactionOn {
			return nil
		}
		literal, _ := ast.Unparen(call.Args[len(call.Args)-1]).(*ast.FuncLit)
		return literal
	}

	// called and valued hold the module functions a call names and the ones
	// named any other way, by which a body nothing calls and one handed
	// around as a value are told.
	called, valued := make(map[*types.Func]bool), make(map[*types.Func]bool)
	for _, p := range plain {
		for _, file := range p.Syntax {
			callees := make(map[*ast.Ident]bool)
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch fun := ast.Unparen(call.Fun).(type) {
				case *ast.Ident:
					callees[fun] = true
				case *ast.SelectorExpr:
					callees[fun.Sel] = true
				case *ast.IndexExpr:
					if id, ok := fun.X.(*ast.Ident); ok {
						callees[id] = true
					} else if sel, ok := fun.X.(*ast.SelectorExpr); ok {
						callees[sel.Sel] = true
					}
				case *ast.IndexListExpr:
					if id, ok := fun.X.(*ast.Ident); ok {
						callees[id] = true
					} else if sel, ok := fun.X.(*ast.SelectorExpr); ok {
						callees[sel.Sel] = true
					}
				}
				if fn, ok := usedObject(p.TypesInfo, call.Fun).(*types.Func); ok {
					called[fn.Origin()] = true
				}
				return true
			})
			ast.Inspect(file, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok && !callees[id] {
					if fn, ok := p.TypesInfo.Uses[id].(*types.Func); ok {
						valued[fn.Origin()] = true
					}
				}
				return true
			})
		}
	}
	for _, p := range plain {
		for _, file := range p.Syntax {
			for _, decl := range file.Decls {
				if _, ok := decl.(*ast.FuncDecl); ok {
					continue
				}
				// A call made as the package initializes hands its error
				// where the check does not follow it.
				ast.Inspect(decl, func(n ast.Node) bool {
					if call, ok := n.(*ast.CallExpr); ok {
						if callee := flowOf(p, call); callee != nil {
							callee.answers = true
						}
					}
					return true
				})
			}
		}
	}
	for fn, flow := range flows {
		rel := strings.TrimPrefix(fn.Pkg().Path(), module)
		public := fn.Exported() && fn.Pkg().Name() != "main" && !strings.Contains(rel+"/", "/internal/")
		flow.answers = flow.answers || !called[fn] || valued[fn] || public
	}

	// follow reads the statements of a body: the service errors it builds,
	// the calls handing it the error of another body, and the transactions
	// whose function literal is a body called there. The calls of a function
	// literal that is no transaction's hand their error where the check does
	// not follow it.
	var follow func(flow *msgReachFlow, body *ast.BlockStmt)
	follow = func(flow *msgReachFlow, body *ast.BlockStmt) {
		p := flow.pkg
		stopped := msgReachStopped(p, body, constructors)
		ast.Inspect(body, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.FuncLit:
				ast.Inspect(node.Body, func(inner ast.Node) bool {
					if call, ok := inner.(*ast.CallExpr); ok {
						if callee := flowOf(p, call); callee != nil {
							callee.answers = true
						}
					}
					return true
				})
				return false
			case *ast.ReturnStmt:
				for _, result := range node.Results {
					if call, ok := ast.Unparen(result).(*ast.CallExpr); ok && constructors[usedObject(p.TypesInfo, call.Fun)] {
						flow.built = append(flow.built, call)
					}
				}
			case *ast.CallExpr:
				if literal := transactionBody(p, node); literal != nil {
					transaction := &msgReachFlow{pkg: p, name: "the transaction of " + flow.name}
					if !stopped[node] {
						transaction.handed = append(transaction.handed, flow)
					}
					bodies[transaction] = literal.Body
					follow(transaction, literal.Body)
					for _, arg := range node.Args[:len(node.Args)-1] {
						ast.Inspect(arg, func(inner ast.Node) bool {
							if call, ok := inner.(*ast.CallExpr); ok {
								if callee := flowOf(p, call); callee != nil {
									callee.handed = append(callee.handed, flow)
								}
							}
							return true
						})
					}
					return false
				}
				if callee := flowOf(p, node); callee != nil && !stopped[node] {
					callee.handed = append(callee.handed, flow)
				}
			}
			return true
		})
	}
	declared := make([]*msgReachFlow, 0, len(flows))
	for _, flow := range flows {
		declared = append(declared, flow)
	}
	for _, flow := range declared {
		follow(flow, bodies[flow])
	}

	// reaches settles, body by body, whether its error may reach an
	// answering one: a body handing its error only to bodies still being
	// settled, a ring of calls, reaches none through them.
	reaches := make(map[*msgReachFlow]bool)
	for changed := true; changed; {
		changed = false
		for flow := range bodies {
			if reaches[flow] {
				continue
			}
			if flow.answers || slices.ContainsFunc(flow.handed, func(to *msgReachFlow) bool { return reaches[to] }) {
				reaches[flow] = true
				changed = true
			}
		}
	}

	var found []violation
	for flow := range bodies {
		if reaches[flow] {
			continue
		}
		for _, call := range flow.built {
			pos := flow.pkg.Fset.Position(call.Pos())
			file := relative(root, pos.Filename)
			found = append(found, violation{
				File: file,
				Message: fmt.Sprintf("Service error at %s:%d never reaches a client: whichever way the error of %s goes, it is dropped or wrapped in another service error, the one the client is answered with; build a plain error instead",
					file, pos.Line, flow.name),
			})
		}
	}
	slices.SortFunc(found, func(a, b violation) int {
		return cmp.Or(strings.Compare(a.File, b.File), strings.Compare(a.Message, b.Message))
	})
	return found, nil
}

// msgReachStopped returns the calls of body, the body of a function, whose
// error is seen to stop in it: a call that is all an assignment assigns, to
// an error variable of the function among its targets, where the statement
// after it, or the if statement the assignment opens, tests that variable
// against nil with a body that always leaves and keeps the error from
// leaving with it (see msgReachKept). The statements of function literals
// are not read.
func msgReachStopped(p *packages.Package, body *ast.BlockStmt, constructors map[types.Object]bool) map[*ast.CallExpr]bool {
	stopped := make(map[*ast.CallExpr]bool)
	errorType := types.Universe.Lookup("error").Type()
	// function is the scope of the function, its parameters and results
	// among it: a variable declared outside it is read where the check does
	// not look.
	function := p.Types.Scope().Innermost(body.Pos())
	check := func(assign *ast.AssignStmt, test *ast.IfStmt) {
		if assign == nil || test == nil || len(assign.Rhs) != 1 {
			return
		}
		call, ok := ast.Unparen(assign.Rhs[0]).(*ast.CallExpr)
		if !ok {
			return
		}
		// variable is the error the assignment sets, and declared tells
		// whether the assignment declares it.
		var variable types.Object
		declared := false
		for _, target := range assign.Lhs {
			id, ok := target.(*ast.Ident)
			if !ok || id.Name == "_" {
				continue
			}
			if obj := p.TypesInfo.ObjectOf(id); obj != nil && types.Identical(obj.Type(), errorType) {
				variable, declared = obj, p.TypesInfo.Defs[id] != nil
			}
		}
		tested := goast.NilChecked(test)
		if variable == nil || tested == nil || p.TypesInfo.ObjectOf(tested) != variable {
			return
		}
		if function == nil || !function.Contains(variable.Pos()) {
			return
		}
		if goast.AlwaysLeaves(test.Body.List) && msgReachKept(p, body, test, variable, declared, constructors) {
			stopped[call] = true
		}
	}
	list := func(stmts []ast.Stmt) {
		for i, stmt := range stmts {
			test, ok := stmt.(*ast.IfStmt)
			if !ok {
				continue
			}
			if assign, ok := test.Init.(*ast.AssignStmt); ok {
				check(assign, test)
			} else if test.Init == nil && i > 0 {
				assign, _ := stmts[i-1].(*ast.AssignStmt)
				check(assign, test)
			}
		}
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.BlockStmt:
			list(node.List)
		case *ast.CaseClause:
			list(node.Body)
		case *ast.CommClause:
			list(node.Body)
		}
		return true
	})
	return stopped
}

// msgReachKept reports whether the error a call left in variable, which the
// if statement test finds set, leaves the function under a service error or
// not at all, with nothing reading what it carries; body is the body of the
// function.
//
//   - The body of test gives the function no way on with the error: it holds
//     no return statement naming no result, which returns the named results
//     as they are, no goto, and a break or a continue only where declared
//     tells the assignment declares the variable, which is gone then where
//     the statement lands.
//   - Every use of the variable in the body of test, and in a function
//     literal of the function, which may run while the variable holds the
//     error, is one msgReachUseKept accepts.
func msgReachKept(p *packages.Package, body *ast.BlockStmt, test *ast.IfStmt, variable types.Object, declared bool, constructors map[types.Object]bool) bool {
	kept := true
	var stack []ast.Node
	// inTest reports whether the node at the end of stack lies in the body
	// of test.
	inTest := func() bool { return slices.Contains(stack, ast.Node(test.Body)) }
	ast.Inspect(body, func(n ast.Node) bool {
		if !kept {
			return false
		}
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		switch node := n.(type) {
		case *ast.ReturnStmt:
			kept = len(node.Results) > 0 || !inTest()
		case *ast.BranchStmt:
			kept = declared && node.Tok != token.GOTO || !inTest()
		case *ast.Ident:
			if p.TypesInfo.Uses[node] != variable {
				break
			}
			literal := slices.ContainsFunc(stack, func(above ast.Node) bool {
				_, ok := above.(*ast.FuncLit)
				return ok
			})
			if literal || inTest() {
				kept = msgReachUseKept(p, stack, constructors)
			}
		}
		return true
	})
	return kept
}

// msgReachUseKept reports whether the use of an error variable at the end of
// stack, the nodes from the body of a function down to the identifier, reads
// nothing the error carries and leaves it nowhere but under a service error:
// the variable is compared with another value, handed to errors.Is, or
// handed to a constructor, one of constructors, which keeps it as the cause
// of the service error it builds. A method of the error, another call or an
// assignment may each carry its status or its sentence on.
func msgReachUseKept(p *packages.Package, stack []ast.Node, constructors map[types.Object]bool) bool {
	switch parent := stack[len(stack)-2].(type) {
	case *ast.BinaryExpr:
		return parent.Op == token.EQL || parent.Op == token.NEQ
	case *ast.CallExpr:
		callee := usedObject(p.TypesInfo, parent.Fun)
		if constructors[callee] {
			return true
		}
		fn, ok := callee.(*types.Func)
		return ok && fn.Pkg() != nil && fn.Pkg().Path() == msgReachErrorsPkg && fn.Name() == msgReachIs
	}
	return false
}
