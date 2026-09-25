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
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gggen"
	"github.com/hydroan/gst/internal/gggen/columns"
	"github.com/hydroan/gst/internal/gggen/pb"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/ggnew"
	"github.com/hydroan/gst/internal/modelinfo"
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
	genCmd.Flags().BoolVar(&prune, "prune", false, "After generating, prune what the models no longer need from service/, middleware/ and pb/, asking once before deleting")
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

	if runProjectChecks(generationChecks(), opts.Quiet, opts.BaselineViolations) > 0 {
		return errors.New("project checks failed")
	}

	// Ensure required files exist
	if !opts.Quiet {
		clioutput.Section("Ensure Required Files")
	}
	createdFiles, err := ggnew.EnsureFileExists()
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

	// Record the service files and protobuf definitions present before
	// generating (if prune option is enabled): the ones this run does not
	// write again are what prune deletes.
	var oldServiceFiles, oldPBFiles []string
	if prune {
		oldServiceFiles = existingServiceFiles()
		oldPBFiles = existingPBFiles()
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
				target := modelinfo.ServiceTarget(m, a, ggconst.DirModel, ggconst.DirService)
				servicePkgs[target.ImportPath] = target.PackageName
			}
			routerPkgs[m.ImportPath()] = m.ModelPkgName
		})
	}

	// A dsl.PayloadEmpty side is emitted as *model.Empty (or *gstmodel.Empty
	// when a routed business model package is itself named "model"), so the
	// qualifier and the import are decided once per router file.
	gstModelPkg, gstModelNeeded := gggen.RouterGstModelUse(allModels)
	routerGstModelPkg := ""
	if gstModelNeeded {
		routerGstModelPkg = gstModelPkg
	}
	// A package whose name another import or a framework import of the same
	// file already takes is imported under an alias, and its registrations
	// refer to it through the alias.
	modelAliases := gggen.ModelFileAliases(modelPkgs)
	serviceAliases := gggen.ServiceFileAliases(servicePkgs)
	routerAliases := gggen.RouterFileAliases(routerPkgs, routerGstModelPkg)

	for _, m := range allModels {
		if !m.Design.Enabled || !m.Design.Migrate {
			continue
		}
		// A model in the root model package registers unqualified, as in
		// "Register[*Record]()"; any other is qualified by the name its
		// package is imported under, as in "Register[*sample.Record]()".
		if m.InModelRoot(ggconst.DirModel) {
			modelStmts = append(modelStmts, gggen.StmtModelRegister(m.ModelName))
		} else {
			modelStmts = append(modelStmts, gggen.StmtModelRegister(importQualifier(modelAliases, m.ImportPath(), m.ModelPkgName)+"."+m.ModelName))
		}
	}
	for _, m := range allModels {
		m.Design.Range(func(route string, act *dsl.Action) {
			// Both registrations below must carry this exact route string:
			// the service registry keys services by route and phase, so the
			// service side and the router side share one route value.
			route, paramName := modelinfo.RouterTargetForAction(route, m.Design, act)

			if act.Service {
				target := modelinfo.ServiceTarget(m, act, ggconst.DirModel, ggconst.DirService)
				serviceStmts = append(serviceStmts, gggen.StmtServiceRegister(importQualifier(serviceAliases, target.ImportPath, target.PackageName)+"."+act.RoleName(), act.Phase, route))
			}
			base := "Auth"
			if act.Public {
				base = "Pub"
			}
			routerStmts = append(routerStmts, gggen.StmtRouterRegister(importQualifier(routerAliases, m.ImportPath(), m.ModelPkgName), m.ModelName, act.Payload, act.Result, gstModelPkg, base, route, paramName, act.Phase.MethodName()))
		})
	}

	// ============================================================
	// Generate model/service/router/main files
	// ============================================================
	if !opts.Quiet {
		clioutput.Section("Generate Files")
	}
	modelCode, err := gggen.BuildModelFile("model", modelAliases, modelStmts...)
	if err != nil {
		return errors.Wrap(err, "build model/model.gen.go")
	}
	if writeErr := writeGenFile(filepath.Join(ggconst.DirModel, ggconst.FileModelGen), modelCode); writeErr != nil {
		return writeErr
	}

	// generate model/apidoc.gen.go, which registers struct and field doc comments
	// so the OpenAPI document keeps schema descriptions in binaries deployed
	// without Go source files.
	docEntries, err := modelinfo.ExtractAPIDocs(module, ggconst.DirModel, ignore, nil)
	if err != nil {
		return errors.Wrap(err, "extract api docs")
	}
	apidocCode, err := gggen.BuildAPIDocFile("model", docEntries)
	if err != nil {
		return errors.Wrap(err, "build model/apidoc.gen.go")
	}
	if writeErr := writeGenFile(filepath.Join(ggconst.DirModel, ggconst.FileAPIDocGen), apidocCode); writeErr != nil {
		return writeErr
	}

	// generate service/service.gen.go
	serviceCode, err := gggen.BuildServiceFile("service", serviceAliases, serviceStmts...)
	if err != nil {
		return errors.Wrap(err, "build service/service.gen.go")
	}
	if writeErr := writeGenFile(filepath.Join(ggconst.DirService, ggconst.FileServiceGen), serviceCode); writeErr != nil {
		return writeErr
	}

	// generate router/router.gen.go
	routerCode, err := gggen.BuildRouterFile("router", routerGstModelPkg, routerAliases, routerStmts...)
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
	mainCode, err := gggen.BuildMainFile(module)
	if err != nil {
		return errors.Wrap(err, "build main.go")
	}
	if err = writeGenFile(ggconst.FileMain, mainCode); err != nil {
		return err
	}

	// Generate the protobuf definitions of the models served over gRPC and
	// the Go files compiled from them. The model packages are type-checked
	// for it, so this runs once their registration files above are current;
	// the whole set is built before any of it is written, so a definition
	// the compiler refuses leaves the files on disk as they were.
	pbFiles, err := protobufFiles(allModels)
	if err != nil {
		return err
	}
	pbPaths := make([]string, 0, len(pbFiles))
	for _, f := range pbFiles {
		pbPaths = append(pbPaths, f.Path)
		if err = writeGenFile(filepath.FromSlash(f.Path), f.Content); err != nil {
			return err
		}
	}

	// ============================================================
	// Apply actions to services
	// ============================================================
	if !opts.Quiet {
		clioutput.Section("Apply Actions To Services")
	}

	fset := token.NewFileSet()
	// applyFile writes the service file target locates: a new file is
	// created with its test scaffolds, an existing one has the action's
	// declarations synced into it. route is the route the router registers
	// the action under.
	applyFile := func(target modelinfo.ServiceTargetInfo, route string, code string, action *dsl.Action, modelInfo *modelinfo.Model) error {
		servicePkgName := target.PackageName
		safePath, err := pathUnderRoot(target.FilePath, ggconst.DirService)
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
			changed, err := gggen.ApplyServiceFileWithModelSync(f, action, servicePkgName, ggconst.DirModel, modelInfo)
			if err != nil {
				return errors.Wrapf(err, "service file %s", safePath)
			}
			if changed {
				// Only reformat and write file when there are changes
				// Use original FileSet to preserve comment positions
				code, err = gggen.FormatNodeExtraWithFileSet(f, fset)
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
			if err := scaffoldServiceTests(modelInfo, target, action, route, opts.Quiet); err != nil {
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
			target := modelinfo.ServiceTarget(m, act, ggconst.DirModel, ggconst.DirService)
			if file := gggen.GenerateService(m, act, act.Phase, target.PackageName); file != nil {
				fset := token.NewFileSet()
				code, err := gggen.FormatNodeExtraWithFileSet(file, fset)
				if err != nil {
					applyErr = err
					return
				}
				registered, _ := modelinfo.RouterTargetForAction(route, m.Design, act)
				applyErr = applyFile(target, registered, code, act, m)
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
		pruneLeftovers(oldServiceFiles, allModels, ignoreResult.KeptServiceFiles, ignoreResult.KeptServiceDirs, ignore, scanned.pruneConfig, oldPBFiles, pbPaths)
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

// scaffoldServiceTests writes the test scaffold of the service file target
// locates, which gg gen has just created for an action of modelInfo, and
// main_test.go for its package when no test file of the package declares
// TestMain yet (see
// gggen.GenerateServiceTest and gggen.GenerateServiceTestMain). The service test
// coverage check requires the test file from the next run on, so the run
// that creates the service file creates its test as well. A test file the
// project already has, in its external or internal form, is kept as it is,
// and so is a main_test.go that exists already.
func scaffoldServiceTests(modelInfo *modelinfo.Model, target modelinfo.ServiceTargetInfo, action *dsl.Action, route string, quiet bool) error {
	stem := strings.TrimSuffix(target.FilePath, ".go")
	if gghelper.FileExists(stem+ggconst.PatternTestFile) || gghelper.FileExists(stem+"_internal"+ggconst.PatternTestFile) {
		return nil
	}

	declared, err := gggen.PackageDeclaresTestMain(target.Dir)
	if err != nil {
		return err
	}
	mainTest := filepath.Join(target.Dir, ggconst.FileMainTest)
	if !declared && !gghelper.FileExists(mainTest) {
		var mainCode string
		if mainCode, err = gggen.GenerateServiceTestMain(module, target.PackageName); err != nil {
			return err
		}
		if err = writeGeneratedFile(mainTest, mainCode, !quiet); err != nil {
			return err
		}
	}

	code, err := gggen.GenerateServiceTest(modelInfo, target, action, route)
	if err != nil {
		return err
	}
	return writeGeneratedFile(stem+ggconst.PatternTestFile, code, !quiet)
}

// scannedModels is the model set code generation works from.
type scannedModels struct {
	models []*modelinfo.Model
	// routeIgnores records the actions the gst.yaml route ignores disabled,
	// with the service files pruning must keep for them.
	routeIgnores modelinfo.RouteIgnoreResult
	// pruneConfig holds the gst.yaml prune settings gg gen --prune applies.
	pruneConfig ggconfig.PruneConfig
}

// protobufFiles renders the .proto files of the models declaring GRPC()
// (see pb.Generate) and compiles the Go files beside them (see pb.Compile),
// the same set gg gen writes and gg prune keeps, definitions first. The
// diagnostics of types protobuf cannot describe come back as they are, one
// line each; any other failure is wrapped.
func protobufFiles(models []*modelinfo.Model) ([]pb.File, error) {
	protos, err := pb.Generate(pb.Config{Dir: ".", ModulePath: module, Models: models})
	var diagnostics *pb.DiagnosticsError
	switch {
	case errors.As(err, &diagnostics):
		return nil, err
	case err != nil:
		return nil, errors.Wrap(err, "generate the protobuf definitions")
	}
	compiled, err := pb.Compile(protos)
	if err != nil {
		return nil, errors.Wrap(err, "compile the protobuf definitions")
	}
	return append(protos, compiled...), nil
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
	allModels, err := modelinfo.FindModels(module, ggconst.DirModel, ignore)
	if err != nil {
		return scannedModels{}, err
	}
	projectCfg, err := loadProjectConfig()
	if err != nil {
		return scannedModels{}, err
	}
	ignoreResult := modelinfo.ResolveRoutes(allModels, projectCfg.Gen.Routes.Ignore)
	if !quiet && len(ignoreResult.Matches) > 0 {
		clioutput.Section("Ignore Routes")
		for _, match := range ignoreResult.Matches {
			clioutput.Item("IGNORE", "%s %s (%s)", match.Method, match.Path, match.Model)
		}
	}
	reportRouteIgnoreWarnings(ignoreResult)

	// Model ignores run after route ignores so the live-action warning sees
	// the final enabled-action set.
	modelIgnores := modelinfo.ApplyModelIgnores(allModels, projectCfg.Gen.Models.Ignore)
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
func reportModelIgnoreWarnings(result modelinfo.ModelIgnoreResult) {
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
func reportRouteIgnoreWarnings(result modelinfo.RouteIgnoreResult) {
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
