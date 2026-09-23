package ggcheck

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// ArchitectureDependencies keeps each project layer from importing the layers
// it must not call.
var ArchitectureDependencies = Check{
	Name: "Architecture dependencies",
	Rule: "service code must not call other service code, dao code must not call service, router, controller or middleware code, and model code must not call service or dao code",
	run:  checkArchitectureDependencies,
}

// checkArchitectureDependencies performs architecture dependency checks.
func checkArchitectureDependencies(ignore gghelper.ProjectIgnore) []string {
	//nolint:prealloc
	var violations []string
	modulePath, err := gghelper.ModulePath()
	if err != nil {
		return []string{fmt.Sprintf("reading the module path: %v", err)}
	}

	// Check service files
	serviceViolations := checkServiceDependencies(modulePath, ignore)
	violations = append(violations, serviceViolations...)

	// Check dao files
	daoViolations := checkDAODependencies(modulePath, ignore)
	violations = append(violations, daoViolations...)

	// Check model files
	modelViolations := checkModelDependencies(modulePath, ignore)
	violations = append(violations, modelViolations...)

	return violations
}

// checkServiceDependencies checks if service code calls other service code.
func checkServiceDependencies(modulePath string, ignore gghelper.ProjectIgnore) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirService); os.IsNotExist(err) {
		return violations
	}

	err := ignore.Walk(ggconst.DirService, func(path string, _ os.FileInfo) error {
		if !strings.HasSuffix(path, ".go") || strings.Contains(path, "_test.go") {
			return nil
		}

		// Skip framework-generated files. Matching the suffix gg owns is
		// exact: matching a bare file name would also skip a project file
		// such as user_service.go and silently drop it from the check.
		if isGeneratedFileName(path) {
			return nil
		}

		fileViolations := checkFileForArchitectureImports(path, "service", modulePath)
		violations = append(violations, fileViolations...)

		return nil
	})
	if err != nil {
		violations = append(violations, fmt.Sprintf("walking service directory: %v", err))
	}

	return violations
}

// checkDAODependencies checks if DAO code calls upper-layer code.
func checkDAODependencies(modulePath string, ignore gghelper.ProjectIgnore) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirDAO); os.IsNotExist(err) {
		return violations
	}

	err := ignore.Walk(ggconst.DirDAO, func(path string, _ os.FileInfo) error {
		if !strings.HasSuffix(path, ".go") || strings.Contains(path, "_test.go") {
			return nil
		}

		fileViolations := checkFileForArchitectureImports(path, "dao", modulePath)
		violations = append(violations, fileViolations...)

		return nil
	})
	if err != nil {
		violations = append(violations, fmt.Sprintf("walking dao directory: %v", err))
	}

	return violations
}

// checkModelDependencies checks if model code calls upper-layer or
// data-access code.
func checkModelDependencies(modulePath string, ignore gghelper.ProjectIgnore) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return violations
	}

	err := ignore.Walk(ggconst.DirModel, func(path string, _ os.FileInfo) error {
		if !strings.HasSuffix(path, ".go") || strings.Contains(path, "_test.go") {
			return nil
		}

		// Skip framework-generated files, matching on the suffix gg owns.
		if isGeneratedFileName(path) {
			return nil
		}

		fileViolations := checkFileForArchitectureImports(path, "model", modulePath)
		violations = append(violations, fileViolations...)

		return nil
	})
	if err != nil {
		violations = append(violations, fmt.Sprintf("walking model directory: %v", err))
	}

	return violations
}

// checkFileForArchitectureImports checks a single file for forbidden
// project-layer imports.
func checkFileForArchitectureImports(filePath, layerType, modulePath string) []string {
	var violations []string

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		// Treat parse errors as violations to prevent code generation
		violation := fmt.Sprintf("%s file '%s' has parse error: %v",
			cases.Title(language.English).String(layerType), filePath, err)
		violations = append(violations, violation)
		return violations
	}

	// Check imports
	for _, imp := range node.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)

		if forbiddenLayer := forbiddenArchitectureImportLayer(filePath, importPath, layerType, modulePath); forbiddenLayer != "" {
			violation := fmt.Sprintf("%s file '%s' imports forbidden %s layer: %s",
				cases.Title(language.English).String(layerType), filePath, forbiddenLayer, importPath)
			violations = append(violations, violation)
		}
	}

	return violations
}

// forbiddenArchitectureImportLayer returns the project layer importPath
// belongs to when a file of layerType must not import it, or "" when the
// import is allowed: in module tmpapp, a dao file importing
// tmpapp/service/sample gives service, and a model file importing tmpapp/dao
// gives dao.
func forbiddenArchitectureImportLayer(filePath, importPath, layerType, modulePath string) string {
	importLayer := projectImportLayer(importPath, modulePath)
	if importLayer == "" {
		return ""
	}

	switch layerType {
	case "service":
		if importLayer == "service" {
			if sameServiceModuleImport(filePath, importPath, modulePath) {
				return ""
			}
			return importLayer
		}
	case "dao":
		if slices.Contains([]string{"service", "router", "controller", "middleware"}, importLayer) {
			return importLayer
		}
	case "model":
		if slices.Contains([]string{"service", "dao"}, importLayer) {
			return importLayer
		}
	}

	return ""
}

// sameServiceModuleImport reports whether a service file imports a package of
// its own service module: service/iam/account/login.go importing
// tmpapp/service/iam/session does, importing tmpapp/service/sample does not.
func sameServiceModuleImport(filePath, importPath, modulePath string) bool {
	// Copied modules can have multiple cooperating packages under one service
	// module, such as service/iam/account importing service/iam/session. The
	// architecture boundary is service/<module>: internal imports stay allowed,
	// while imports across different service/<module> trees remain forbidden.
	sourceModule := serviceModuleNameFromPath(filePath)
	importModule := serviceModuleNameFromImport(importPath, modulePath)
	return sourceModule != "" && sourceModule == importModule
}

// serviceModuleNameFromPath returns the service module a file belongs to, the
// first directory below the service directory: service/iam/account/login.go
// gives iam, while service/login.go and model/iam/user.go give "".
func serviceModuleNameFromPath(filePath string) string {
	rel, err := filepath.Rel(filepath.Clean(ggconst.DirService), filepath.Clean(filePath))
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	moduleName, _, ok := strings.Cut(rel, string(filepath.Separator))
	if !ok || moduleName == "" || strings.HasSuffix(moduleName, ".go") {
		return ""
	}
	return moduleName
}

// serviceModuleNameFromImport returns the service module an import path
// points into: in module tmpapp, tmpapp/service/iam/session gives iam, while
// tmpapp/service and tmpapp/dao/iam give "".
func serviceModuleNameFromImport(importPath, modulePath string) string {
	prefix := strings.Trim(modulePath, "/") + "/service/"
	if !strings.HasPrefix(importPath, prefix) {
		return ""
	}
	rel := strings.TrimPrefix(importPath, prefix)
	moduleName, _, _ := strings.Cut(rel, "/")
	return moduleName
}

// projectImportLayer returns the top-level project directory an import path
// points into: in module tmpapp, tmpapp/service/sample gives service, while
// tmpapp itself and github.com/x/y give "".
func projectImportLayer(importPath, modulePath string) string {
	if len(modulePath) == 0 {
		return ""
	}
	if importPath == modulePath {
		return ""
	}

	prefix := modulePath + "/"
	if !strings.HasPrefix(importPath, prefix) {
		return ""
	}

	layer, _, _ := strings.Cut(strings.TrimPrefix(importPath, prefix), "/")
	return layer
}
