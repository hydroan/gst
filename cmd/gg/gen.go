package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/hydroan/gst/ds/tree/trie"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/codegen"
	"github.com/hydroan/gst/internal/codegen/gen"
	pkgnew "github.com/hydroan/gst/internal/codegen/new"
	"github.com/hydroan/gst/types/consts"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

var genCmd = &cobra.Command{
	Use:   "gen",
	Short: "generate service code",
	Run: func(cmd *cobra.Command, args []string) {
		genRun()
	},
}

var tsCmd = &cobra.Command{
	Use:   "ts",
	Short: "generate typescript interface code",
	Run: func(cmd *cobra.Command, args []string) {
		clioutput.Warn("", "TypeScript generation is not implemented yet")
	},
}

func init() {
	genCmd.AddCommand(tsCmd)
}

type genRunOptions struct {
	Quiet bool
}

func genRun() {
	if err := genRunWithOptions(genRunOptions{}); err != nil {
		clioutput.Error("", "%v", err)
		os.Exit(1)
	}
}

func genRunWithOptions(opts genRunOptions) error {
	if cleanOrphans && !prune {
		return errors.New("--clean-orphans requires --prune when used with gg gen")
	}

	if len(module) == 0 {
		var err error
		module, err = gen.GetModulePath()
		if err != nil {
			return err
		}
	}

	checks := runProjectChecks
	if opts.Quiet {
		checks = runProjectChecksQuiet
	}
	if checks() > 0 {
		return errors.New("project checks failed")
	}

	// Ensure required files exist
	if !opts.Quiet {
		clioutput.Section("Ensure Required Files")
	}
	createdFiles, err := pkgnew.EnsureFileExists()
	if err != nil {
		return err
	}
	if !opts.Quiet {
		if len(createdFiles) == 0 {
			clioutput.Success("", "Required files are present")
		} else {
			for _, file := range createdFiles {
				clioutput.Success("CREATE", "%s", file)
			}
		}
	}

	if !fileExists(modelDir) {
		return fmt.Errorf("model dir not found: %s", modelDir)
	}

	// Scan all models
	if !opts.Quiet {
		clioutput.Section("Scan Models")
	}
	allModels, err := codegen.FindModels(module, modelDir, serviceDir, excludes)
	if err != nil {
		return err
	}
	buildHierarchicalEndpoints(allModels)
	propagateParentParams(allModels)

	// Record old service files list (if prune option is enabled)
	var oldServiceFiles []string
	if prune {
		oldServiceFiles = scanExistingServiceFiles(serviceDir)
	}

	if !opts.Quiet {
		if len(allModels) == 0 {
			clioutput.Item("", "No models found, generating empty registration files")
		} else {
			clioutput.Success("", "%d models found", len(allModels))
		}
	}

	modelStmts := make([]ast.Stmt, 0)
	serviceStmts := make([]ast.Stmt, 0)
	routerStmts := make([]ast.Stmt, 0)
	modelImportMap := make(map[string]struct{})
	routerImportMap := make(map[string]struct{})
	serviceImportMap := make(map[string]struct{})
	writeGenFile := func(filename string, content string) error {
		if opts.Quiet {
			return writeGeneratedFile(filename, content, false)
		}
		writeFileWithLog(filename, content)
		return nil
	}

	for _, m := range allModels {
		if m.Design.Enabled && m.Design.Migrate {
			// If the ModelFileDir is "model" or "model/", the model package name is the same as the model name,
			// and the statement in model/model.go will be "Register[*Project]()".
			// otherwise, the model package name is the last segment of the model file dir.
			//
			// For example:
			// If the ModelFileDir is "model/setting", the model package name is "setting",
			// then the statement in model/model.go should be "Register[*setting.Project]()"
			if m.ModelPkgName == strings.TrimRight(m.ModelFileDir, "/") {
				modelStmts = append(modelStmts, gen.StmtModelRegister(m.ModelName))
			} else {
				modelStmts = append(modelStmts, gen.StmtModelRegister(fmt.Sprintf("%s.%s", m.ModelPkgName, m.ModelName)))
			}

			if path, shouldImport := m.ModelImportPath(); shouldImport {
				modelImportMap[path] = struct{}{}
			}
		}

		m.Design.Range(func(s string, a *dsl.Action) {
			if a.Service {
				target := gen.ServiceTarget(m, a, modelDir, serviceDir)
				serviceImportMap[target.ImportPath] = struct{}{}
			}
			routerImportMap[m.RouterImportPath()] = struct{}{}
		})
	}

	// Resolve import conflicts
	serviceImports := lo.Keys(serviceImportMap)
	sort.Strings(serviceImports)
	serviceAliasMap := gen.ResolveImportConflicts(serviceImports)
	for _, m := range allModels {
		m.Design.Range(func(route string, act *dsl.Action) {
			if act.Service {
				target := gen.ServiceTarget(m, act, modelDir, serviceDir)
				if alias := serviceAliasMap[target.ImportPath]; len(alias) > 0 {
					// alias import package, eg:
					// pkg1_user "service/pkg1/user"
					// pkg2_user "service/pkg2/user"
					serviceStmts = append(serviceStmts, gen.StmtServiceRegister(fmt.Sprintf("%s.%s", alias, act.RoleName()), act.Phase))
				} else {
					serviceStmts = append(serviceStmts, gen.StmtServiceRegister(fmt.Sprintf("%s.%s", target.PackageName, act.RoleName()), act.Phase))
				}
			}
			base := "Auth"
			if act.Public {
				base = "Pub"
			}
			route, paramName := routerTargetForAction(route, m.Design, act)
			routerStmts = append(routerStmts, gen.StmtRouterRegister(m.ModelPkgName, m.ModelName, act.Payload, act.Result, base, route, paramName, act.Phase.MethodName()))
		})
	}

	// ============================================================
	// Generate model/service/router/main files
	// ============================================================
	if !opts.Quiet {
		clioutput.Section("Generate Files")
	}
	modelImports := lo.Keys(modelImportMap)
	sort.Strings(modelImports)
	modelCode, err := gen.BuildModelFile("model", modelImports, modelStmts...)
	if err != nil {
		return err
	}
	if writeErr := writeGenFile(filepath.Join(modelDir, "model.go"), modelCode); writeErr != nil {
		return writeErr
	}

	// generate service/service.go
	serviceImports = lo.Keys(serviceImportMap)
	sort.Strings(serviceImports)
	serviceCode, err := gen.BuildServiceFile("service", serviceImports, serviceStmts...)
	if err != nil {
		return err
	}
	if writeErr := writeGenFile(filepath.Join(serviceDir, "service.go"), serviceCode); writeErr != nil {
		return writeErr
	}

	// generate router/router.go
	// router always imports "github.com/hydroan/gst/types"
	routerImportMap["github.com/hydroan/gst/types"] = struct{}{}
	routerImports := lo.Keys(routerImportMap)
	sort.Strings(routerImports)
	routerCode, err := gen.BuildRouterFile("router", routerImports, routerStmts...)
	if err != nil {
		return err
	}
	if writeErr := writeGenFile(filepath.Join(routerDir, "router.go"), routerCode); writeErr != nil {
		return writeErr
	}

	// generate main.go
	mainCode, err := gen.BuildMainFile(module)
	if err != nil {
		return err
	}
	if err := writeGenFile("main.go", mainCode); err != nil {
		return err
	}

	// ============================================================
	// Apply actions to services
	// ============================================================
	if !opts.Quiet {
		clioutput.Section("Apply Actions To Services")
	}

	fset := token.NewFileSet()
	applyFile := func(filename string, code string, action *dsl.Action, servicePkgName string, modelInfo *gen.ModelInfo) error {
		safePath, err := pathUnderRoot(filename, serviceDir)
		if err != nil {
			return err
		}

		if fileExists(safePath) {
			// Read original file content to preserve comments and formatting
			src, err := os.ReadFile(safePath)
			if err != nil {
				return err
			}
			f, err := parser.ParseFile(fset, safePath, src, parser.ParseComments)
			if err != nil {
				return err
			}

			// Calculate the correct model import path and package name
			correctModelImportPath := filepath.Join(modelInfo.ModulePath, modelInfo.ModelFileDir)
			correctModelPkgName := modelInfo.ModelPkgName

			// Apply changes and sync model imports to handle import path and package name updates
			changed := gen.ApplyServiceFileWithModelSync(f, action, servicePkgName, correctModelImportPath, correctModelPkgName)
			if changed {
				// Only reformat and write file when there are changes
				// Use original FileSet to preserve comment positions
				code, err = gen.FormatNodeExtraWithFileSet(f, fset)
				if err != nil {
					return err
				}
				if !opts.Quiet {
					clioutput.Status(clioutput.StyleWarn, clioutput.SymbolSuccess, "UPDATE", "%s", safePath)
				}
				if err := ensureParentDir(safePath); err != nil {
					return err
				}
				// #nosec G703 -- safePath validated under serviceDir by pathUnderRoot
				if err := os.WriteFile(safePath, []byte(code), 0o600); err != nil {
					return err
				}
			} else if !opts.Quiet {
				clioutput.Item("SKIP", "%s", safePath)
			}
		} else {
			if !opts.Quiet {
				clioutput.Success("CREATE", "%s", safePath)
			}
			if err := ensureParentDir(safePath); err != nil {
				return err
			}
			// #nosec G703 -- safePath validated under serviceDir by pathUnderRoot
			if err := os.WriteFile(safePath, []byte(code), 0o600); err != nil {
				return err
			}
		}
		return nil
	}

	var applyErr error
	for _, m := range allModels {
		m.Design.Range(func(route string, act *dsl.Action) {
			if applyErr != nil {
				return
			}
			target := gen.ServiceTarget(m, act, modelDir, serviceDir)
			if file := gen.GenerateServiceWithPackage(m, act, act.Phase, target.PackageName); file != nil {
				fset := token.NewFileSet()
				code, err := gen.FormatNodeExtraWithFileSet(file, fset)
				// pretty.Println(file)
				if err != nil {
					applyErr = err
					return
				}
				// code = gen.MethodAddComments(code, m.ModelName)
				applyErr = applyFile(target.FilePath, code, act, target.PackageName, m)
			}
		})
		if applyErr != nil {
			return applyErr
		}
	}

	// ============================================================
	// Prune disabled service files
	// ============================================================
	if prune {
		pruneServiceFiles(oldServiceFiles, allModels)
	}

	// ============================================================
	// Completion message
	// ============================================================
	if !opts.Quiet {
		clioutput.Section("Done")
		clioutput.Done("Code generation completed successfully!")
	}
	return nil
}

func routerTargetForAction(route string, design *dsl.Design, action *dsl.Action) (string, string) {
	if action == nil {
		return route, ""
	}

	if action.Exact {
		return route, routerPathParamName(route)
	}

	paramName := ""

	// If the phase is matched, the model endpoint will append the param, eg:
	// Endpoint: tenant, param is ":tenant", new endpoint is "tenant/:tenant"
	// Endpoint: tenant, param is ":id", new endpoint is "tenant/:id"
	switch action.Phase {
	case consts.PHASE_DELETE, consts.PHASE_UPDATE, consts.PHASE_PATCH, consts.PHASE_GET:
		param := ":id"
		if design != nil && len(design.Param) > 0 {
			param = design.Param
		}
		route = filepath.Join(route, param)
		paramName = routerPathParamName(route)
	case consts.PHASE_CREATE_MANY, consts.PHASE_DELETE_MANY, consts.PHASE_UPDATE_MANY, consts.PHASE_PATCH_MANY:
		route = filepath.Join(route, "batch")
	case consts.PHASE_IMPORT:
		route = filepath.Join(route, "import")
	case consts.PHASE_EXPORT:
		route = filepath.Join(route, "export")
	}

	return route, paramName
}

func routerPathParamName(route string) string {
	parts := strings.Split(route, "/")
	for _, part := range slices.Backward(parts) {
		trimmedPart := strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(trimmedPart, ":"):
			name := strings.TrimPrefix(trimmedPart, ":")
			if name != "" {
				return name
			}
		case strings.HasPrefix(trimmedPart, "{") && strings.HasSuffix(trimmedPart, "}"):
			name := strings.TrimSuffix(strings.TrimPrefix(trimmedPart, "{"), "}")
			if name != "" {
				return name
			}
		}
	}
	return ""
}

// pathUnderRoot returns path cleaned and verified to be under root (no path traversal).
// It satisfies gosec G703 by ensuring the write path is constrained to the output directory.
func pathUnderRoot(path, root string) (string, error) {
	path = filepath.Clean(path)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %s is not under root %s", path, root)
	}
	return path, nil
}

// buildHierarchicalEndpoints constructs complete hierarchical endpoint paths for all models.
// It maps directory structures to their corresponding endpoint names and builds full endpoint paths
// by replacing directory names with their custom endpoint names (if defined).
//
// For example:
//   - model/config/namespace.go with Endpoint("namespaces") -> config/namespaces
//   - model/config/namespace/app.go with Endpoint("apps") -> config/namespaces/apps
//   - model/config/namespace/app/env.go with Endpoint("envs") -> config/namespaces/apps/envs
func buildHierarchicalEndpoints(allModels []*gen.ModelInfo) {
	// Create a map to store directory-to-endpoint mappings
	// This will store what endpoint name should be used for each directory
	dirEndpointMap := make(map[string]string)

	// First pass: build directory-to-endpoint mapping
	for _, m := range allModels {
		if m.Design == nil {
			continue
		}

		// Extract directory from model file path
		modelFilePath := strings.TrimPrefix(m.ModelFilePath, "model/")
		modelDir_ := filepath.Dir(modelFilePath)
		if modelDir_ == "." {
			modelDir_ = ""
		}

		// Get the filename without extension
		fileName := strings.TrimSuffix(filepath.Base(modelFilePath), ".go")

		// Determine the directory path that this model defines endpoint for
		// The rule is: model file defines endpoint for the directory path formed by modelDir + fileName
		var targetDir string
		if modelDir_ == "" {
			targetDir = fileName
		} else {
			targetDir = filepath.Join(modelDir_, fileName)
		}

		// Store the endpoint mapping for the target directory
		if m.Design.Endpoint != "" {
			dirEndpointMap[targetDir] = m.Design.Endpoint
		}
	}

	// Second pass: build complete endpoints by replacing directory names with mapped endpoints
	for _, m := range allModels {
		if m.Design == nil {
			continue
		}

		// Extract directory from model file path
		modelFilePath := strings.TrimPrefix(m.ModelFilePath, "model/")
		modelDir_ := filepath.Dir(modelFilePath)
		if modelDir_ == "." {
			modelDir_ = ""
		}

		// Store the original endpoint from DSL
		originalEndpoint := m.Design.Endpoint

		if modelDir_ == "" {
			// Model is in root model directory, keep original endpoint
			continue
		}

		// Build the complete endpoint path by replacing directory names with mapped endpoints
		var endpointParts []string
		pathParts := strings.Split(modelDir_, "/")

		// For each directory level, use mapped endpoint or directory name
		for i := range pathParts {
			currentPath := strings.Join(pathParts[:i+1], "/")
			if mappedEndpoint, exists := dirEndpointMap[currentPath]; exists {
				// Use the mapped endpoint for this directory
				endpointParts = append(endpointParts, mappedEndpoint)
			} else {
				// No mapping found, use directory name
				endpointParts = append(endpointParts, pathParts[i])
			}
		}

		// Add the current model's original endpoint
		endpointParts = append(endpointParts, originalEndpoint)

		// Join all parts to form the complete endpoint
		m.Design.Endpoint = strings.Join(endpointParts, "/")
	}

	// for _, m := range allModels {
	// 	fmt.Println("-----", m.ModelFilePath, "=>", m.Design.Endpoint)
	// }
}

// propagateParentParams propagates parent resource parameters to all child resource endpoints.
// This function uses a trie data structure to efficiently organize and traverse the hierarchical
// endpoint structure, ensuring that parent parameters are correctly inherited by all descendant resources.
//
// When a parent resource defines a parameter (e.g., Param("ns")), all its child resources
// should inherit this parameter in their endpoint paths to maintain proper REST hierarchy.
// This is essential for creating RESTful APIs that follow nested resource patterns.
//
// Real-world usage scenarios:
//
// 1. Kubernetes-style namespace hierarchy:
//
//   - model/config/namespace.go defines Endpoint("namespaces") with Param("ns")
//
//   - model/config/namespace/app.go defines Endpoint("apps") with Param("app")
//
//   - model/config/namespace/app/env.go defines Endpoint("envs")
//
//     Before propagation:
//
//   - config/namespaces (with Param("ns"))
//
//   - config/namespaces/apps (with Param("app"))
//
//   - config/namespaces/apps/envs
//
//     After propagation:
//
//   - config/namespaces
//
//   - config/namespaces/:ns/apps
//
//   - config/namespaces/:ns/apps/:app/envs
//
//     Generated API endpoints:
//     GET    /api/config/namespaces
//     POST   /api/config/namespaces
//     GET    /api/config/namespaces/:ns/apps
//     POST   /api/config/namespaces/:ns/apps
//     GET    /api/config/namespaces/:ns/apps/:app/envs
//     POST   /api/config/namespaces/:ns/apps/:app/envs
//
// 2. Multi-tenant organization structure:
//
//   - model/tenant.go defines Endpoint("tenants") with Param("tenant")
//
//   - model/tenant/project.go defines Endpoint("projects") with Param("project")
//
//   - model/tenant/project/resource.go defines Endpoint("resources")
//
//     Results in endpoints like:
//     /api/tenants/:tenant/projects/:project/resources
//
// 3. E-commerce category hierarchy:
//
//   - model/category.go defines Endpoint("categories") with Param("category")
//
//   - model/category/product.go defines Endpoint("products") with Param("product")
//
//   - model/category/product/variant.go defines Endpoint("variants")
//
//     Results in endpoints like:
//     /api/categories/:category/products/:product/variants
//
// The trie data structure provides several advantages:
// - Efficient hierarchical organization of endpoints
// - O(log n) lookup time for ancestor relationships
// - Natural representation of tree-like endpoint structures
// - Easy parameter propagation through PathAncestors method
//
// This ensures that child resources are properly nested under their parent's parameter scope,
// maintaining RESTful conventions and enabling proper resource identification in nested APIs.
func propagateParentParams(allModels []*gen.ModelInfo) {
	nodeFormater := trie.WithNodeFormatter[string, *gen.ModelInfo](func(v *gen.ModelInfo, depth int, hasValue bool) string {
		if !hasValue || v == nil {
			return "<nil>"
		}
		return fmt.Sprintf("%s (param: %s)", v.Design.Endpoint, v.Design.Param)
	})
	keyFormater := trie.WithKeyFormatter[string, *gen.ModelInfo](func(k string, v *gen.ModelInfo, depth int, hasValue bool) string {
		return k
	})

	// Create a trie tree to organize endpoints hierarchically
	// Key type is string, value type is *gen.ModelInfo
	tree, err := trie.New[string, *gen.ModelInfo](nodeFormater, keyFormater)
	if err != nil {
		panic(err)
	}

	// Build the trie tree
	for _, m := range allModels {
		// Split endpoint into segments for trie insertion
		// e.g., "config/namespaces/apps" -> ["config", "namespaces", "apps"]
		tree.Put(strings.Split(m.Design.Endpoint, "/"), m)
	}

	// Use trie's PathAncestors to collect parameters from all ancestor levels
	for _, model := range allModels {
		// Get all ancestors (including self) for this endpoint
		ancestors := tree.PathAncestors(strings.Split(model.Design.Endpoint, "/"))

		// Build the new endpoint path by inserting parameters from all ancestors
		newPathSegments := make([]string, 0)

		// Process each ancestor to build the hierarchical path with parameters
		// Note: ancestors[len(ancestors)-1] is the model itself, so we exclude it from parameter propagation
		for i, ancestor := range ancestors {
			// Add path segments from this ancestor level
			if i == 0 {
				// First ancestor: add all its path segments
				newPathSegments = append(newPathSegments, ancestor.Keys...)
			} else {
				// Subsequent ancestors: add only the new segments (difference from previous)
				prevAncestor := ancestors[i-1]
				if len(ancestor.Keys) > len(prevAncestor.Keys) {
					// Add the new segments
					newSegments := ancestor.Keys[len(prevAncestor.Keys):]
					newPathSegments = append(newPathSegments, newSegments...)
				}
			}

			// Add the parameter for this ancestor (if it has one)
			// But skip the last ancestor (which is the model itself) to avoid duplicate parameters
			if i < len(ancestors)-1 && ancestor.Value != nil && len(ancestor.Value.Design.Param) > 0 {
				param := ancestor.Value.Design.Param
				newPathSegments = append(newPathSegments, param)
			}
		}

		// Update the model's endpoint with the new path that includes all ancestor parameters
		if len(newPathSegments) > 0 {
			newEndpoint := strings.Join(newPathSegments, "/")
			model.Design.Endpoint = newEndpoint
		}
	}
}
