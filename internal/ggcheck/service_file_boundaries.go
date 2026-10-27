package ggcheck

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"strings"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/goast"
)

// ServiceFileBoundaries allows at most one service struct per service file.
var ServiceFileBoundaries = Check{
	Name: "Service file boundaries",
	Rule: "service files must contain at most one service struct",
	run:  checkServiceFileBoundaries,
}

// checkServiceFileBoundaries checks that each service file contains at most one service struct.
func checkServiceFileBoundaries(ignore gghelper.ProjectIgnore) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirService); os.IsNotExist(err) {
		return violations
	}

	err := ignore.Walk(ggconst.DirService, func(path string, info os.FileInfo) error {
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.Contains(path, "_test.go") {
			return nil
		}

		if isGeneratedFileName(path) {
			return nil
		}

		fileViolations := checkFileServiceBoundary(path)
		violations = append(violations, fileViolations...)

		return nil
	})
	if err != nil {
		violations = append(violations, fmt.Sprintf("walking service directory: %v", err))
	}

	return violations
}

// checkFileServiceBoundary checks the number of service structs in one service file.
func checkFileServiceBoundary(filePath string) []string {
	violations := make([]string, 0, 1)

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	serviceNames := goast.ServiceStructNames(node)
	if len(serviceNames) <= 1 {
		return violations
	}

	relPath := gghelper.RelativePath(filePath)
	violations = append(violations, fmt.Sprintf("Service file '%s' should contain at most one service struct (found: %s)", relPath, strings.Join(serviceNames, ", ")))

	return violations
}
