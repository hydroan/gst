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
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/goast"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "check the project against the framework's conventions",
	Long:  projectCheckHelp(),
	Run: func(cmd *cobra.Command, args []string) {
		checkRun()
	},
}

// projectCheck is one gg check rule: the name its result prints under, the
// rule the help states for it, and the check that finds its violations.
type projectCheck struct {
	name  string
	rule  string
	check func(gitignore.Matcher) []string
}

// projectChecks lists every gg check rule in the order gg check runs them,
// prints their results and numbers them in its help.
var projectChecks = []projectCheck{
	{
		name:  "Architecture dependencies",
		rule:  "service code must not call other service code, dao code must not call service, router, controller or middleware code, and model code must not call service or dao code",
		check: CheckArchitectureDependency,
	},
	{
		name:  "Model singular naming",
		rule:  "model directories and files must be singular",
		check: CheckModelSingularNaming,
	},
	{
		name:  "Model file name hyphens",
		rule:  "model file names must not contain hyphens (use underscores instead)",
		check: CheckModelFileNameHyphens,
	},
	{
		name:  "JSON tag naming",
		rule:  "model struct and explicit DSL Payload/Result type json tags must use snake_case naming",
		check: CheckJSONTagNaming,
	},
	{
		name:  "Model action type naming",
		rule:  "explicit DSL Payload types must end with Req and Result types with Rsp",
		check: CheckModelActionTypeNaming,
	},
	{
		name:  "Action type form",
		rule:  "explicit DSL Payload/Result types must be named types declared in the same model package: struct types use the pointer form, slice and map types use the value form, an empty struct type may only pair with an empty peer side, and a Payload type is never an interface with methods",
		check: CheckActionTypeForm,
	},
	{
		name:  "Model file boundaries",
		rule:  "model files must contain at most one model struct",
		check: CheckModelFileBoundary,
	},
	{
		name:  "Service file boundaries",
		rule:  "service files must contain at most one service struct",
		check: CheckServiceFileBoundary,
	},
	{
		name:  "Model package naming",
		rule:  "model package names must match their directory names",
		check: CheckModelPackageNaming,
	},
	{
		name:  "Directory restrictions",
		rule:  "top-level directories must be ones the framework layout names, such as model, service, router and dao, or hold no Go packages, such as logs and deploy",
		check: CheckAllowedDirectories,
	},
	{
		name:  "DSL design rules",
		rule:  "model files must pass the same validation rules that gate gg gen: the Design() DSL rules, and the base types model.Base, model.AutoBase and model.Empty embedded by value, never through a pointer",
		check: CheckDSLDesign,
	},
	{
		name:  "Database chain termination",
		rule:  "database.Database operation chains must end with a terminal operation inline or be passed directly as a call argument",
		check: CheckDatabaseChainTermination,
	},
	{
		name:  "Transaction closure context",
		rule:  "database.Database chains and nested database.Transaction calls inside a database.Transaction closure must use the closure's context parameter",
		check: CheckTransactionClosureContext,
	},
	{
		name:  "Detached context",
		rule:  "in service, dao, cronjob, leader, lock, component and router code, the context passed to a framework database function or to a dao function must not be context.Background() or context.TODO(): the context handed down — a request's, a round's, a tenure's, a lock's, the process's, the start's — carries the identity, the transaction and the lease the work runs under; startup seeding runs in the router package's routes-ready hooks on the context the hook receives",
		check: CheckDetachedContext,
	},
	{
		name:  "Service error discipline",
		rule:  "errors leaving service methods must be built by service.NewError or service.NewErrorWithCause",
		check: CheckServiceErrorDiscipline,
	},
	{
		name:  "Service test coverage",
		rule:  "service files generated for DSL Service() actions must have a matching test file (create.go pairs with create_test.go or create_internal_test.go)",
		check: CheckServiceTestCoverage,
	},
	{
		name:  "Service test organization",
		rule:  "test files under the service directory must pair with a source file of their package; main_test.go only declares TestMain, fixtures_test.go only holds shared test fixtures, and test cases without a source file of their own belong in the test file of a related source file",
		check: CheckServiceTestOrganization,
	},
	{
		name:  "Log field boundedness",
		rule:  "project code must not declare MarshalLogObject or MarshalLogArray methods and must not call zap.Namespace: zapcore marshalers and nested namespaces bypass the reflected-value collapsing that keeps log-store field mappings bounded",
		check: CheckLogFieldBoundedness,
	},
	{
		name:  "Model table name declaration",
		rule:  "model structs embedding model.Base or model.AutoBase must declare TableName() string on the struct itself, as a single return of a non-empty string literal",
		check: CheckModelTableNameDeclaration,
	},
	{
		name:  "Gorm tag index ban",
		rule:  "gorm struct tags must not configure indexes (index, uniqueIndex, unique); models declare indexes through the Indexes() []model.Index method",
		check: CheckGormTagIndexBan,
	},
	{
		name:  "Version field declaration",
		rule:  `model.Version declarations must keep the optimistic-locking shape: on database models a named field with json:",omitempty" and gorm:"not null;default:1", and on DSL Payload/Result types (plus the same-package types reachable from their fields) a json tag of exactly "version,omitempty"`,
		check: CheckVersionFieldDeclarations,
	},
	{
		name:  "Module assembly",
		rule:  "the assembly calls a copied framework module declares in its module.json must be made in the project's non-test code, outside the model and service subtrees the module was copied into",
		check: CheckModuleAssembly,
	},
	{
		name:  "Column reference minting",
		rule:  "project code, tests included, must not mint column references through gst.NewColumn, NewNumericColumn or NewTimeColumn; columns are read through the XxxCols variables gg gen writes, which the model schema checks, generated files excepted. Generic code, which has no Cols variable to read, may mint a reference whose model is its own type parameter",
		check: CheckColumnReferenceMinting,
	},
}

// projectCheckSkips closes the gg check help: what the checks leave out.
const projectCheckSkips = `Model and service subtrees owned by copyable framework modules are skipped by Service test coverage, Service test organization, Log field boundedness, Model table name declaration, Gorm tag index ban, Version field declaration and Column reference minting, and their service subtrees by Detached context: copied module code is tested inside the framework repository.
Paths ignored by the project's Git ignore rules are skipped by every check, so runtime artifacts such as log directories never fail checks.`

// projectCheckHelp renders the gg check help from projectChecks: one numbered
// line per check, in the order gg check runs them and under the name its
// result prints, then projectCheckSkips. The help starts
//
//	Check the project against the framework's conventions:
//	1. Architecture dependencies: service code must not call other service code, dao code must not call service, router, controller or middleware code, and model code must not call service or dao code
//	2. Model singular naming: model directories and files must be singular
//	3. Model file name hyphens: model file names must not contain hyphens (use underscores instead)
func projectCheckHelp() string {
	var b strings.Builder
	b.WriteString("Check the project against the framework's conventions:\n")
	for i, pc := range projectChecks {
		fmt.Fprintf(&b, "%d. %s: %s\n", i+1, pc.name, pc.rule)
	}
	b.WriteString("\n")
	b.WriteString(projectCheckSkips)
	return b.String()
}

func checkRun() {
	totalViolations := runProjectChecks(false, nil)

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

// runProjectChecks runs every project check, shared by gg check and gg gen.
//
// quiet suppresses output when the project is clean; violations always
// print. Violations recorded in baseline are treated as pre-existing and
// are neither counted nor printed, so callers such as module copy fail only
// on violations introduced after the baseline snapshot. A nil baseline
// keeps the full check behavior.
func runProjectChecks(quiet bool, baseline map[string]struct{}) int {
	results := filterProjectCheckResults(collectProjectChecks(), baseline)
	total := totalProjectCheckViolations(results)
	if !quiet || total > 0 {
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
	// One matcher serves every check: building it scans the whole worktree
	// for ignore files, which is too expensive to repeat per check.
	ignore := newProjectIgnoreMatcher()
	results := make([]projectCheckResult, 0, len(projectChecks))
	for _, pc := range projectChecks {
		results = append(results, projectCheckResult{Name: pc.name, Violations: pc.check(ignore)})
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
func CheckArchitectureDependency(ignore gitignore.Matcher) []string {
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

	if _, err := os.Stat(serviceDir); os.IsNotExist(err) {
		return violations
	}

	err := walkProjectDir(serviceDir, ignore, func(path string, _ os.FileInfo) error {
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

	if _, err := os.Stat(daoDir); os.IsNotExist(err) {
		return violations
	}

	err := walkProjectDir(daoDir, ignore, func(path string, _ os.FileInfo) error {
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

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return violations
	}

	err := walkProjectDir(modelDir, ignore, func(path string, _ os.FileInfo) error {
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

// CheckModelSingularNaming checks that model directories and files use
// singular names.
func CheckModelSingularNaming(ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
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

	err := walkProjectDir(modelDir, ignore, func(path string, info os.FileInfo) error {
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

// CheckModelFileNameHyphens checks that model file names separate words with
// underscores rather than hyphens.
func CheckModelFileNameHyphens(ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return violations
	}

	err := walkProjectDir(modelDir, ignore, func(path string, info os.FileInfo) error {
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

// CheckJSONTagNaming checks that json tags declared under the model directory
// use snake_case naming. It covers model structs and the explicit DSL Payload
// and Result types referenced by Design methods.
func CheckJSONTagNaming(ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return violations
	}

	// Files are grouped per directory because an explicit DSL action type may
	// be declared in a different file of the package that references it.
	var packageDirs []string
	packageFiles := make(map[string][]string)
	err := walkProjectDir(modelDir, ignore, func(path string, info os.FileInfo) error {
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

// CheckModelActionTypeNaming checks explicit DSL Payload and Result type names.
func CheckModelActionTypeNaming(ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return violations
	}

	err := walkProjectDir(modelDir, ignore, func(path string, info os.FileInfo) error {
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

// CheckModelFileBoundary checks that each model file contains at most one model struct.
func CheckModelFileBoundary(ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return violations
	}

	err := walkProjectDir(modelDir, ignore, func(path string, info os.FileInfo) error {
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

// CheckServiceFileBoundary checks that each service file contains at most one service struct.
func CheckServiceFileBoundary(ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(serviceDir); os.IsNotExist(err) {
		return violations
	}

	err := walkProjectDir(serviceDir, ignore, func(path string, info os.FileInfo) error {
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

// CheckModelPackageNaming checks if model package names match their directory names
func CheckModelPackageNaming(ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return violations
	}

	err := walkProjectDir(modelDir, ignore, func(path string, info os.FileInfo) error {
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

		// Go convention discourages underscores in package names, so the
		// directory name is compared without them.
		expectedName := gen.ModelPackageName(dirName)

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
func CheckAllowedDirectories(ignore gitignore.Matcher) []string {
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
// collectProjectChecks builds one and shares it across every check.
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
func CheckDSLDesign(ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return violations
	}

	err := walkProjectDir(modelDir, ignore, func(path string, info os.FileInfo) error {
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

// isGeneratedFileName reports whether a path is a file gg generates and owns.
func isGeneratedFileName(path string) bool {
	return strings.HasSuffix(path, ggconst.SuffixGenGo)
}
