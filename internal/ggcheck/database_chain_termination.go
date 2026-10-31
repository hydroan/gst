package ggcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"

	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/goast"
)

// databaseTerminalMethods lists the gst.Database methods that finish an
// operation chain. TestDatabaseChainMethodSetsMatchDatabaseInterfaces guards this
// set against drifting from the interface declaration.
var databaseTerminalMethods = map[string]bool{
	"Create":     true,
	"Delete":     true,
	"Update":     true,
	"Upsert":     true,
	"UpdateByID": true,
	"List":       true,
	"Get":        true,
	"First":      true,
	"Last":       true,
	"Take":       true,
	"Count":      true,
}

// databaseChainMethods lists the gst.DatabaseOption methods that keep the
// chain open. TestDatabaseChainMethodSetsMatchDatabaseInterfaces guards this set
// against drifting from the interface declaration.
var databaseChainMethods = map[string]bool{
	"WithQuery":        true,
	"WithCursor":       true,
	"WithSelect":       true,
	"WithLock":         true,
	"WithBatchSize":    true,
	"WithPagination":   true,
	"WithLimit":        true,
	"WithOrder":        true,
	"WithExpand":       true,
	"WithPurge":        true,
	"WithAllowMissing": true,
	"WithDeleted":      true,
	"WithReplica":      true,
	"WithDryRun":       true,
	"WithoutHook":      true,
}

// DatabaseChainTermination keeps every database.Database chain to one
// terminal operation.
var DatabaseChainTermination = Check{
	Name: "Database chain termination",
	Rule: "a database.Database chain runs one terminal operation: written inline it ends in one, held in a variable it is terminated once, not again and not inside a loop it was bound outside of",
	run:  checkDatabaseChainTermination,
}

// checkDatabaseChainTermination reports a database.Database chain that runs
// no operation or more than one. Database implementations share an
// underlying GORM session, so a second operation run on a chain through the
// variable holding it carries the first operation's state into the second.
// A chain may be held in a variable, passed on or returned, as long as it is
// terminated once: a second termination is reported unless the two sit in
// branches that exclude each other, an if and its else or the cases of a
// switch, and so is a termination inside a loop the variable was bound
// outside of, which repeats it. A chain written inline that reaches no
// terminal operation runs nothing, and is reported as well. Test files are
// not checked, matching the other project checks.
func checkDatabaseChainTermination(ignore gghelper.ProjectIgnore) []string {
	var violations []string

	err := ignore.Walk(".", func(path string, info os.FileInfo) error {
		if info.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		violations = append(violations, checkFileDatabaseChains(path)...)
		return nil
	})
	if err != nil {
		violations = append(violations, fmt.Sprintf("walking project directory: %v", err))
	}

	return violations
}

// checkFileDatabaseChains reports the chains of one file (see
// checkDatabaseChainTermination), in the order of their lines.
func checkFileDatabaseChains(filePath string) []string {
	dbNames, ok := gstDatabaseImportNames(filePath)
	if !ok {
		return nil
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil
	}

	parents := nodeParents(file)
	relPath := gghelper.RelativePath(filePath)

	type violation struct {
		line    int
		message string
	}
	var found []violation
	report := func(at ast.Node, message string) {
		found = append(found, violation{line: fset.Position(at.Pos()).Line, message: message})
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			if isDatabaseChainStart(node, dbNames) && inlineChainUnterminated(node, parents) {
				report(node, "database.Database chain never calls a terminal operation such as List, Get, or Create")
			}
		case *ast.FuncDecl:
			if node.Body != nil {
				reportHeldChains(node.Body, dbNames, parents, fset, report)
			}
		case *ast.FuncLit:
			reportHeldChains(node.Body, dbNames, parents, fset, report)
		}
		return true
	})
	slices.SortStableFunc(found, func(a, b violation) int { return a.line - b.line })

	violations := make([]string, 0, len(found))
	for _, v := range found {
		violations = append(violations, fmt.Sprintf("%s:%d: %s", relPath, v.line, v.message))
	}
	return violations
}

// isDatabaseChainStart reports whether call is a generic database.Database
// instantiation call that starts an operation chain, under the names dbNames
// resolves.
func isDatabaseChainStart(call *ast.CallExpr, dbNames goast.PackageNames) bool {
	switch fun := call.Fun.(type) {
	case *ast.IndexExpr:
		return dbNames.Refers(fun.X, "Database")
	case *ast.IndexListExpr:
		return dbNames.Refers(fun.X, "Database")
	}
	return false
}

// inlineChainUnterminated reports whether the chain started by anchor is a
// statement of its own that reaches no terminal operation: it follows the
// chain outward through the chain methods, and a chain that ends in a
// terminal method, or leaves the statement as a value, is terminated or
// held elsewhere.
func inlineChainUnterminated(anchor *ast.CallExpr, parents map[ast.Node]ast.Node) bool {
	node := ast.Node(anchor)
	for {
		parent := parents[node]
		if paren, ok := parent.(*ast.ParenExpr); ok {
			node = paren
			continue
		}
		sel, ok := parent.(*ast.SelectorExpr)
		if !ok || sel.X != node {
			break
		}
		call, ok := parents[sel].(*ast.CallExpr)
		if !ok || call.Fun != sel {
			// The method value leaves the statement, consume(db.List).
			return false
		}
		if databaseChainMethods[sel.Sel.Name] {
			node = call
			continue
		}
		// A terminal method finishes the chain. Methods unknown to both sets
		// are treated as terminal so interface additions cannot produce false
		// positives before the method sets catch up.
		return false
	}
	_, ok := parents[node].(*ast.ExprStmt)
	return ok
}

// heldChain is a chain a variable of a function body holds.
type heldChain struct {
	name  string
	bound ast.Node   // the statement binding the chain to the variable
	ends  []ast.Node // the terminations of the chain since it was bound
}

// reportHeldChains reports, through report, the chains the variables of body
// hold that are terminated again or inside a loop (see
// checkDatabaseChainTermination). A chain is held by the variable a start
// of one, or a chain method call on a held one, is assigned to, so two
// variables may hold the same chain; it is terminated by a terminal method
// called on it, and by being passed on, as an argument or a method value,
// or returned. Nested function literals hold chains of their own and are
// walked on their own.
func reportHeldChains(body *ast.BlockStmt, dbNames goast.PackageNames, parents map[ast.Node]ast.Node, fset *token.FileSet, report func(ast.Node, string)) {
	held := make(map[string]*heldChain)
	// heldRoot returns the chain expr holds through a variable: the
	// variable itself, a chain method called on it or a method value of it;
	// a terminal method called on it is the chain's end, not the chain.
	var heldRoot func(expr ast.Expr) *heldChain
	heldRoot = func(expr ast.Expr) *heldChain {
		switch e := expr.(type) {
		case *ast.ParenExpr:
			return heldRoot(e.X)
		case *ast.CallExpr:
			if sel, ok := e.Fun.(*ast.SelectorExpr); ok && databaseChainMethods[sel.Sel.Name] {
				return heldRoot(sel.X)
			}
		case *ast.SelectorExpr:
			return heldRoot(e.X)
		case *ast.Ident:
			return held[e.Name]
		}
		return nil
	}
	end := func(chain *heldChain, at ast.Node) {
		for _, prior := range chain.ends {
			if !exclusiveBranches(prior, at, parents) {
				report(at, fmt.Sprintf("database.Database chain held in %s is terminated again, first at line %d; a chain runs one operation, call database.Database again for the next", chain.name, fset.Position(prior.Pos()).Line))
				break
			}
		}
		if loopBetween(chain.bound, at, body, parents) {
			report(at, fmt.Sprintf("database.Database chain held in %s is terminated inside a loop it was bound outside of; a chain runs one operation, call database.Database inside the loop", chain.name))
		}
		chain.ends = append(chain.ends, at)
	}
	bind := func(name string, value ast.Expr, at ast.Node) {
		switch {
		case startsChain(value, dbNames):
			held[name] = &heldChain{name: name, bound: at}
		case heldRoot(value) != nil:
			held[name] = heldRoot(value)
		default:
			delete(held, name)
		}
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.AssignStmt:
			for i, value := range node.Rhs {
				if i >= len(node.Lhs) {
					break
				}
				if ident, ok := node.Lhs[i].(*ast.Ident); ok {
					bind(ident.Name, value, node)
				}
			}
		case *ast.DeclStmt:
			if gen, ok := node.Decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
				for _, spec := range gen.Specs {
					if valueSpec, ok := spec.(*ast.ValueSpec); ok {
						for i, value := range valueSpec.Values {
							if i < len(valueSpec.Names) {
								bind(valueSpec.Names[i].Name, value, node)
							}
						}
					}
				}
			}
		case *ast.CallExpr:
			if sel, ok := node.Fun.(*ast.SelectorExpr); ok && !databaseChainMethods[sel.Sel.Name] {
				if chain := heldRoot(sel.X); chain != nil {
					end(chain, node)
				}
			}
			for _, arg := range node.Args {
				if chain := heldRoot(arg); chain != nil {
					end(chain, arg)
				}
			}
		case *ast.ReturnStmt:
			for _, result := range node.Results {
				if chain := heldRoot(result); chain != nil {
					end(chain, result)
				}
			}
		}
		return true
	})
}

// startsChain reports whether expr is a chain started by database.Database,
// with or without chain methods called on it.
func startsChain(expr ast.Expr, dbNames goast.PackageNames) bool {
	for {
		switch e := expr.(type) {
		case *ast.ParenExpr:
			expr = e.X
		case *ast.CallExpr:
			if isDatabaseChainStart(e, dbNames) {
				return true
			}
			sel, ok := e.Fun.(*ast.SelectorExpr)
			if !ok || !databaseChainMethods[sel.Sel.Name] {
				return false
			}
			expr = sel.X
		default:
			return false
		}
	}
}

// exclusiveBranches reports whether a and b sit in branches that exclude
// each other: the body and the else of one if statement, or two cases of
// one switch or select.
func exclusiveBranches(a, b ast.Node, parents map[ast.Node]ast.Node) bool {
	above := make(map[ast.Node]ast.Node) // each ancestor of a to the child of it on the way to a
	for child, parent := a, parents[a]; parent != nil; child, parent = parent, parents[parent] {
		above[parent] = child
	}
	for child, parent := b, parents[b]; parent != nil; child, parent = parent, parents[parent] {
		fromA, ok := above[parent]
		if !ok {
			continue
		}
		if fromA == child {
			return false
		}
		switch lowest := parent.(type) {
		case *ast.IfStmt:
			return (fromA == lowest.Body || fromA == lowest.Else) && (child == lowest.Body || child == lowest.Else)
		case *ast.BlockStmt:
			switch parents[lowest].(type) {
			case *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
				return true
			}
		}
		return false
	}
	return false
}

// loopBetween reports whether the node at sits inside a loop of body that
// bound does not, so that a chain bound at bound and terminated there is
// terminated once per iteration.
func loopBetween(bound, at ast.Node, body *ast.BlockStmt, parents map[ast.Node]ast.Node) bool {
	for node := parents[at]; node != nil && node != body; node = parents[node] {
		switch node.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			if !isAncestor(node, bound, parents) {
				return true
			}
		}
	}
	return false
}

// isAncestor reports whether node is an ancestor of descendant.
func isAncestor(node, descendant ast.Node, parents map[ast.Node]ast.Node) bool {
	for n := parents[descendant]; n != nil; n = parents[n] {
		if n == node {
			return true
		}
	}
	return false
}

// nodeParents maps every AST node in file to its parent node.
func nodeParents(file *ast.File) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	var stack []ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if len(stack) > 0 {
			parents[n] = stack[len(stack)-1]
		}
		stack = append(stack, n)
		return true
	})
	return parents
}
