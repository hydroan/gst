package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/gertd/go-pluralize"
	"github.com/go-git/go-billy/v5/osfs"
	gitignore "github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "check architecture dependencies in generated code",
	Long: `Check architecture dependencies in generated code:
1. Service code should not call other service code
2. DAO code should not call service, router, controller, or middleware code
3. Model code should not call service or dao code
4. Model directories and files must be singular
5. Model file names should not contain hyphens (use underscores instead)
6. Model struct json tags should use snake_case naming
7. Model package names must match their directory names
8. Explicit DSL Payload types should end with Req and Result types should end with Rsp
9. Model files should contain at most one model struct
10. Service files should contain at most one service struct
11. Only allowed directories are enforced for gst framework projects
12. Model Design() DSL must pass the same validation rules that gate gg gen
13. database.Database operation chains must end with a terminal operation inline or be passed directly as a call argument`,
	Run: func(cmd *cobra.Command, args []string) {
		checkRun()
	},
}

func checkRun() {
	totalViolations := runProjectChecks()

	clioutput.Section("Summary")
	if totalViolations > 0 {
		clioutput.Error("", "%d violations found", totalViolations)
		os.Exit(1)
	} else {
		clioutput.Success("", "All checks passed")
	}
}

type projectCheckResult struct {
	Name       string
	Violations []string
}

// runProjectChecks runs all project checks shared by gg check and gg gen.
func runProjectChecks() int {
	results := collectProjectChecks()
	printProjectCheckResults(results)
	return totalProjectCheckViolations(results)
}

// runProjectChecksQuiet reports project check violations without printing
// anything when the project is clean. Violations recorded in baseline are
// treated as pre-existing and are neither counted nor printed, so callers such
// as module copy fail only on violations introduced after the baseline
// snapshot. A nil baseline keeps the full check behavior.
func runProjectChecksQuiet(baseline map[string]struct{}) int {
	results := filterProjectCheckResults(collectProjectChecks(), baseline)
	total := totalProjectCheckViolations(results)
	if total > 0 {
		printProjectCheckResults(results)
	}
	return total
}

// collectProjectCheckBaseline snapshots the current project check violations.
// Module copy records this baseline before writing any file, so its embedded
// gg gen run fails only on violations introduced by the copied module instead
// of pre-existing project issues.
func collectProjectCheckBaseline() map[string]struct{} {
	baseline := make(map[string]struct{})
	for _, result := range collectProjectChecks() {
		for _, violation := range result.Violations {
			baseline[violation] = struct{}{}
		}
	}
	return baseline
}

// filterProjectCheckResults drops violations recorded in baseline, keeping
// only violations introduced after the baseline snapshot.
func filterProjectCheckResults(results []projectCheckResult, baseline map[string]struct{}) []projectCheckResult {
	if len(baseline) == 0 {
		return results
	}
	filtered := make([]projectCheckResult, 0, len(results))
	for _, result := range results {
		violations := make([]string, 0, len(result.Violations))
		for _, violation := range result.Violations {
			if _, preexisting := baseline[violation]; preexisting {
				continue
			}
			violations = append(violations, violation)
		}
		filtered = append(filtered, projectCheckResult{Name: result.Name, Violations: violations})
	}
	return filtered
}

func collectProjectChecks() []projectCheckResult {
	results := []projectCheckResult{
		{Name: "Architecture dependencies", Violations: CheckArchitectureDependency()},
		{Name: "Model singular naming", Violations: CheckModelSingularNaming()},
		{Name: "Model JSON tag naming", Violations: CheckModelJSONTagNaming()},
		{Name: "Model action type naming", Violations: CheckModelActionTypeNaming()},
		{Name: "Model file boundaries", Violations: CheckModelFileBoundary()},
		{Name: "Service file boundaries", Violations: CheckServiceFileBoundary()},
		{Name: "Model package naming", Violations: CheckModelPackageNaming()},
		{Name: "Directory restrictions", Violations: CheckAllowedDirectories()},
		{Name: "DSL design rules", Violations: CheckDSLDesign()},
		{Name: "Database chain termination", Violations: CheckDatabaseChainTermination()},
	}
	return results
}

func printProjectCheckResults(results []projectCheckResult) {
	clioutput.Section("Project Checks")
	for _, result := range results {
		if len(result.Violations) == 0 {
			clioutput.Success("", "%s", result.Name)
			continue
		}

		clioutput.Error("", "%s (%d)", result.Name, len(result.Violations))
		for _, violation := range result.Violations {
			clioutput.Item("", "%s", violation)
		}
	}
}

func totalProjectCheckViolations(results []projectCheckResult) int {
	var total int
	for _, result := range results {
		total += len(result.Violations)
	}
	return total
}

// CheckArchitectureDependency performs architecture dependency checks.
func CheckArchitectureDependency() []string {
	//nolint:prealloc
	var violations []string
	modulePath := currentProjectModulePath()

	// Check service files
	serviceViolations := checkServiceDependencies(modulePath)
	violations = append(violations, serviceViolations...)

	// Check dao files
	daoViolations := checkDAODependencies(modulePath)
	violations = append(violations, daoViolations...)

	// Check model files
	modelViolations := checkModelDependencies(modulePath)
	violations = append(violations, modelViolations...)

	return violations
}

// checkServiceDependencies checks if service code calls other service code
func checkServiceDependencies(modulePath string) []string {
	var violations []string

	if _, err := os.Stat(serviceDir); os.IsNotExist(err) {
		return violations
	}

	err := filepath.Walk(serviceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !strings.HasSuffix(path, ".go") || strings.Contains(path, "_test.go") {
			return nil
		}

		// Skip service.go registration file
		if strings.HasSuffix(path, "service.go") {
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
func checkDAODependencies(modulePath string) []string {
	var violations []string

	if _, err := os.Stat(daoDir); os.IsNotExist(err) {
		return violations
	}

	err := filepath.Walk(daoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

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

// checkModelDependencies checks if model code calls upper-layer or data-access code.
func checkModelDependencies(modulePath string) []string {
	var violations []string

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return violations
	}

	err := filepath.Walk(modelDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !strings.HasSuffix(path, ".go") || strings.Contains(path, "_test.go") {
			return nil
		}

		// Skip model.go registration file
		if strings.HasSuffix(path, "model.go") {
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

// checkFileForArchitectureImports checks a single file for forbidden project-layer imports.
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

// CheckModelSingularNaming checks if model directories and files use singular names
func CheckModelSingularNaming() []string {
	var violations []string

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return violations
	}

	// Common plural file names that are allowed in Go projects
	allowedPluralFiles := map[string]bool{
		"types":       true,
		"errors":      true,
		"constants":   true,
		"consts":      true,
		"vars":        true,
		"handlers":    true,
		"models":      true,
		"examples":    true,
		"configs":     true,
		"options":     true,
		"helpers":     true,
		"utils":       true,
		"interfaces":  true,
		"services":    true,
		"clients":     true,
		"controllers": true,
		"apis":        true,
		"schemas":     true,
		"entities":    true,
		"records":     true,
		"data":        true,
	}
	allowedPluralDirs := map[string]bool{
		"types": true,
		"data":  true,
	}

	client := pluralize.NewClient()

	err := filepath.Walk(modelDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path from model directory
		relPath, err := filepath.Rel(modelDir, path)
		if err != nil {
			return err
		}

		// Skip the root model directory itself
		if relPath == "." {
			return nil
		}

		if info.IsDir() {
			// Check directory name.
			// Directory name length must greater than 3 before check.
			// Check singular must before plural.
			dirName := info.Name()
			if len(dirName) > 3 && !allowedPluralDirs[dirName] && !client.IsSingular(dirName) && client.IsPlural(dirName) {
				violation := fmt.Sprintf("Model directory '%s' should be singular (suggested: %s)",
					path, client.Singular(dirName))
				violations = append(violations, violation)
			}
		} else if strings.HasSuffix(path, ".go") && !strings.Contains(path, "_test.go") {
			// Skip model.go registration file
			if strings.HasSuffix(path, "model.go") {
				return nil
			}

			// Check Go file name (without .go extension)
			fileName := strings.TrimSuffix(info.Name(), ".go")

			// Check if file name contains hyphen
			if strings.Contains(fileName, "-") {
				suggestedName := strings.ReplaceAll(fileName, "-", "_")
				violation := fmt.Sprintf("Model file '%s' should not contain hyphens (suggested: %s.go)",
					path, suggestedName)
				violations = append(violations, violation)
			}

			// File name length must greater than 3 before check.
			// Check singular must before plural.
			// Skip check for allowed plural file names
			if len(fileName) > 3 && !allowedPluralFiles[fileName] && !client.IsSingular(fileName) && client.IsPlural(fileName) {
				violation := fmt.Sprintf("Model file '%s' should be singular (suggested: %s.go)",
					path, client.Singular(fileName))
				violations = append(violations, violation)
			}
		}

		return nil
	})
	if err != nil {
		violations = append(violations, fmt.Sprintf("walking model directory: %v", err))
	}

	return violations
}

func currentProjectModulePath() string {
	modulePath, err := getModuleName()
	if err != nil {
		return ""
	}
	return strings.Trim(modulePath, "/")
}

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

func sameServiceModuleImport(filePath, importPath, modulePath string) bool {
	// Copied modules can have multiple cooperating packages under one service
	// module, such as service/iam/account importing service/iam/session. The
	// architecture boundary is service/<module>: internal imports stay allowed,
	// while imports across different service/<module> trees remain forbidden.
	sourceModule := serviceModuleNameFromPath(filePath)
	importModule := serviceModuleNameFromImport(importPath, modulePath)
	return sourceModule != "" && sourceModule == importModule
}

func serviceModuleNameFromPath(filePath string) string {
	rel, err := filepath.Rel(filepath.Clean(serviceDir), filepath.Clean(filePath))
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	moduleName, _, ok := strings.Cut(rel, string(filepath.Separator))
	if !ok || moduleName == "" || strings.HasSuffix(moduleName, ".go") {
		return ""
	}
	return moduleName
}

func serviceModuleNameFromImport(importPath, modulePath string) string {
	prefix := strings.Trim(modulePath, "/") + "/service/"
	if !strings.HasPrefix(importPath, prefix) {
		return ""
	}
	rel := strings.TrimPrefix(importPath, prefix)
	moduleName, _, _ := strings.Cut(rel, "/")
	return moduleName
}

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

// CheckModelJSONTagNaming checks if model struct json tags use camelCase naming
func CheckModelJSONTagNaming() []string {
	var violations []string

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return violations
	}

	err := filepath.Walk(modelDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-Go files
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip generated files
		if strings.HasSuffix(path, "model.go") {
			return nil
		}

		fileViolations := checkFileJSONTagNaming(path)
		violations = append(violations, fileViolations...)

		return nil
	})
	if err != nil {
		violations = append(violations, fmt.Sprintf("walking model directory: %v", err))
	}

	return violations
}

// checkFileJSONTagNaming checks json tag naming in a single file
func checkFileJSONTagNaming(filePath string) []string {
	var violations []string

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Find all model structs in this file
	modelBaseNames := dsl.FindAllModelBase(node)
	modelEmptyNames := dsl.FindAllModelEmpty(node)
	allModelNames := slices.Concat(modelBaseNames, modelEmptyNames)

	// If no model structs found, skip this file
	if len(allModelNames) == 0 {
		return violations
	}

	// Get relative path for cleaner output
	cwd, _ := os.Getwd()
	relPath, _ := filepath.Rel(cwd, filePath)

	// Check only model structs
	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			// Check if this struct is a model
			isModel := slices.Contains(allModelNames, typeSpec.Name.Name)
			if !isModel {
				continue
			}

			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok || structType.Fields == nil {
				continue
			}

			// Check JSON tags in this model struct
			for _, field := range structType.Fields.List {
				if field.Tag != nil {
					tagValue := strings.Trim(field.Tag.Value, "`")
					if jsonTag := extractJSONTag(tagValue); jsonTag != "" {
						if !isSnakeCase(jsonTag) {
							fieldName := ""
							if len(field.Names) > 0 {
								fieldName = field.Names[0].Name
							}
							violations = append(violations, fmt.Sprintf(
								"%s: field '%s' json tag '%s' should be '%s'",
								relPath, fieldName, jsonTag, toSnakeCase(jsonTag),
							))
						}
					}
				}
			}
		}
	}

	return violations
}

// extractJSONTag extracts the json tag value from struct tag
func extractJSONTag(tag string) string {
	re := regexp.MustCompile(`json:"([^"]+)"`)
	matches := re.FindStringSubmatch(tag)
	if len(matches) > 1 {
		// Remove options like omitempty
		parts := strings.Split(matches[1], ",")
		return parts[0]
	}
	return ""
}

// isSnakeCase checks if a string is in snake_case format
func isSnakeCase(s string) bool {
	if s == "" {
		return true
	}

	// Skip special cases like "-" or single characters
	if s == "-" || len(s) == 1 {
		return true
	}

	// Check if it contains hyphens (kebab-case) or uppercase letters
	if strings.Contains(s, "-") {
		return false
	}

	// Check for uppercase letters (not snake_case)
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			return false
		}
	}

	return true
}

// toSnakeCase converts camelCase or kebab-case to snake_case
func toSnakeCase(s string) string {
	if s == "" {
		return s
	}

	// Replace hyphens with underscores
	s = strings.ReplaceAll(s, "-", "_")

	// Convert camelCase to snake_case
	var result strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				result.WriteRune('_')
			}
			result.WriteRune(r - 'A' + 'a')
		} else {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// CheckModelActionTypeNaming checks explicit DSL Payload and Result type names.
func CheckModelActionTypeNaming() []string {
	var violations []string

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return violations
	}

	err := filepath.Walk(modelDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.Contains(path, "_test.go") {
			return nil
		}

		if strings.HasSuffix(path, "model.go") {
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

	cwd, _ := os.Getwd()
	relPath, _ := filepath.Rel(cwd, filePath)

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

			typeName, ok := actionTypeBaseName(typeExpr)
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
	return actionTypeBaseName(fn.Recv.List[0].Type)
}

// dslActionTypeCall returns the kind and type argument for DSL Payload/Result calls.
func dslActionTypeCall(expr ast.Expr) (string, ast.Expr, bool) {
	switch x := expr.(type) {
	case *ast.IndexExpr:
		if kind, ok := dslActionTypeName(x.X); ok {
			return kind, x.Index, true
		}
	case *ast.IndexListExpr:
		if len(x.Indices) == 1 {
			if kind, ok := dslActionTypeName(x.X); ok {
				return kind, x.Indices[0], true
			}
		}
	}
	return "", nil, false
}

// dslActionTypeName returns the DSL function name for Payload or Result.
func dslActionTypeName(expr ast.Expr) (string, bool) {
	switch x := expr.(type) {
	case *ast.Ident:
		if x.Name == "Payload" || x.Name == "Result" {
			return x.Name, true
		}
	case *ast.SelectorExpr:
		if x.Sel != nil && (x.Sel.Name == "Payload" || x.Sel.Name == "Result") {
			return x.Sel.Name, true
		}
	}
	return "", false
}

// actionTypeBaseName extracts the named type from supported DSL type arguments.
func actionTypeBaseName(expr ast.Expr) (string, bool) {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name, true
	case *ast.StarExpr:
		return actionTypeBaseName(x.X)
	case *ast.SelectorExpr:
		if x.Sel != nil {
			return x.Sel.Name, true
		}
	}
	return "", false
}

// CheckModelFileBoundary checks that each model file contains at most one model struct.
func CheckModelFileBoundary() []string {
	var violations []string

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return violations
	}

	err := filepath.Walk(modelDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.Contains(path, "_test.go") {
			return nil
		}

		if strings.HasSuffix(path, "model.go") {
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

	relPath := relativePath(filePath)
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

// CheckServiceFileBoundary checks that each service file contains at most one service struct.
func CheckServiceFileBoundary() []string {
	var violations []string

	if _, err := os.Stat(serviceDir); os.IsNotExist(err) {
		return violations
	}

	err := filepath.Walk(serviceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.Contains(path, "_test.go") {
			return nil
		}

		if strings.HasSuffix(path, "service.go") {
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

	serviceNames := serviceStructNames(node)
	if len(serviceNames) <= 1 {
		return violations
	}

	relPath := relativePath(filePath)
	violations = append(violations, fmt.Sprintf("Service file '%s' should contain at most one service struct (found: %s)", relPath, strings.Join(serviceNames, ", ")))

	return violations
}

// serviceStructNames returns structs that embed service.Base with three type parameters.
func serviceStructNames(node *ast.File) []string {
	var names []string

	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok || structType.Fields == nil {
				continue
			}

			for _, field := range structType.Fields.List {
				if isServiceBase(node, field) {
					names = append(names, typeSpec.Name.Name)
					break
				}
			}
		}
	}

	return names
}

// isServiceBase reports whether the field anonymously embeds service.Base[M, REQ, RSP].
func isServiceBase(file *ast.File, field *ast.Field) bool {
	if file == nil || field == nil || len(field.Names) != 0 {
		return false
	}

	indexListExpr, ok := field.Type.(*ast.IndexListExpr)
	if !ok || len(indexListExpr.Indices) != 3 {
		return false
	}

	return isServiceBaseName(file, indexListExpr.X)
}

// isServiceBaseName checks whether expr names the gst service.Base type.
func isServiceBaseName(file *ast.File, expr ast.Expr) bool {
	aliasNames := []string{"service"}
	var dotImport bool

	for _, imp := range file.Imports {
		if imp.Path == nil || imp.Path.Value != `"github.com/hydroan/gst/service"` {
			continue
		}
		if imp.Name == nil {
			continue
		}
		if imp.Name.Name == "." {
			dotImport = true
			continue
		}
		if !slices.Contains(aliasNames, imp.Name.Name) {
			aliasNames = append(aliasNames, imp.Name.Name)
		}
	}

	switch x := expr.(type) {
	case *ast.SelectorExpr:
		ident, ok := x.X.(*ast.Ident)
		return ok && x.Sel != nil && x.Sel.Name == "Base" && slices.Contains(aliasNames, ident.Name)
	case *ast.Ident:
		return dotImport && x.Name == "Base"
	}

	return false
}

// relativePath returns filePath relative to the current working directory when possible.
func relativePath(filePath string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return filePath
	}
	relPath, err := filepath.Rel(cwd, filePath)
	if err != nil {
		return filePath
	}
	return relPath
}

// CheckModelPackageNaming checks if model package names match their directory names
func CheckModelPackageNaming() []string {
	var violations []string

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return violations
	}

	err := filepath.Walk(modelDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-Go files
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip files in the root model directory
		relPath, err := filepath.Rel(modelDir, path)
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

		// Go convention discourages underscores in package names, so strip them
		// from the directory name before comparing.
		expectedName := strings.ReplaceAll(dirName, "_", "")

		// Allow black-box test files to use the `<package>_test` external test package name.
		if strings.HasSuffix(path, "_test.go") && packageName == expectedName+"_test" {
			return nil
		}

		// Check if package name matches directory name
		if packageName != expectedName {
			relativePath, _ := filepath.Rel(modelDir, path)
			violations = append(violations, fmt.Sprintf("%s: package name '%s' should match directory name '%s'", relativePath, packageName, dirName))
		}

		return nil
	})
	if err != nil {
		violations = append(violations, fmt.Sprintf("walking model directory: %v", err))
	}

	return violations
}

// CheckAllowedDirectories checks if only allowed directories exist in the project
func CheckAllowedDirectories() []string {
	projectDir := "."
	var violations []string

	// Check if this is a gst framework project by reading go.mod
	if isGstFrameworkProject(projectDir) {
		// Skip directory restriction check for gst framework itself
		return violations
	}

	// Check if this project uses gst framework
	if !usesGstFramework(projectDir) {
		// Skip directory restriction check for projects not using gst framework
		return violations
	}

	// Define allowed directories for gst framework projects
	allowedDirs := map[string]bool{
		"model":      true,
		"module":     true,
		"service":    true,
		"router":     true,
		"dao":        true,
		"provider":   true,
		"middleware": true,
		"cronjob":    true,
		"configx":    true,
		"config":     true,
		"typesx":     true,
		"consts":     true,
		"constx":     true,
		"type":       true,
		"typex":      true,
		"helper":     true,
		"internal":   true,
		"cmd":        true,
		"errorx":     true,
		"testcode":   true,
		"testdata":   true,
		"docs":       true,
		"doc":        true,
	}

	whitelistDirs := map[string]bool{
		"tmp":       true,
		"logs":      true,
		"dist":      true,
		"generated": true,
	}
	ignoreMatcher := newProjectIgnoreMatcher(projectDir)

	// Read directory contents
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return violations
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()

		// Skip hidden directories and common project files
		if strings.HasPrefix(dirName, ".") {
			continue
		}
		if isIgnoredProjectDirectory(ignoreMatcher, dirName) {
			continue
		}

		// Check if directory is allowed
		if !allowedDirs[dirName] && !whitelistDirs[dirName] {
			violations = append(violations, fmt.Sprintf("Directory '%s' is not allowed in project structure", dirName))
		}
	}

	return violations
}

// newProjectIgnoreMatcher loads Git ignore rules for the project root.
func newProjectIgnoreMatcher(projectDir string) gitignore.Matcher {
	patterns, err := gitignore.ReadPatterns(osfs.New(projectDir), nil)
	if err != nil || len(patterns) == 0 {
		return nil
	}
	return gitignore.NewMatcher(patterns)
}

// isIgnoredProjectDirectory reports whether a root-level project directory is ignored by Git rules.
func isIgnoredProjectDirectory(matcher gitignore.Matcher, dirName string) bool {
	if matcher == nil {
		return false
	}
	return matcher.Match([]string{dirName}, true)
}

// isGstFrameworkProject checks if this is the gst framework project itself
func isGstFrameworkProject(projectDir string) bool {
	goModPath := filepath.Join(projectDir, "go.mod")
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return false
	}

	// Check if module name is github.com/hydroan/gst
	lines := strings.SplitSeq(string(content), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			moduleName := strings.TrimSpace(strings.TrimPrefix(line, "module"))
			return moduleName == "github.com/hydroan/gst"
		}
	}
	return false
}

// usesGstFramework checks if the project uses gst framework as a dependency
func usesGstFramework(projectDir string) bool {
	goModPath := filepath.Join(projectDir, "go.mod")
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return false
	}

	// Check if github.com/hydroan/gst is in dependencies
	return strings.Contains(string(content), "github.com/hydroan/gst")
}

// CheckDSLDesign runs DSL Design() validation on every model file, so keyword
// placement and generation-semantic violations fail gg check with the same
// rules that block gg gen.
func CheckDSLDesign() []string {
	var violations []string

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return violations
	}

	err := filepath.Walk(modelDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		base := filepath.Base(path)
		if info.IsDir() {
			if path != modelDir && (base == "vendor" || base == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(base, ".go") ||
			strings.HasSuffix(base, "_test.go") ||
			strings.HasPrefix(base, "_") ||
			slices.Contains(excludes, base) {
			return nil
		}

		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			violations = append(violations, fmt.Sprintf("%s: %v", path, parseErr))
			return nil
		}
		for _, validateErr := range dsl.Validate(file, modelDir, path) {
			violations = append(violations, validateErr.Error())
		}
		return nil
	})
	if err != nil {
		violations = append(violations, err.Error())
	}

	return violations
}
