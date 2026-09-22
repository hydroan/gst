package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/ds/tree/trie"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/codegen"
	"github.com/hydroan/gst/internal/codegen/gen"
	pkgnew "github.com/hydroan/gst/internal/codegen/new"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/spf13/cobra"
)

var genCmd = &cobra.Command{
	Use:   "gen",
	Short: "generate service code",
	Run: func(cmd *cobra.Command, args []string) {
		genRun()
	},
}

type genRunOptions struct {
	Quiet bool
	// BaselineViolations lists project check violations that already existed
	// before the caller started. The quiet pre-generation checks fail only on
	// violations outside this baseline; module copy uses it so pre-existing
	// project issues do not block copying an unrelated module.
	BaselineViolations map[string]struct{}
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

	// Heal bare model.Version declarations before the checks run: the tag
	// shape is framework-owned, and gen filling it in is what keeps the
	// version-field check below from failing the very command that fixes it.
	if err := fillVersionFieldTags(opts.Quiet); err != nil {
		return err
	}

	if runProjectChecks(opts.Quiet, opts.BaselineViolations) > 0 {
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

	scanned, err := scanModels(opts.Quiet)
	if err != nil {
		return err
	}
	allModels, ignoreResult := scanned.models, scanned.routeIgnores

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
	// The packages each registration file imports, every import path mapped
	// to the name of the package it declares.
	modelPkgs := make(map[string]string)
	routerPkgs := make(map[string]string)
	servicePkgs := make(map[string]string)
	writeGenFile := func(filename string, content string) error {
		if opts.Quiet {
			return writeGeneratedFile(filename, content, false)
		}
		writeFileWithLog(filename, content)
		return nil
	}

	for _, m := range allModels {
		// The model registration file belongs to the root model package; a
		// model anywhere else is registered through an import of its package.
		if m.Design.Enabled && m.Design.Migrate && !m.InModelRoot(modelDir) {
			modelPkgs[m.ImportPath()] = m.ModelPkgName
		}

		m.Design.Range(func(s string, a *dsl.Action) {
			if a.Service {
				target := gen.ServiceTarget(m, a, modelDir, serviceDir)
				servicePkgs[target.ImportPath] = target.PackageName
			}
			routerPkgs[m.ImportPath()] = m.ModelPkgName
		})
	}

	// A dsl.PayloadEmpty side is emitted as *model.Empty (or *gstmodel.Empty
	// when a routed business model package is itself named "model"), so the
	// qualifier and the import are decided once per router file.
	gstModelPkg, gstModelNeeded := gen.RouterGstModelUse(allModels)
	routerGstModelPkg := ""
	if gstModelNeeded {
		routerGstModelPkg = gstModelPkg
	}
	// A package whose name another import or a framework import of the same
	// file already takes is imported under an alias, and its registrations
	// refer to it through the alias.
	modelAliases := gen.ModelFileAliases(modelPkgs)
	serviceAliases := gen.ServiceFileAliases(servicePkgs)
	routerAliases := gen.RouterFileAliases(routerPkgs, routerGstModelPkg)

	for _, m := range allModels {
		if !m.Design.Enabled || !m.Design.Migrate {
			continue
		}
		// A model in the root model package registers unqualified, as in
		// "Register[*Record]()"; any other is qualified by the name its
		// package is imported under, as in "Register[*sample.Record]()".
		if m.InModelRoot(modelDir) {
			modelStmts = append(modelStmts, gen.StmtModelRegister(m.ModelName))
		} else {
			modelStmts = append(modelStmts, gen.StmtModelRegister(importQualifier(modelAliases, m.ImportPath(), m.ModelPkgName)+"."+m.ModelName))
		}
	}
	for _, m := range allModels {
		m.Design.Range(func(route string, act *dsl.Action) {
			// Both registrations below must carry this exact route string:
			// the service registry keys services by route and phase, so the
			// service side and the router side share one route value.
			route, paramName := routerTargetForAction(route, m.Design, act)

			if act.Service {
				target := gen.ServiceTarget(m, act, modelDir, serviceDir)
				serviceStmts = append(serviceStmts, gen.StmtServiceRegister(importQualifier(serviceAliases, target.ImportPath, target.PackageName)+"."+act.RoleName(), act.Phase, route))
			}
			base := "Auth"
			if act.Public {
				base = "Pub"
			}
			routerStmts = append(routerStmts, gen.StmtRouterRegister(importQualifier(routerAliases, m.ImportPath(), m.ModelPkgName), m.ModelName, act.Payload, act.Result, gstModelPkg, base, route, paramName, act.Phase.MethodName()))
		})
	}

	// ============================================================
	// Generate model/service/router/main files
	// ============================================================
	if !opts.Quiet {
		clioutput.Section("Generate Files")
	}
	modelCode, err := gen.BuildModelFile("model", modelAliases, modelStmts...)
	if err != nil {
		return errors.Wrap(err, "build model/model.gen.go")
	}
	if writeErr := writeGenFile(filepath.Join(modelDir, ggconst.FileModelGen), modelCode); writeErr != nil {
		return writeErr
	}

	// generate model/apidoc.gen.go, which registers struct and field doc comments
	// so the OpenAPI document keeps schema descriptions in binaries deployed
	// without Go source files.
	docEntries, err := codegen.ExtractAPIDocs(module, modelDir, excludes)
	if err != nil {
		return errors.Wrap(err, "extract api docs")
	}
	apidocCode, err := gen.BuildAPIDocFile("model", docEntries)
	if err != nil {
		return errors.Wrap(err, "build model/apidoc.gen.go")
	}
	if writeErr := writeGenFile(filepath.Join(modelDir, ggconst.FileAPIDocGen), apidocCode); writeErr != nil {
		return writeErr
	}

	// generate service/service.gen.go
	serviceCode, err := gen.BuildServiceFile("service", serviceAliases, serviceStmts...)
	if err != nil {
		return errors.Wrap(err, "build service/service.gen.go")
	}
	if writeErr := writeGenFile(filepath.Join(serviceDir, ggconst.FileServiceGen), serviceCode); writeErr != nil {
		return writeErr
	}

	// generate router/router.gen.go
	routerCode, err := gen.BuildRouterFile("router", routerGstModelPkg, routerAliases, routerStmts...)
	if err != nil {
		return errors.Wrap(err, "build router/router.gen.go")
	}
	if writeErr := writeGenFile(filepath.Join(routerDir, ggconst.FileRouterGen), routerCode); writeErr != nil {
		return writeErr
	}

	// Generate the typed column references of every model, so filters can
	// name columns through the compiler instead of through string literals.
	if genErr := generateColumnFiles(module, modelDir, allModels, opts.Quiet); genErr != nil {
		return genErr
	}

	// generate main.go
	mainCode, err := gen.BuildMainFile(module)
	if err != nil {
		return errors.Wrap(err, "build main.go")
	}
	if err := writeGenFile(ggconst.FileMain, mainCode); err != nil {
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

			// Apply changes and sync model imports to handle import path and package name updates
			changed, err := gen.ApplyServiceFileWithModelSync(f, action, servicePkgName, modelDir, modelInfo)
			if err != nil {
				return errors.Wrapf(err, "service file %s", safePath)
			}
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
				if err := os.WriteFile(safePath, []byte(code), ggconst.FileModeGenerated); err != nil {
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
			if err := os.WriteFile(safePath, []byte(code), ggconst.FileModeGenerated); err != nil {
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
			if file := gen.GenerateService(m, act, act.Phase, target.PackageName); file != nil {
				fset := token.NewFileSet()
				code, err := gen.FormatNodeExtraWithFileSet(file, fset)
				if err != nil {
					applyErr = err
					return
				}
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
		pruneServiceFiles(oldServiceFiles, allModels, ignoreResult.KeptServiceFiles, ignoreResult.KeptServiceDirs)
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

// scannedModels is the model set code generation works from.
type scannedModels struct {
	models []*gen.ModelInfo
	// routeIgnores records the actions the gst.yaml route ignores disabled,
	// with the service files pruning must keep for them.
	routeIgnores routeIgnoreResult
}

// scanModels reads the models of the model directory and resolves their
// routes: hierarchical endpoints and parent params are applied, then the
// gst.yaml route and model ignores. Route ignores apply before anything reads
// the actions, so a matched action behaves exactly like an action that was
// never declared. gg gen and gg gen ts both start from here, which keeps the
// TypeScript declarations on the routes the generated router registers.
func scanModels(quiet bool) (scannedModels, error) {
	if !fileExists(modelDir) {
		return scannedModels{}, fmt.Errorf("model dir not found: %s", modelDir)
	}

	if !quiet {
		clioutput.Section("Scan Models")
	}
	allModels, err := codegen.FindModels(module, modelDir, excludes)
	if err != nil {
		return scannedModels{}, err
	}
	buildHierarchicalEndpoints(allModels)
	propagateParentParams(allModels)

	projectCfg, err := ggconfig.Load(".")
	if err != nil {
		return scannedModels{}, err
	}
	ignoreResult := applyRouteIgnores(allModels, projectCfg.Gen.Routes.Ignore)
	if !quiet && len(ignoreResult.Matches) > 0 {
		clioutput.Section("Ignore Routes")
		for _, match := range ignoreResult.Matches {
			clioutput.Item("IGNORE", "%s %s (%s)", match.Method, match.Path, match.Model)
		}
	}
	reportRouteIgnoreWarnings(ignoreResult)

	// Model ignores run after route ignores so the live-action warning sees
	// the final enabled-action set.
	modelIgnores := applyModelIgnores(allModels, projectCfg.Gen.Models.Ignore)
	if !quiet && len(modelIgnores.Matches) > 0 {
		clioutput.Section("Ignore Models")
		for _, match := range modelIgnores.Matches {
			clioutput.Item("IGNORE", "model %s (%s)", match.Model, match.File)
		}
	}
	reportModelIgnoreWarnings(modelIgnores)

	return scannedModels{models: allModels, routeIgnores: ignoreResult}, nil
}

// importQualifier returns the name a generated registration file refers to
// the package at importPath by: the alias aliases maps it to, or its package
// name pkgName when it was imported without one.
func importQualifier(aliases map[string]string, importPath, pkgName string) string {
	if alias := aliases[importPath]; alias != "" {
		return alias
	}
	return pkgName
}

func routerTargetForAction(route string, design *dsl.Design, action *dsl.Action) (string, string) {
	if action == nil {
		return route, ""
	}

	if action.Exact {
		return route, routerPathParamName(route)
	}

	paramName := ""

	// If the phase is matched, the route appends the param, eg:
	// route "tenant" with param ":tenant" becomes "tenant/:tenant"
	// route "tenant" with param ":id" becomes "tenant/:id"
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
//   - model/sample.go with Endpoint("samples") -> samples
//   - model/sample/item.go with Endpoint("items") -> samples/items
//   - model/sample/item/entry.go with Endpoint("entries") -> samples/items/entries
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
}

// propagateParentParams propagates the parameter of every parent resource into
// the endpoints of its descendants, so a nested resource is addressed inside the
// scope of the resources it belongs to. The endpoints are organized in a trie,
// which hands each of them its ancestors in one lookup.
//
// For example, with model/sample.go declaring Endpoint("samples") and
// Param("sample"), model/sample/item.go declaring Endpoint("items") and
// Param("item"), and model/sample/item/entry.go declaring Endpoint("entries"),
// the endpoints
//
//	samples, samples/items, samples/items/entries
//
// become
//
//	samples, samples/:sample/items, samples/:sample/items/:item/entries
//
// so the last one registers routes such as GET and POST
// /api/samples/:sample/items/:item/entries.
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
		// e.g., "samples/items/entries" -> ["samples", "items", "entries"]
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
