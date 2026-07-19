package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// CheckTransactionClosureContext checks that inside a database.Transaction
// closure, every database.Database chain and every nested database.Transaction
// call uses the closure's own context parameter. Passing any other context
// identifier makes the operation silently escape the transaction, which is
// exactly the bug the context-injecting Transaction API exists to prevent.
//
// The check is purely syntactic, mirroring CheckDatabaseChainTermination: it
// flags context arguments that are plain identifiers different from the
// enclosing closure's context parameter name. Non-identifier arguments (call
// results, selector expressions) are left alone.
func CheckTransactionClosureContext() []string {
	var violations []string

	ignoreMatcher := newProjectIgnoreMatcher(".")

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
	aliases, dotImport, ok := gstDatabaseImportNames(filePath)
	if !ok {
		return nil
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil
	}

	relPath := relativePath(filePath)

	var violations []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		closure, ctxParam, ok := transactionClosure(call, aliases, dotImport)
		if !ok {
			return true
		}

		ast.Inspect(closure.Body, func(inner ast.Node) bool {
			innerCall, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}

			// A nested Transaction closure has its own context parameter and is
			// checked by the enclosing file walk; only its context argument is
			// this closure's responsibility, so its subtree is skipped here.
			if _, _, isNested := transactionClosure(innerCall, aliases, dotImport); isNested {
				if name, escapes := escapingContextIdent(innerCall.Args[0], ctxParam); escapes {
					pos := fset.Position(innerCall.Pos())
					violations = append(violations, fmt.Sprintf(
						"%s:%d: nested database.Transaction receives context %q inside a database.Transaction closure whose context parameter is %q; it starts a separate transaction instead of joining the enclosing one",
						relPath, pos.Line, name, ctxParam,
					))
				}
				return false
			}

			if !isDatabaseChainStart(innerCall, aliases, dotImport) || len(innerCall.Args) != 1 {
				return true
			}
			if name, escapes := escapingContextIdent(innerCall.Args[0], ctxParam); escapes {
				pos := fset.Position(innerCall.Pos())
				violations = append(violations, fmt.Sprintf(
					"%s:%d: database.Database uses context %q inside a database.Transaction closure whose context parameter is %q; the chain escapes the transaction",
					relPath, pos.Line, name, ctxParam,
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
func transactionClosure(call *ast.CallExpr, aliases []string, dotImport bool) (*ast.FuncLit, string, bool) {
	if !isDatabaseTransactionCall(call, aliases, dotImport) || len(call.Args) != 2 {
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

// isDatabaseTransactionCall reports whether call invokes the framework's
// package-level Transaction function under any recognized import name.
func isDatabaseTransactionCall(call *ast.CallExpr, aliases []string, dotImport bool) bool {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		ident, ok := fun.X.(*ast.Ident)
		return ok && fun.Sel != nil && fun.Sel.Name == "Transaction" && slices.Contains(aliases, ident.Name)
	case *ast.Ident:
		return dotImport && fun.Name == "Transaction"
	}
	return false
}

// escapingContextIdent reports whether arg is a plain identifier that differs
// from the closure's context parameter name, naming the identifier when so.
func escapingContextIdent(arg ast.Expr, ctxParam string) (string, bool) {
	ident, ok := arg.(*ast.Ident)
	if !ok || ident.Name == ctxParam {
		return "", false
	}
	return ident.Name, true
}
