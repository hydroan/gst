package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/codegen"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/codegen/gen/columns"
	pkgnew "github.com/hydroan/gst/internal/codegen/new"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/spf13/cobra"
)

var genCmd = &cobra.Command{
	Use:   "gen",
	Short: "generate service code",
	Run: func(cmd *cobra.Command, args []string) {
		genRun()
	},
}

// prune is gg gen's --prune flag: once the code is generated, gg gen prunes
// the way gg prune does.
var prune bool

func init() {
	genCmd.Flags().BoolVar(&prune, "prune", false, "After generating, prune what the models no longer need from service/ and middleware/, asking once before deleting")
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
	if len(module) == 0 {
		var err error
		module, err = gghelper.ModulePath()
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

	ignore := gghelper.NewProjectIgnore()
	scanned, err := scanModels(opts.Quiet, ignore)
	if err != nil {
		return err
	}
	allModels, ignoreResult := scanned.models, scanned.routeIgnores

	// Record old service files list (if prune option is enabled)
	var oldServiceFiles []string
	if prune {
		oldServiceFiles = existingServiceFiles()
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
		return writeGeneratedFile(filename, content, !opts.Quiet)
	}

	for _, m := range allModels {
		// The model registration file belongs to the root model package; a
		// model anywhere else is registered through an import of its package.
		if m.Design.Enabled && m.Design.Migrate && !m.InModelRoot(ggconst.DirModel) {
			modelPkgs[m.ImportPath()] = m.ModelPkgName
		}

		m.Design.Range(func(s string, a *dsl.Action) {
			if a.Service {
				target := gen.ServiceTarget(m, a, ggconst.DirModel, ggconst.DirService)
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
		if m.InModelRoot(ggconst.DirModel) {
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
			route, paramName := codegen.RouterTargetForAction(route, m.Design, act)

			if act.Service {
				target := gen.ServiceTarget(m, act, ggconst.DirModel, ggconst.DirService)
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
	if writeErr := writeGenFile(filepath.Join(ggconst.DirModel, ggconst.FileModelGen), modelCode); writeErr != nil {
		return writeErr
	}

	// generate model/apidoc.gen.go, which registers struct and field doc comments
	// so the OpenAPI document keeps schema descriptions in binaries deployed
	// without Go source files.
	docEntries, err := codegen.ExtractAPIDocs(module, ggconst.DirModel, ignore, nil)
	if err != nil {
		return errors.Wrap(err, "extract api docs")
	}
	apidocCode, err := gen.BuildAPIDocFile("model", docEntries)
	if err != nil {
		return errors.Wrap(err, "build model/apidoc.gen.go")
	}
	if writeErr := writeGenFile(filepath.Join(ggconst.DirModel, ggconst.FileAPIDocGen), apidocCode); writeErr != nil {
		return writeErr
	}

	// generate service/service.gen.go
	serviceCode, err := gen.BuildServiceFile("service", serviceAliases, serviceStmts...)
	if err != nil {
		return errors.Wrap(err, "build service/service.gen.go")
	}
	if writeErr := writeGenFile(filepath.Join(ggconst.DirService, ggconst.FileServiceGen), serviceCode); writeErr != nil {
		return writeErr
	}

	// generate router/router.gen.go
	routerCode, err := gen.BuildRouterFile("router", routerGstModelPkg, routerAliases, routerStmts...)
	if err != nil {
		return errors.Wrap(err, "build router/router.gen.go")
	}
	if writeErr := writeGenFile(filepath.Join(ggconst.DirRouter, ggconst.FileRouterGen), routerCode); writeErr != nil {
		return writeErr
	}

	// Generate the typed column references of every model, so filters can
	// name columns through the compiler instead of through string literals.
	columnFiles, genErr := columns.Generate(module, ggconst.DirModel, allModels, ignore)
	if !opts.Quiet {
		for _, path := range columnFiles.Written {
			clioutput.Success("GENERATE", "%s", path)
		}
		for _, path := range columnFiles.Removed {
			clioutput.Success("REMOVE", "%s (model source is gone)", path)
		}
	}
	if genErr != nil {
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
		safePath, err := pathUnderRoot(filename, ggconst.DirService)
		if err != nil {
			return err
		}

		if gghelper.FileExists(safePath) {
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
			changed, err := gen.ApplyServiceFileWithModelSync(f, action, servicePkgName, ggconst.DirModel, modelInfo)
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
				if err := gghelper.EnsureParentDir(safePath); err != nil {
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
			if err := gghelper.EnsureParentDir(safePath); err != nil {
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
			target := gen.ServiceTarget(m, act, ggconst.DirModel, ggconst.DirService)
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
	// Prune what the models no longer need
	// ============================================================
	if prune {
		pruneLeftovers(oldServiceFiles, allModels, ignoreResult.KeptServiceFiles, ignoreResult.KeptServiceDirs, ignore, scanned.pruneConfig)
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
	routeIgnores codegen.RouteIgnoreResult
	// pruneConfig holds the gst.yaml prune settings gg gen --prune applies.
	pruneConfig ggconfig.PruneConfig
}

// scanModels reads the models of the model directory and resolves their
// routes: hierarchical endpoints and parent params are applied, then the
// gst.yaml route and model ignores. Route ignores apply before anything reads
// the actions, so a matched action behaves exactly like an action that was
// never declared. gg gen and gg gen ts both start from here, which keeps the
// TypeScript declarations on the routes the generated router registers.
func scanModels(quiet bool, ignore gghelper.ProjectIgnore) (scannedModels, error) {
	if !gghelper.FileExists(ggconst.DirModel) {
		return scannedModels{}, fmt.Errorf("model dir not found: %s", ggconst.DirModel)
	}

	if !quiet {
		clioutput.Section("Scan Models")
	}
	allModels, err := codegen.FindModels(module, ggconst.DirModel, ignore)
	if err != nil {
		return scannedModels{}, err
	}
	projectCfg, err := loadProjectConfig()
	if err != nil {
		return scannedModels{}, err
	}
	ignoreResult := codegen.ResolveRoutes(allModels, projectCfg.Gen.Routes.Ignore)
	if !quiet && len(ignoreResult.Matches) > 0 {
		clioutput.Section("Ignore Routes")
		for _, match := range ignoreResult.Matches {
			clioutput.Item("IGNORE", "%s %s (%s)", match.Method, match.Path, match.Model)
		}
	}
	reportRouteIgnoreWarnings(ignoreResult)

	// Model ignores run after route ignores so the live-action warning sees
	// the final enabled-action set.
	modelIgnores := codegen.ApplyModelIgnores(allModels, projectCfg.Gen.Models.Ignore)
	if !quiet && len(modelIgnores.Matches) > 0 {
		clioutput.Section("Ignore Models")
		for _, match := range modelIgnores.Matches {
			clioutput.Item("IGNORE", "model %s (%s)", match.Model, match.File)
		}
	}
	reportModelIgnoreWarnings(modelIgnores)

	return scannedModels{models: allModels, routeIgnores: ignoreResult, pruneConfig: projectCfg.Prune}, nil
}

// reportModelIgnoreWarnings warns about model ignore rules that matched no
// migrating model (a stale rule means a previously removed table silently
// comes back), about From-less rules matching models under several
// directories, and about ignored models whose routes are still enabled.
// Warnings are emitted even in quiet mode.
func reportModelIgnoreWarnings(result codegen.ModelIgnoreResult) {
	for _, rule := range result.Unmatched {
		clioutput.Warn("", "gst.yaml model ignore rule matched no migrating model: %s", rule.Raw)
	}
	for _, rule := range result.MultiSourceRules {
		clioutput.Warn("", "gst.yaml model ignore rule %q matched models under %s; add \"from\" to scope it to one directory", rule.Raw, strings.Join(rule.Dirs, ", "))
	}
	for _, match := range result.LiveActionModels {
		clioutput.Warn("", "gst.yaml ignores registration of model %s (%s) but its routes stay enabled; add gen.routes.ignore entries or ensure another model owns its table", match.Model, match.File)
	}
}

// reportRouteIgnoreWarnings warns about ignore rules that matched no
// generated route (a stale rule means a previously ignored route may have
// silently come back) and about From-less rules matching models under
// several directories (likely swallowing the project's own re-declaration).
// Warnings are emitted even in quiet mode.
func reportRouteIgnoreWarnings(result codegen.RouteIgnoreResult) {
	for _, rule := range result.Unmatched {
		clioutput.Warn("", "gst.yaml ignore rule matched no route: %s", rule.Raw)
	}
	for _, rule := range result.MultiSourceRules {
		clioutput.Warn("", "gst.yaml ignore rule %q matched models under %s; add \"from\" to scope it to one directory", rule.Raw, strings.Join(rule.Dirs, ", "))
	}
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
