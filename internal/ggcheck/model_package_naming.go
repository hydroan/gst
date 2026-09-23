package ggcheck

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
)

// ModelPackageNaming holds model package names to their directory names.
var ModelPackageNaming = Check{
	Name: "Model package naming",
	Rule: "model package names must match their directory names",
	run:  checkModelPackageNaming,
}

// checkModelPackageNaming checks if model package names match their directory
// names.
func checkModelPackageNaming(ignore gghelper.ProjectIgnore) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return violations
	}

	err := ignore.Walk(ggconst.DirModel, func(path string, info os.FileInfo) error {
		// Skip directories and non-Go files
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip files in the root model directory
		relPath, err := filepath.Rel(ggconst.DirModel, path)
		if err != nil {
			return err
		}
		if !strings.Contains(relPath, string(filepath.Separator)) {
			return nil
		}

		// Get the directory name (should match package name)
		dir := filepath.Dir(path)
		dirName := filepath.Base(dir)

		// Parse the Go file to get package name
		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, path, nil, parser.PackageClauseOnly)
		if err != nil {
			return err
		}

		packageName := node.Name.Name

		// Go convention discourages underscores in package names, so the
		// directory name is compared without them.
		expectedName := gen.ModelPackageName(dirName)

		// Allow black-box test files to use the `<package>_test` external test package name.
		if strings.HasSuffix(path, "_test.go") && packageName == expectedName+"_test" {
			return nil
		}

		// Check if package name matches directory name
		if packageName != expectedName {
			violations = append(violations, fmt.Sprintf("%s: package name '%s' should match directory name '%s'", relPath, packageName, dirName))
		}

		return nil
	})
	if err != nil {
		violations = append(violations, fmt.Sprintf("walking model directory: %v", err))
	}

	return violations
}
