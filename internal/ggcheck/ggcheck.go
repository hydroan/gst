// Package ggcheck holds the rules gg check holds a business project to. Each
// Check finds the violations of one rule in the project in the working
// directory; the gg command running the checks decides which run, in what
// order, and how their results print.
package ggcheck

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
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/goast"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Check is one rule gg check holds a project to.
type Check struct {
	// Name is the name the check's result prints under.
	Name string
	// Rule states the rule, as the help of gg check lists it.
	Rule string

	run func(ignore gitignore.Matcher) []string
}

// Result is what one check found in the project.
type Result struct {
	// Name is the name of the check.
	Name string
	// Violations describes each violation found, one line each; it is empty
	// when the project follows the rule.
	Violations []string
}

// Run runs checks over the project in the working directory, in the order
// given, and returns their results in the same order. Paths the project's Git
// ignore rules ignore are left out of every check.
func Run(checks []Check) []Result {
	// One matcher serves every check: building it scans the whole worktree
	// for ignore files, which is too expensive to repeat per check.
	ignore := newProjectIgnoreMatcher()
	results := make([]Result, 0, len(checks))
	for _, check := range checks {
		results = append(results, Result{Name: check.Name, Violations: check.run(ignore)})
	}
	return results
}

// ArchitectureDependencies keeps each project layer from importing the layers
// it must not call.
var ArchitectureDependencies = Check{
	Name: "Architecture dependencies",
	Rule: "service code must not call other service code, dao code must not call service, router, controller or middleware code, and model code must not call service or dao code",
	run:  checkArchitectureDependencies,
}

// checkArchitectureDependencies performs architecture dependency checks.
func checkArchitectureDependencies(ignore gitignore.Matcher) []string {
	//nolint:prealloc
	var violations []string
	modulePath := currentProjectModulePath()

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

// checkServiceDependencies checks if service code calls other service code
func checkServiceDependencies(modulePath string, ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirService); os.IsNotExist(err) {
		return violations
	}

	err := walkProjectDir(ggconst.DirService, ignore, func(path string, _ os.FileInfo) error {
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
func checkDAODependencies(modulePath string, ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirDAO); os.IsNotExist(err) {
		return violations
	}

	err := walkProjectDir(ggconst.DirDAO, ignore, func(path string, _ os.FileInfo) error {
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
func checkModelDependencies(modulePath string, ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return violations
	}

	err := walkProjectDir(ggconst.DirModel, ignore, func(path string, _ os.FileInfo) error {
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

// ModelSingularNaming keeps model directory and file names singular.
var ModelSingularNaming = Check{
	Name: "Model singular naming",
	Rule: "model directories and files must be singular",
	run:  checkModelSingularNaming,
}

// checkModelSingularNaming checks that model directories and files use
// singular names.
func checkModelSingularNaming(ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return violations
	}

	// Common plural file names that are allowed in Go projects. data and
	// stats name one body of content rather than many items; stats is the
	// conventional short form of statistics.
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
		"stats":       true,
	}
	// Plural directory names that are allowed: types for a directory of
	// shared types, and data and stats, which name one body of content.
	allowedPluralDirs := map[string]bool{
		"types": true,
		"data":  true,
		"stats": true,
	}

	client := pluralize.NewClient()

	err := walkProjectDir(ggconst.DirModel, ignore, func(path string, info os.FileInfo) error {
		// Get relative path from model directory
		relPath, err := filepath.Rel(ggconst.DirModel, path)
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
			// Skip framework-generated files.
			if isGeneratedFileName(path) {
				return nil
			}

			// Check Go file name (without .go extension)
			fileName := strings.TrimSuffix(info.Name(), ".go")

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

// ModelFileNameHyphens keeps hyphens out of model file names.
var ModelFileNameHyphens = Check{
	Name: "Model file name hyphens",
	Rule: "model file names must not contain hyphens (use underscores instead)",
	run:  checkModelFileNameHyphens,
}

// checkModelFileNameHyphens checks that model file names separate words with
// underscores rather than hyphens.
func checkModelFileNameHyphens(ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return violations
	}

	err := walkProjectDir(ggconst.DirModel, ignore, func(path string, info os.FileInfo) error {
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.Contains(path, "_test.go") || isGeneratedFileName(path) {
			return nil
		}
		fileName := strings.TrimSuffix(info.Name(), ".go")
		if strings.Contains(fileName, "-") {
			violations = append(violations, fmt.Sprintf("Model file '%s' should not contain hyphens (suggested: %s.go)",
				path, strings.ReplaceAll(fileName, "-", "_")))
		}
		return nil
	})
	if err != nil {
		violations = append(violations, fmt.Sprintf("walking model directory: %v", err))
	}

	return violations
}

func currentProjectModulePath() string {
	modulePath, err := gghelper.ModulePath()
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

// JSONTagNaming holds the json tags of models and of explicit DSL Payload and
// Result types to snake_case.
var JSONTagNaming = Check{
	Name: "JSON tag naming",
	Rule: "model struct and explicit DSL Payload/Result type json tags must use snake_case naming",
	run:  checkJSONTagNaming,
}

// checkJSONTagNaming checks that json tags declared under the model directory
// use snake_case naming. It covers model structs and the explicit DSL Payload
// and Result types referenced by Design methods.
func checkJSONTagNaming(ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return violations
	}

	// Files are grouped per directory because an explicit DSL action type may
	// be declared in a different file of the package that references it.
	var packageDirs []string
	packageFiles := make(map[string][]string)
	err := walkProjectDir(ggconst.DirModel, ignore, func(path string, info os.FileInfo) error {
		// Skip directories and non-Go files
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip framework-generated files.
		if isGeneratedFileName(path) {
			return nil
		}

		violations = append(violations, checkFileModelJSONTagNaming(path)...)

		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		dir := filepath.Dir(path)
		if _, seen := packageFiles[dir]; !seen {
			packageDirs = append(packageDirs, dir)
		}
		packageFiles[dir] = append(packageFiles[dir], path)
		return nil
	})
	if err != nil {
		violations = append(violations, fmt.Sprintf("walking model directory: %v", err))
	}

	for _, dir := range packageDirs {
		violations = append(violations, checkPackageActionTypeJSONTagNaming(packageFiles[dir])...)
	}

	return violations
}

// checkFileModelJSONTagNaming checks json tags of the model structs in one file.
func checkFileModelJSONTagNaming(filePath string) []string {
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

	relPath := relativePath(filePath)

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
			if !slices.Contains(allModelNames, typeSpec.Name.Name) {
				continue
			}

			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok || structType.Fields == nil {
				continue
			}

			violations = append(violations, structJSONTagViolations(relPath, structType)...)
		}
	}

	return violations
}

// checkPackageActionTypeJSONTagNaming checks json tags of the explicit DSL
// Payload and Result types declared in one model package. Only types
// referenced by a Design method and declared in the same package are checked:
// model structs are already covered by checkFileModelJSONTagNaming, and
// unreferenced structs, such as DTOs mirroring an external wire contract, must
// keep their own naming.
func checkPackageActionTypeJSONTagNaming(paths []string) []string {
	var violations []string

	fset := token.NewFileSet()
	files := make(map[string]*ast.File, len(paths))
	for _, path := range paths {
		node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			continue
		}
		files[path] = node
	}

	// Model structs are covered by the per-file model check already.
	covered := make(map[string]bool)
	for _, node := range files {
		for _, name := range slices.Concat(dsl.FindAllModelBase(node), dsl.FindAllModelEmpty(node)) {
			covered[name] = true
		}
	}

	// Collect the same-package type names referenced as Payload[T] or Result[T].
	referenced := make(map[string]bool)
	for _, node := range files {
		for _, decl := range node.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Name == nil || funcDecl.Name.Name != "Design" || funcDecl.Recv == nil || funcDecl.Body == nil {
				continue
			}
			ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				_, typeExpr, ok := dslActionTypeCall(call.Fun)
				if !ok {
					return true
				}
				if name, ok := localActionTypeName(typeExpr); ok && !covered[name] {
					referenced[name] = true
				}
				return true
			})
		}
	}
	if len(referenced) == 0 {
		return violations
	}

	// Check declarations of the referenced types in file walk order.
	for _, path := range paths {
		node, ok := files[path]
		if !ok {
			continue
		}
		relPath := relativePath(path)
		for _, decl := range node.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}
			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || !referenced[typeSpec.Name.Name] {
					continue
				}

				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok || structType.Fields == nil {
					continue
				}

				violations = append(violations, structJSONTagViolations(relPath, structType)...)
			}
		}
	}

	return violations
}

// localActionTypeName resolves a DSL type argument to a type name declared in
// the same package. Pointer forms are unwrapped; qualified names from other
// packages are not resolved.
func localActionTypeName(expr ast.Expr) (string, bool) {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name, true
	case *ast.StarExpr:
		return localActionTypeName(x.X)
	}
	return "", false
}

// structJSONTagViolations reports the fields of one struct whose json tags are
// not snake_case.
func structJSONTagViolations(relPath string, structType *ast.StructType) []string {
	var violations []string

	for _, field := range structType.Fields.List {
		if field.Tag == nil {
			continue
		}
		tagValue := strings.Trim(field.Tag.Value, "`")
		jsonTag := extractJSONTag(tagValue)
		if jsonTag == "" || isSnakeCase(jsonTag) {
			continue
		}

		fieldName := ""
		if len(field.Names) > 0 {
			fieldName = field.Names[0].Name
		}
		violations = append(violations, fmt.Sprintf(
			"%s: field '%s' json tag '%s' should be '%s'",
			relPath, fieldName, jsonTag, toSnakeCase(jsonTag),
		))
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

// ModelActionTypeNaming holds explicit DSL Payload type names to the Req
// suffix and Result type names to Rsp.
var ModelActionTypeNaming = Check{
	Name: "Model action type naming",
	Rule: "explicit DSL Payload types must end with Req and Result types with Rsp",
	run:  checkModelActionTypeNaming,
}

// checkModelActionTypeNaming checks explicit DSL Payload and Result type names.
func checkModelActionTypeNaming(ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return violations
	}

	err := walkProjectDir(ggconst.DirModel, ignore, func(path string, info os.FileInfo) error {
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

	relPath := relativePath(filePath)

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

// ModelFileBoundaries allows at most one model struct per model file.
var ModelFileBoundaries = Check{
	Name: "Model file boundaries",
	Rule: "model files must contain at most one model struct",
	run:  checkModelFileBoundaries,
}

// checkModelFileBoundaries checks that each model file contains at most one model struct.
func checkModelFileBoundaries(ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return violations
	}

	err := walkProjectDir(ggconst.DirModel, ignore, func(path string, info os.FileInfo) error {
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

// ServiceFileBoundaries allows at most one service struct per service file.
var ServiceFileBoundaries = Check{
	Name: "Service file boundaries",
	Rule: "service files must contain at most one service struct",
	run:  checkServiceFileBoundaries,
}

// checkServiceFileBoundaries checks that each service file contains at most one service struct.
func checkServiceFileBoundaries(ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirService); os.IsNotExist(err) {
		return violations
	}

	err := walkProjectDir(ggconst.DirService, ignore, func(path string, info os.FileInfo) error {
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
				if goast.IsServiceBase(node, field) {
					names = append(names, typeSpec.Name.Name)
					break
				}
			}
		}
	}

	return names
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

// ModelPackageNaming holds model package names to their directory names.
var ModelPackageNaming = Check{
	Name: "Model package naming",
	Rule: "model package names must match their directory names",
	run:  checkModelPackageNaming,
}

// checkModelPackageNaming checks if model package names match their directory names
func checkModelPackageNaming(ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return violations
	}

	err := walkProjectDir(ggconst.DirModel, ignore, func(path string, info os.FileInfo) error {
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
			relativePath, _ := filepath.Rel(ggconst.DirModel, path)
			violations = append(violations, fmt.Sprintf("%s: package name '%s' should match directory name '%s'", relativePath, packageName, dirName))
		}

		return nil
	})
	if err != nil {
		violations = append(violations, fmt.Sprintf("walking model directory: %v", err))
	}

	return violations
}

// DirectoryRestrictions limits the top-level directories to the ones the
// framework layout names and the ones holding no Go package.
var DirectoryRestrictions = Check{
	Name: "Directory restrictions",
	Rule: "top-level directories must be ones the framework layout names, such as model, service, router and dao, or hold no Go packages, such as logs and deploy",
	run:  checkDirectoryRestrictions,
}

// checkDirectoryRestrictions checks if only allowed directories exist in the project
func checkDirectoryRestrictions(ignore gitignore.Matcher) []string {
	projectDir := "."
	var violations []string

	// Check if this is a gst framework project by reading go.mod
	if gghelper.IsFrameworkProject(projectDir) {
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
		"leader":     true,
		"lock":       true,
		"component":  true,
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
		"test":       true,
		"docs":       true,
		"doc":        true,
	}

	// Directories that hold no Go packages: build and log output, and the
	// conventional homes of deployment manifests and Helm charts, operator
	// scripts and development scripts.
	whitelistDirs := map[string]bool{
		"tmp":       true,
		"logs":      true,
		"dist":      true,
		"generated": true,
		"deploy":    true,
		"charts":    true,
		"scripts":   true,
		"hack":      true,
	}

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
		if isIgnoredProjectPath(ignore, dirName, true) {
			continue
		}

		// Check if directory is allowed
		if !allowedDirs[dirName] && !whitelistDirs[dirName] {
			violations = append(violations, fmt.Sprintf("Directory '%s' is not allowed in project structure", dirName))
		}
	}

	return violations
}

// newProjectIgnoreMatcher loads Git ignore rules for the project root. Every
// check walks the project from its root, so the root is not a parameter.
// Building a matcher scans the whole worktree for ignore files, so
// Run builds one and shares it across every check it runs.
func newProjectIgnoreMatcher() gitignore.Matcher {
	patterns, err := gitignore.ReadPatterns(osfs.New("."), nil)
	if err != nil || len(patterns) == 0 {
		return nil
	}
	return gitignore.NewMatcher(patterns)
}

// isIgnoredProjectPath reports whether a project-relative path is ignored by
// Git rules.
func isIgnoredProjectPath(matcher gitignore.Matcher, path string, isDir bool) bool {
	if matcher == nil {
		return false
	}
	return matcher.Match(strings.Split(path, string(filepath.Separator)), isDir)
}

// walkProjectDir walks root while pruning paths ignored by Git rules, so
// runtime artifacts such as the log directories a test run leaves behind
// never reach checkFn. Walk errors abort the walk instead of being delegated,
// so checkFn only sees paths that exist.
func walkProjectDir(root string, ignore gitignore.Matcher, checkFn func(path string, info os.FileInfo) error) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path != root && isIgnoredProjectPath(ignore, path, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		return checkFn(path, info)
	})
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

// DSLDesignRules runs the Design() validation that gates gg gen over every
// model file.
var DSLDesignRules = Check{
	Name: "DSL design rules",
	Rule: "model files must pass the same validation rules that gate gg gen: the Design() DSL rules, and the base types model.Base, model.AutoBase and model.Empty embedded by value, never through a pointer",
	run:  checkDSLDesignRules,
}

// checkDSLDesignRules runs DSL Design() validation on every model file, so keyword
// placement and generation-semantic violations fail gg check with the same
// rules that block gg gen.
func checkDSLDesignRules(ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return violations
	}

	err := walkProjectDir(ggconst.DirModel, ignore, func(path string, info os.FileInfo) error {
		base := filepath.Base(path)
		if info.IsDir() {
			if path != ggconst.DirModel && (base == "vendor" || base == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(base, ".go") ||
			strings.HasSuffix(base, "_test.go") ||
			strings.HasPrefix(base, "_") {
			return nil
		}

		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			violations = append(violations, fmt.Sprintf("%s: %v", path, parseErr))
			return nil
		}
		for _, validateErr := range dsl.Validate(file, ggconst.DirModel, path) {
			violations = append(violations, validateErr.Error())
		}
		return nil
	})
	if err != nil {
		violations = append(violations, err.Error())
	}

	return violations
}

// fileExists reports whether a file exists at filename.
func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

// isGeneratedFileName reports whether a path is a file gg generates and owns.
func isGeneratedFileName(path string) bool {
	return strings.HasSuffix(path, ggconst.SuffixGenGo)
}
