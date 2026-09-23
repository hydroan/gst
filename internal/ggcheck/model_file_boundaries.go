package ggcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
)

// ModelFileBoundaries allows at most one model struct per model file.
var ModelFileBoundaries = Check{
	Name: "Model file boundaries",
	Rule: "model files must contain at most one model struct",
	run:  checkModelFileBoundaries,
}

// checkModelFileBoundaries checks that each model file contains at most one model struct.
func checkModelFileBoundaries(ignore gghelper.ProjectIgnore) []string {
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

		fileViolations := checkFileModelBoundary(path)
		violations = append(violations, fileViolations...)

		return nil
	})
	if err != nil {
		violations = append(violations, fmt.Sprintf("walking model directory: %v", err))
	}

	return violations
}

// checkFileModelBoundary checks the number of model structs in one model file.
func checkFileModelBoundary(filePath string) []string {
	violations := make([]string, 0, 1)

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	modelNames := modelStructNames(node)
	if len(modelNames) <= 1 {
		return violations
	}

	relPath := gghelper.RelativePath(filePath)
	violations = append(violations, fmt.Sprintf("Model file '%s' should contain at most one model struct (found: %s)", relPath, strings.Join(modelNames, ", ")))

	return violations
}

// modelStructNames returns structs that embed model.Base or model.Empty.
func modelStructNames(node *ast.File) []string {
	var names []string

	for _, name := range dsl.FindAllModelBase(node) {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	for _, name := range dsl.FindAllModelEmpty(node) {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}

	return names
}
