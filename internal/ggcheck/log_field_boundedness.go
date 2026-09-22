package ggcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/goast"
)

// zapImportPath is the logging package whose Namespace function the log field
// check forbids.
const zapImportPath = "go.uber.org/zap"

// LogFieldBoundedness keeps zapcore marshalers and zap.Namespace out of
// project code.
var LogFieldBoundedness = Check{
	Name: "Log field boundedness",
	Rule: "project code must not declare MarshalLogObject or MarshalLogArray methods and must not call zap.Namespace: zapcore marshalers and nested namespaces bypass the reflected-value collapsing that keeps log-store field mappings bounded",
	run:  checkLogFieldBoundedness,
}

// checkLogFieldBoundedness reports project code that would re-open unbounded
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
func checkLogFieldBoundedness(ignore gitignore.Matcher) []string {
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
			if moduleOwnedPath(owned, ggconst.DirModel, path) || moduleOwnedPath(owned, ggconst.DirService, path) {
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

	zapNames := goast.ImportedNames(node, zapImportPath, "zap")
	if len(zapNames.Qualifiers) == 0 && !zapNames.DotImported {
		return violations
	}
	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !zapNames.Refers(call.Fun, "Namespace") {
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
