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

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// CheckLogFieldBoundedness reports project code that would re-open unbounded
// structured log fields. The framework encoder collapses every reflected log
// value into a single JSON string field so a log store's per-index field
// mapping stays bounded no matter what gets logged. Declaring a zapcore
// marshaler method (MarshalLogObject, MarshalLogArray) or opening a
// zap.Namespace bypasses that collapsing: zap dispatches marshalers before
// its reflection fallback, so each declaration grows the mapping with its own
// key set until the store starts dropping entries. One walk covers the whole
// project including test files; model and service subtrees owned by copyable
// framework modules are skipped, since copied module code is owned by the
// framework repository.
func CheckLogFieldBoundedness(ignore gitignore.Matcher) []string {
	var violations []string

	owned, err := copyableModuleOwners()
	if err != nil {
		return append(violations, fmt.Sprintf("listing copyable framework modules: %v", err))
	}

	walkErr := walkProjectDir(".", ignore, func(path string, info os.FileInfo) error {
		base := filepath.Base(path)
		if info.IsDir() {
			if path == "." {
				return nil
			}
			if strings.HasPrefix(base, ".") || base == "vendor" || base == "testdata" {
				return filepath.SkipDir
			}
			if moduleOwnedPath(owned, modelDir, path) || moduleOwnedPath(owned, serviceDir, path) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(base, ".go") || isGeneratedFileName(path) {
			return nil
		}
		violations = append(violations, checkFileLogFieldBoundedness(path)...)
		return nil
	})
	if walkErr != nil {
		violations = append(violations, fmt.Sprintf("walking project directory: %v", walkErr))
	}

	return violations
}

// checkFileLogFieldBoundedness reports the marshaler method declarations and
// zap.Namespace calls in one file. A file that fails to parse is reported as
// a violation so broken code cannot slip past the check.
func checkFileLogFieldBoundedness(filePath string) []string {
	var violations []string

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return append(violations, fmt.Sprintf("%s has parse error: %v", relativePath(filePath), err))
	}

	relPath := relativePath(filePath)

	for _, decl := range node.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Name == nil || funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
			continue
		}
		name := funcDecl.Name.Name
		if name != "MarshalLogObject" && name != "MarshalLogArray" {
			continue
		}
		recvName, _ := actionTypeBaseName(funcDecl.Recv.List[0].Type)
		pos := fset.Position(funcDecl.Pos())
		violations = append(violations, fmt.Sprintf(
			"%s:%d: type '%s' must not declare %s: zapcore marshalers re-open the structured field expansion the log encoder collapses to keep log-store field mappings bounded; log the value with zap.Any instead",
			relPath, pos.Line, recvName, name,
		))
	}

	aliases, dotImported := zapImportAliases(node)
	if len(aliases) == 0 && !dotImported {
		return violations
	}
	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !isZapNamespaceCall(call.Fun, aliases, dotImported) {
			return true
		}
		pos := fset.Position(call.Pos())
		violations = append(violations, fmt.Sprintf(
			"%s:%d: zap.Namespace must not be called: a nested field namespace grows the log-store field mapping with every key logged under it; use flat typed fields instead",
			relPath, pos.Line,
		))
		return true
	})

	return violations
}

// zapImportAliases returns the names the zap package is imported under in one
// file, and whether it is dot-imported.
func zapImportAliases(file *ast.File) (aliases []string, dotImported bool) {
	for _, imp := range file.Imports {
		if imp.Path == nil || imp.Path.Value != `"go.uber.org/zap"` {
			continue
		}
		switch {
		case imp.Name == nil:
			aliases = append(aliases, "zap")
		case imp.Name.Name == ".":
			dotImported = true
		case imp.Name.Name == "_":
		default:
			aliases = append(aliases, imp.Name.Name)
		}
	}
	return aliases, dotImported
}

// isZapNamespaceCall reports whether a call expression invokes zap.Namespace
// under any of the file's zap import names.
func isZapNamespaceCall(fun ast.Expr, aliases []string, dotImported bool) bool {
	switch x := fun.(type) {
	case *ast.SelectorExpr:
		ident, ok := x.X.(*ast.Ident)
		return ok && x.Sel != nil && x.Sel.Name == "Namespace" && slices.Contains(aliases, ident.Name)
	case *ast.Ident:
		return dotImported && x.Name == "Namespace"
	}
	return false
}
