package ggcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
)

// ModelActionTypeNaming holds explicit DSL Payload type names to the Req
// suffix and Result type names to Rsp.
var ModelActionTypeNaming = Check{
	Name: "Model action type naming",
	Rule: "explicit DSL Payload types must end with Req and Result types with Rsp",
	run:  checkModelActionTypeNaming,
}

// checkModelActionTypeNaming checks explicit DSL Payload and Result type names.
func checkModelActionTypeNaming(ignore gghelper.ProjectIgnore) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return violations
	}

	err := ignore.Walk(ggconst.DirModel, func(path string, info os.FileInfo) error {
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.Contains(path, "_test.go") {
			return nil
		}

		if isGeneratedFileName(path) {
			return nil
		}

		fileViolations := checkFileActionTypeNaming(path)
		violations = append(violations, fileViolations...)

		return nil
	})
	if err != nil {
		violations = append(violations, fmt.Sprintf("walking model directory: %v", err))
	}

	return violations
}

// checkFileActionTypeNaming checks explicit Payload and Result types in Design methods.
func checkFileActionTypeNaming(filePath string) []string {
	var violations []string

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	relPath := gghelper.RelativePath(filePath)

	for _, decl := range node.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Name == nil || funcDecl.Name.Name != "Design" || funcDecl.Body == nil {
			continue
		}

		modelName, ok := designReceiverTypeName(funcDecl)
		if !ok {
			continue
		}

		ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			kind, typeExpr, ok := dslActionTypeCall(call.Fun)
			if !ok {
				return true
			}

			typeName, ok := typeBaseName(typeExpr)
			if !ok || typeName == modelName {
				return true
			}

			switch kind {
			case "Payload":
				if !strings.HasSuffix(typeName, "Req") {
					pos := fset.Position(call.Pos())
					violations = append(violations, fmt.Sprintf("%s:%d: Payload type '%s' should end with Req", relPath, pos.Line, typeName))
				}
			case "Result":
				if !strings.HasSuffix(typeName, "Rsp") {
					pos := fset.Position(call.Pos())
					violations = append(violations, fmt.Sprintf("%s:%d: Result type '%s' should end with Rsp", relPath, pos.Line, typeName))
				}
			}

			return true
		})
	}

	return violations
}

// designReceiverTypeName returns the receiver type name for a Design method.
func designReceiverTypeName(fn *ast.FuncDecl) (string, bool) {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return "", false
	}
	return typeBaseName(fn.Recv.List[0].Type)
}
