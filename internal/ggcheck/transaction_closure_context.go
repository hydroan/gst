package ggcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/goast"
)

// TransactionClosureContext holds the database calls inside a
// database.Transaction closure to the context the closure receives.
var TransactionClosureContext = Check{
	Name: "Transaction closure context",
	Rule: "database.Database chains and nested database.Transaction calls inside a database.Transaction closure must use the closure's context parameter",
	run:  checkTransactionClosureContext,
}

// checkTransactionClosureContext checks that inside a database.Transaction
// closure, every call into the database package — a chain, a select, a
// nested transaction, an after-commit registration, whatever the package's
// entry points are — takes the closure's own context. Passing any other
// context makes the operation silently escape the transaction, which is
// exactly the bug the context-injecting Transaction API exists to prevent,
// and an after-commit action registered on an escaped context runs at once
// instead of after the commit.
//
// The check is purely syntactic, mirroring checkDatabaseChainTermination. A
// context argument is flagged when it is a plain identifier other than the
// closure's parameter, or a detached context written at the call
// (context.Background, context.TODO), including through a derivation such as
// context.WithTimeout: a context derived from the closure's own passes, since
// it still carries the transaction. Anything else — a call result, a selector
// expression — is left alone.
func checkTransactionClosureContext(ignore gghelper.ProjectIgnore) []string {
	var violations []string

	err := ignore.Walk(".", func(path string, info os.FileInfo) error {
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
			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		violations = append(violations, checkFileTransactionClosures(path)...)
		return nil
	})
	if err != nil {
		violations = append(violations, fmt.Sprintf("walking project directory: %v", err))
	}

	return violations
}

// checkFileTransactionClosures reports database.Database chains and nested
// database.Transaction calls inside database.Transaction closures whose
// context argument is not the closure's context parameter.
func checkFileTransactionClosures(filePath string) []string {
	dbNames, ok := gstDatabaseImportNames(filePath)
	if !ok {
		return nil
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil
	}

	relPath := gghelper.RelativePath(filePath)

	var violations []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		closure, ctxParam, ok := transactionClosure(call, dbNames)
		if !ok {
			return true
		}
		inTransaction := contextsInTransaction(closure.Body, ctxParam)

		ast.Inspect(closure.Body, func(inner ast.Node) bool {
			innerCall, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}

			// A nested Transaction closure has its own context parameter and is
			// checked by the enclosing file walk; only its context argument is
			// this closure's responsibility, so its subtree is skipped here.
			if _, _, isNested := transactionClosure(innerCall, dbNames); isNested {
				if name, escapes := escapingContextIdent(innerCall.Args[0], inTransaction); escapes {
					pos := fset.Position(innerCall.Pos())
					violations = append(violations, fmt.Sprintf(
						"%s:%d: nested database.Transaction receives context %q inside a database.Transaction closure whose context parameter is %q; it starts a separate transaction instead of joining the enclosing one",
						relPath, pos.Line, name, ctxParam,
					))
				}
				return false
			}

			entry, ok := databaseEntryPointCall(innerCall, dbNames)
			if !ok {
				return true
			}
			if name, escapes := escapingContext(innerCall.Args[0], inTransaction); escapes {
				pos := fset.Position(innerCall.Pos())
				violations = append(violations, fmt.Sprintf(
					"%s:%d: database.%s uses context %s inside a database.Transaction closure whose context parameter is %q; the call escapes the transaction",
					relPath, pos.Line, entry, name, ctxParam,
				))
			}
			return true
		})
		return true
	})

	return violations
}

// transactionClosure reports whether call is database.Transaction with an
// inline closure, returning the closure and its context parameter name.
func transactionClosure(call *ast.CallExpr, dbNames goast.PackageNames) (*ast.FuncLit, string, bool) {
	if !dbNames.Refers(call.Fun, "Transaction") || len(call.Args) != 2 {
		return nil, "", false
	}
	closure, ok := call.Args[1].(*ast.FuncLit)
	if !ok || closure.Type.Params == nil || len(closure.Type.Params.List) != 1 {
		return nil, "", false
	}
	names := closure.Type.Params.List[0].Names
	if len(names) != 1 {
		return nil, "", false
	}
	return closure, names[0].Name, true
}

// escapingContextIdent reports whether arg is a plain identifier naming a
// context that does not carry the closure's transaction, naming it when so.
func escapingContextIdent(arg ast.Expr, inTransaction map[string]struct{}) (string, bool) {
	ident, ok := arg.(*ast.Ident)
	if !ok {
		return "", false
	}
	if _, held := inTransaction[ident.Name]; held {
		return "", false
	}
	return ident.Name, true
}

// contextsInTransaction returns the names inside the closure that hold a
// context carrying its transaction: the closure's own parameter, and every
// variable assigned from a derivation of one — the bound a statement of the
// transaction takes for itself, for instance. A derivation carries the
// transaction; only a context from elsewhere leaves it.
func contextsInTransaction(body *ast.BlockStmt, ctxParam string) map[string]struct{} {
	held := map[string]struct{}{ctxParam: {}}
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Rhs) != 1 {
			return true
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil || !slices.Contains(contextDerivations, sel.Sel.Name) {
			return true
		}
		if pkg, isIdent := sel.X.(*ast.Ident); !isIdent || pkg.Name != "context" {
			return true
		}
		source, ok := call.Args[0].(*ast.Ident)
		if !ok {
			return true
		}
		if _, ok := held[source.Name]; !ok {
			return true
		}
		// The derived context is the first result; the rest is the cancel.
		if ident, ok := assign.Lhs[0].(*ast.Ident); ok {
			held[ident.Name] = struct{}{}
		}
		return true
	})
	return held
}

// escapingContext reports whether arg is a context that does not carry the
// closure's transaction, naming it as it reads in the source. A derivation
// written at the call is followed to what it derives from, so
// context.WithTimeout(ctx, d) passes and context.WithTimeout(context.Background(), d)
// does not.
func escapingContext(arg ast.Expr, inTransaction map[string]struct{}) (string, bool) {
	switch expr := arg.(type) {
	case *ast.Ident:
		if _, held := inTransaction[expr.Name]; held {
			return "", false
		}
		return fmt.Sprintf("%q", expr.Name), true
	case *ast.CallExpr:
		sel, ok := expr.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil {
			return "", false
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "context" {
			return "", false
		}
		switch {
		case sel.Sel.Name == "Background" || sel.Sel.Name == "TODO":
			return "context." + sel.Sel.Name + "()", true
		case slices.Contains(contextDerivations, sel.Sel.Name) && len(expr.Args) > 0:
			if name, escapes := escapingContext(expr.Args[0], inTransaction); escapes {
				return "context." + sel.Sel.Name + " of " + name, true
			}
		}
	}
	return "", false
}

// databaseEntryPointCall reports whether call enters the framework's database
// package through one of its context-taking entry points, naming the entry.
// The list is the one checkDetachedContext keeps: both rules ask the same
// question about the same calls, one about a detached context and one about
// a context that left the transaction.
func databaseEntryPointCall(call *ast.CallExpr, dbNames goast.PackageNames) (string, bool) {
	if len(call.Args) == 0 {
		return "", false
	}
	fun := call.Fun
	// A generic entry point carries its type arguments: database.Database[M].
	switch indexed := fun.(type) {
	case *ast.IndexExpr:
		fun = indexed.X
	case *ast.IndexListExpr:
		fun = indexed.X
	}
	if !dbNames.Refers(fun, databaseEntryPoints...) {
		return "", false
	}
	switch expr := fun.(type) {
	case *ast.SelectorExpr:
		return expr.Sel.Name, true
	case *ast.Ident:
		return expr.Name, true
	}
	return "", false
}
