package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/ggcheck"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gggen"
	"github.com/hydroan/gst/internal/gggen/columns"
	"github.com/hydroan/gst/internal/gggen/pb"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/ggnew"
	"github.com/hydroan/gst/internal/modelsource"
	"github.com/hydroan/gst/internal/pbschema"
	"github.com/spf13/cobra"
)

var genCmd = &cobra.Command{
	Use:   "gen",
	Short: "Generate the code the models declare",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return genRun()
	},
}

type genRunOptions struct {
	// Quiet keeps the run to the violations of its checks: the lines about
	// the checks passed, the models read and the files written are left out.
	Quiet bool
	// Log receives what the run writes and removes; nil gives the run a log
	// of its own, quiet when the run is. gg gen passes the one the line it
	// ends on is read from.
	Log *changes
	// BaselineViolations lists project check violations that already existed
	// before the caller started. The quiet pre-generation checks fail only on
	// violations outside this baseline; module copy uses it so pre-existing
	// project issues do not block copying an unrelated module.
	BaselineViolations map[string]struct{}
	// Checks are the project checks the run makes before generating; nil
	// runs generationChecks. Module copy passes moduleCopyChecks.
	Checks []ggcheck.Check
	// MergeShells names, by path, the service files this run creates as the
	// shells module copy merges the module's source onto: the struct and the
	// action method alone, no hook placeholders and no test scaffolds, which
	// are for the services a project writes by hand. Module copy passes its
	// service targets; a plain gg gen passes nothing.
	MergeShells map[string]struct{}
}

// mergesShell reports whether the service file at path is one of the shells
// module copy merges onto.
func (o genRunOptions) mergesShell(path string) bool {
	_, ok := o.MergeShells[filepath.Clean(path)]
	return ok
}

// genRun generates the code, tidies the module, so that go.mod and go.sum
// hold what the written files import (see tidyModule), and ends on what the
// run changed: the files it created, updated and removed, or that every
// file was up to date. A run that created service files says what is left
// to do in them.
func genRun() error {
	started := time.Now()
	log := new(changes)
	if err := genRunWithOptions(genRunOptions{Log: log}); err != nil {
		return err
	}
	if err := tidyModule(log); err != nil {
		return err
	}

	summary := log.summary()
	if summary == "" {
		clioutput.Done("Nothing to generate: every file is up to date (%s)", elapsedSince(started))
		return nil
	}
	clioutput.Done("Generated: %s (%s)", summary, elapsedSince(started))
	for path := range log.created {
		if strings.HasPrefix(path, ggconst.DirService+string(filepath.Separator)) && !strings.HasSuffix(path, ggconst.PatternTestFile) && !strings.HasSuffix(path, ggconst.SuffixGenGo) {
			clioutput.Note("Next: write the logic of the new service files; the test of each fails on its first line until that line is deleted")
			break
		}
	}
	return nil
}

func genRunWithOptions(opts genRunOptions) error {
	log := opts.Log
	if log == nil {
		log = &changes{quiet: opts.Quiet}
	}
	if len(module) == 0 {
		var err error
		module, err = gghelper.ModulePath(".")
		if err != nil {
			return err
		}
	}

	// Heal bare model.Version declarations before the checks run: the tag
	// shape is framework-owned, and gen filling it in is what keeps the
	// version-field check below from failing the very command that fixes it.
	if err := fillVersionFieldTags(log); err != nil {
		return err
	}
	// gst.yaml is read once, the warnings about the files next to it gg does
	// not read printed once with it, for both scans below.
	projectCfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	// Number the fields of the models served over gRPC that carry no pb tag,
	// for the same reason (see fillPBTags); the definitions it derived on the
	// way are the ones written below when it wrote no tag.
	ignore := gghelper.NewProjectIgnore()
	pbDerived, err := fillPBTags(log, ignore, projectCfg)
	if err != nil {
		return err
	}

	checks := opts.Checks
	if checks == nil {
		checks = generationChecks()
	}
	if err = runProjectChecks(checks, opts.Quiet, opts.BaselineViolations); err != nil {
		return errors.Wrap(err, "nothing was generated")
	}

	// Ensure required files exist
	createdFiles, err := ggnew.EnsureFileExists()
	for _, file := range createdFiles {
		log.record(clioutput.LabelCreate, file, "")
	}
	if err != nil {
		return err
	}

	scanned, err := scanModels(opts.Quiet, ignore, projectCfg)
	if err != nil {
		return err
	}
	reportIgnoreWarnings(scanned)
	allModels := scanned.models

	modelStmts := make([]ast.Stmt, 0)
	serviceStmts := make([]ast.Stmt, 0)
	routerStmts := make([]ast.Stmt, 0)
	// The packages each registration file imports, every import path mapped
	// to the name of the package it declares.
	modelPkgs := make(map[string]string)
	routerPkgs := make(map[string]string)
	servicePkgs := make(map[string]string)
	writeGenFile := func(filename string, content string) error {
		return writeGeneratedFile(filename, content, log)
	}

	for _, m := range allModels {
		// The model registration file belongs to the root model package; a
		// model anywhere else is registered through an import of its package.
		if m.Design.Migrate && !m.InModelRoot(ggconst.DirModel) {
			modelPkgs[m.ImportPath()] = m.ModelPkgName
		}

		m.Design.Range(func(s string, a *dsl.Action) {
			if a.Service {
				target := modelsource.ServiceTarget(m, a, ggconst.DirModel, ggconst.DirService)
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
		if !m.Design.Migrate {
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
			route, paramName := modelsource.RouterTargetForAction(route, m.Design, act)

			if act.Service {
				target := modelsource.ServiceTarget(m, act, ggconst.DirModel, ggconst.DirService)
				serviceStmts = append(serviceStmts, gggen.StmtServiceRegister(importQualifier(serviceAliases, target.ImportPath, target.PackageName)+"."+act.RoleName(), act.Phase, route))
			}
			base := ggconst.RouterAuth
			if act.Public {
				base = ggconst.RouterPub
			}
			routerStmts = append(routerStmts, gggen.StmtRouterRegister(importQualifier(routerAliases, m.ImportPath(), m.ModelPkgName), m.ModelName, act.Payload, act.Result, gstModelPkg, base, route, paramName, act.Phase.Name()))
		})
	}

	// ============================================================
	// Generate model/service/router/main files
	// ============================================================
	modelCode, err := gggen.BuildModelFile(module, ggconst.PkgModel, modelAliases, modelStmts...)
	if err != nil {
		return errors.Wrap(err, "build model/model.gen.go")
	}
	if writeErr := writeGenFile(filepath.Join(ggconst.DirModel, ggconst.FileModelGen), modelCode); writeErr != nil {
		return writeErr
	}

	// generate model/apidoc.gen.go, which registers struct and field doc
	// comments so the OpenAPI document keeps schema descriptions in binaries
	// deployed without Go source files.
	docEntries, err := modelsource.ExtractAPIDocs(module, ggconst.DirModel, ignore, nil)
	if err != nil {
		return errors.Wrap(err, "extract api docs")
	}
	apidocCode, err := gggen.BuildAPIDocFile(module, ggconst.PkgModel, docEntries)
	if err != nil {
		return errors.Wrap(err, "build model/apidoc.gen.go")
	}
	if writeErr := writeGenFile(filepath.Join(ggconst.DirModel, ggconst.FileAPIDocGen), apidocCode); writeErr != nil {
		return writeErr
	}

	// generate service/service.gen.go
	serviceCode, err := gggen.BuildServiceFile(module, ggconst.PkgService, serviceAliases, serviceStmts...)
	if err != nil {
		return errors.Wrap(err, "build service/service.gen.go")
	}
	if writeErr := writeGenFile(filepath.Join(ggconst.DirService, ggconst.FileServiceGen), serviceCode); writeErr != nil {
		return writeErr
	}

	// generate router/router.gen.go
	routerCode, err := gggen.BuildRouterFile(module, ggconst.PkgRouter, routerGstModelPkg, routerAliases, routerStmts...)
	if err != nil {
		return errors.Wrap(err, "build router/router.gen.go")
	}
	if writeErr := writeGenFile(filepath.Join(ggconst.DirRouter, ggconst.FileRouterGen), routerCode); writeErr != nil {
		return writeErr
	}

	// Generate the typed column references of every model, so filters can
	// name columns through the compiler instead of through string literals.
	columnFiles, genErr := columns.Generate(module, ggconst.DirModel, allModels, ignore)
	for _, path := range columnFiles.Created {
		log.record(clioutput.LabelCreate, path, "")
	}
	for _, path := range columnFiles.Updated {
		log.record(clioutput.LabelUpdate, path, "")
	}
	for _, path := range columnFiles.Removed {
		log.record(clioutput.LabelRemove, path, " (its source declares no model with columns any more)")
	}
	if genErr != nil {
		return genErr
	}

	// Generate the protobuf definitions of the models served over gRPC, the
	// Go files serving them and the ones compiled from them, unless
	// fillPBTags derived the definitions already. The model packages are
	// type-checked for it, so this runs once their registration files above
	// are current; the whole set is built before any of it is written, so a
	// definition the compiler refuses leaves the files on disk as they were.
	pbFiles, err := protobufFiles(allModels, pbDerived, ignore)
	if err != nil {
		return err
	}
	// The Go files are checked as the packages they make up before any is
	// written (see pb.TypeCheck): one the compiler would refuse is a defect
	// of gg gen, reported here rather than by go build on what was written.
	if err = pb.TypeCheck(".", module, pbFiles); err != nil {
		if !errors.Is(err, pb.ErrUnchecked) {
			return errors.Wrap(err, "the Go files generated under "+ggconst.DirPB+"/ do not compile, a defect of gg gen")
		}
		clioutput.Warn("%v; go build reports what the check would have", err)
	}
	pbPaths := make([]string, 0, len(pbFiles))
	for _, f := range pbFiles {
		pbPaths = append(pbPaths, f.Path)
	}
	// The Go files under pb/ this run does not write served a model deleted
	// or no longer declaring GRPC(): derived from the models alone, they go
	// with it, so that the project builds. The definition stays, with the
	// numbers and names it reserves, for prune to delete (see
	// pruneLeftovers). They are worked out before anything is written, so a
	// Go file there gg gen did not generate leaves pb/ as it was.
	stale, err := staleDerivedPBFiles(pbPaths, projectCfg.Prune)
	if err != nil {
		return err
	}
	for _, f := range pbFiles {
		if err = writeGenFile(filepath.FromSlash(f.Path), f.Content); err != nil {
			return err
		}
	}
	for _, path := range stale {
		if err = os.Remove(path); err != nil {
			return errors.Wrapf(err, "remove %s", path)
		}
		log.record(clioutput.LabelRemove, path, " (its model is no longer served over gRPC)")
	}

	// generate main.go, which imports the interceptor and pb packages
	// exactly while they hold Go sources, the pb package's written or
	// removed just above; the test files declaring TestMain import them
	// the same way.
	extraDirs := gghelper.PresentOptionalImportDirs()
	mainCode, err := gggen.BuildMainFile(module, extraDirs...)
	if err != nil {
		return errors.Wrap(err, "build main.go")
	}
	if err = writeGenFile(ggconst.FileMain, mainCode); err != nil {
		return err
	}
	if err = syncTestMainImports(extraDirs, ignore, log); err != nil {
		return err
	}

	// ============================================================
	// Apply actions to services
	// ============================================================
	fset := token.NewFileSet()
	// applyFile writes the service file target locates: a new file is
	// created with its test scaffolds, an existing one has the action's
	// declarations synced into it. route is the route the router registers
	// the action under.
	applyFile := func(target modelsource.ServiceTargetInfo, route string, code string, action *dsl.Action, modelInfo *modelsource.Model) error {
		servicePkgName := target.PackageName
		safePath, err := gghelper.PathUnderRoot(target.FilePath, ggconst.DirService)
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

			// Apply changes and sync model imports to handle import path and
			// package name updates
			changed, err := gggen.ApplyServiceFileWithModelSync(f, action, servicePkgName, ggconst.DirModel, modelInfo)
			if err != nil {
				return errors.Wrapf(err, "service file %s", safePath)
			}
			if changed {
				// The file is the project's own: it is written back only
				// when it changed, through the FileSet it was parsed with so
				// its comments stay in place, the way gopls saves it.
				code, err = gghelper.FormatFile(f, fset, module)
				if err != nil {
					return err
				}
				if err := gghelper.EnsureParentDir(safePath); err != nil {
					return err
				}
				// #nosec G703 -- safePath validated under serviceDir by
				// gghelper.PathUnderRoot
				if err := os.WriteFile(safePath, []byte(code), ggconst.FileModeGenerated); err != nil {
					return err
				}
				log.record(clioutput.LabelUpdate, safePath, "")
			}
		} else {
			if err := gghelper.EnsureParentDir(safePath); err != nil {
				return err
			}
			// #nosec G703 -- safePath validated under serviceDir by
			// gghelper.PathUnderRoot
			if err := os.WriteFile(safePath, []byte(code), ggconst.FileModeGenerated); err != nil {
				return err
			}
			log.record(clioutput.LabelCreate, safePath, "")
			if opts.mergesShell(target.FilePath) {
				return nil
			}
			if err := scaffoldServiceTests(modelInfo, target, action, route, extraDirs, log); err != nil {
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
			target := modelsource.ServiceTarget(m, act, ggconst.DirModel, ggconst.DirService)
			generate := gggen.GenerateService
			if opts.mergesShell(target.FilePath) {
				generate = gggen.GenerateServiceShell
			}
			if file := generate(m, act, act.Phase, target.PackageName); file != nil {
				fset := token.NewFileSet()
				code, err := gghelper.FormatFile(file, fset, module)
				if err != nil {
					applyErr = err
					return
				}
				registered, _ := modelsource.RouterTargetForAction(route, m.Design, act)
				applyErr = applyFile(target, registered, code, act, m)
			}
		})
		if applyErr != nil {
			return applyErr
		}
	}
	return nil
}

// scaffoldServiceTests writes the test scaffold of the service file target
// locates, which gg gen has just created for an action of modelInfo, and
// main_test.go for its package when no test file of the package declares
// TestMain yet (see gggen.GenerateServiceTest and
// gggen.GenerateServiceTestMain); extraDirs are the optional project
// packages main.go imports, which TestMain imports the same way. The
// service test coverage check requires the test file from the next run on,
// so the run that creates the service file creates its test as well. A test
// file the project already has, in its external or internal form, is kept
// as it is, and so is a main_test.go that exists already, its imports of the
// optional packages following main.go's (see syncTestMainImports).
func scaffoldServiceTests(modelInfo *modelsource.Model, target modelsource.ServiceTargetInfo, action *dsl.Action, route string, extraDirs []string, log *changes) error {
	stem := strings.TrimSuffix(target.FilePath, ggconst.ExtensionGo)
	if gghelper.FileExists(stem+ggconst.PatternTestFile) || gghelper.FileExists(stem+ggconst.PatternInternalTestFile) {
		return nil
	}

	mainTestFile, err := gggen.TestMainFile(target.Dir)
	if err != nil {
		return err
	}
	mainTest := filepath.Join(target.Dir, ggconst.FileMainTest)
	if mainTestFile == "" && !gghelper.FileExists(mainTest) {
		var mainCode string
		if mainCode, err = gggen.GenerateServiceTestMain(module, target.PackageName, extraDirs...); err != nil {
			return err
		}
		if err = writeGeneratedFile(mainTest, mainCode, log); err != nil {
			return err
		}
	}

	code, err := gggen.GenerateServiceTest(modelInfo, target, action, route)
	if err != nil {
		return err
	}
	return writeGeneratedFile(stem+ggconst.PatternTestFile, code, log)
}

// syncTestMainImports makes every test file of the project declaring TestMain
// in an external test package import the optional project packages the way
// main.go does, extraDirs being the ones main.go imports (see
// gggen.SyncTestMainImports), recording in log the files it rewrites, which
// it prints the way gopls saves them (see gghelper.FormatFile), so an import
// a file gains lands in the group gopls puts it in.
func syncTestMainImports(extraDirs []string, ignore gghelper.ProjectIgnore, log *changes) error {
	files, err := gghelper.TestMainFiles(ignore)
	if err != nil {
		return err
	}
	for _, path := range files {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return errors.Wrapf(err, "reading the test file %s", path)
		}
		if !gggen.SyncTestMainImports(fset, file, module, extraDirs) {
			continue
		}
		code, err := gghelper.FormatFile(file, fset, module)
		if err != nil {
			return errors.Wrapf(err, "formatting %s", path)
		}
		if err := os.WriteFile(path, []byte(code), ggconst.FileModeGenerated); err != nil {
			return err
		}
		log.record(clioutput.LabelUpdate, path, "")
	}
	return nil
}

// scannedModels is the model set code generation works from.
type scannedModels struct {
	models []*modelsource.Model
	// routeIgnores records the actions the gst.yaml route ignores disabled,
	// with the service files pruning must keep for them.
	routeIgnores modelsource.RouteIgnoreResult
	// modelIgnores records the models the gst.yaml model ignores dropped from
	// registration, with the rules reportIgnoreWarnings warns about.
	modelIgnores modelsource.ModelIgnoreResult
}

// protobufFiles renders the .proto files of the models declaring GRPC() and
// the Go files serving them (see protobufDefinitions), unless derived holds
// the ones fillPBTags rendered, and compiles the Go files of the
// definitions (see pb.Compile), the set gg gen writes, the rendered files
// first.
func protobufFiles(models []*modelsource.Model, derived []pb.File, ignore gghelper.ProjectIgnore) ([]pb.File, error) {
	generated := derived
	if generated == nil {
		var err error
		if generated, err = protobufDefinitions(models, ignore); err != nil {
			return nil, err
		}
	}
	compiled, err := pb.Compile(module, definitionsOf(generated))
	if err != nil {
		return nil, err
	}
	return append(generated, compiled...), nil
}

// definitionsOf picks the .proto files out of the files pb.Generate wrote.
func definitionsOf(files []pb.File) []pb.File {
	var definitions []pb.File
	for _, f := range files {
		if f.Definition() {
			definitions = append(definitions, f)
		}
	}
	return definitions
}

// protobufConfig describes a derivation of the protobuf definitions of
// models: the model packages are type-checked through the overlay the column
// inspection compiles them with, the generated files of the previous run
// stubbed out (see columns.InspectionOverlay), so a run that deletes a model,
// and prune after one, do not fail on a registration or column file still
// naming it.
func protobufConfig(models []*modelsource.Model, ignore gghelper.ProjectIgnore) (pbschema.Config, error) {
	overlay, err := columns.InspectionOverlay(module, ggconst.DirModel, models, ignore)
	if err != nil {
		return pbschema.Config{}, errors.Wrap(err, "overlay the model packages")
	}
	contents := make(map[string][]byte, len(overlay))
	for path, content := range overlay {
		contents[path] = []byte(content)
	}
	return pbschema.Config{Dir: ".", ModulePath: module, Models: models, Overlay: contents}, nil
}

// protobufDefinitions renders the .proto files of the models declaring
// GRPC() and the Go files serving them (see pb.Generate), the files gg gen
// writes itself and gg prune keeps. The diagnostics of types protobuf cannot
// describe come back as they are, one line each; any other failure is
// wrapped.
func protobufDefinitions(models []*modelsource.Model, ignore gghelper.ProjectIgnore) ([]pb.File, error) {
	cfg, err := protobufConfig(models, ignore)
	if err != nil {
		return nil, err
	}
	files, err := pb.Generate(cfg)
	var diagnostics *pbschema.DiagnosticsError
	switch {
	case errors.As(err, &diagnostics):
		return nil, err
	case err != nil:
		return nil, errors.Wrap(err, "generate the protobuf definitions")
	}
	return files, nil
}

// scanModels reads the models of the model directory the way every command
// and check reads them (see modelsource.ScanModels): their routes resolved,
// then the gst.yaml route and model ignores of projectCfg applied, so a
// matched action behaves exactly like an action that was never declared.
// Unless quiet, it prints one line counting the models and the routes left,
// with how many the ignores took out: "13 models, 54 routes (2 routes
// ignored by gst.yaml)"; the warnings about the rules are its caller's to
// print (see reportIgnoreWarnings). gg gen and gg gen ts both start from
// here, which keeps the TypeScript declarations on the routes the generated
// router registers. The command reads gst.yaml (see loadProjectConfig) once
// for every scan it makes.
func scanModels(quiet bool, ignore gghelper.ProjectIgnore, projectCfg *ggconfig.Config) (scannedModels, error) {
	if !gghelper.FileExists(ggconst.DirModel) {
		return scannedModels{}, errors.Newf("model dir not found: %s", ggconst.DirModel)
	}

	scanned, err := modelsource.ScanModels(module, ggconst.DirModel, ignore, projectCfg)
	if err != nil {
		return scannedModels{}, err
	}
	// Two actions registering one path would stop the router at startup;
	// the scan stops here instead, naming both.
	if conflicts := modelsource.RouteConflicts(scanned.Models); len(conflicts) > 0 {
		return scannedModels{}, errors.Join(conflicts...)
	}
	// Two actions writing one service file, one service type into a
	// directory or two packages into one would leave a project that does
	// not build, the one written first overwritten; the scan stops here
	// instead, naming both.
	if conflicts := modelsource.ServiceConflicts(scanned.Models); len(conflicts) > 0 {
		return scannedModels{}, errors.Join(conflicts...)
	}
	if !quiet {
		routes := 0
		for _, m := range scanned.Models {
			m.Design.Range(func(string, *dsl.Action) { routes++ })
		}
		var ignored []string
		if n := len(scanned.RouteIgnores.Matches); n > 0 {
			ignored = append(ignored, counted(n, "route"))
		}
		if n := len(scanned.ModelIgnores.Matches); n > 0 {
			ignored = append(ignored, counted(n, "model registration"))
		}
		summary := counted(len(scanned.Models), "model") + ", " + counted(routes, "route") //gst:literal the word for what is counted
		if len(ignored) > 0 {
			summary += " (" + strings.Join(ignored, " and ") + " ignored by " + ggconfig.FileName + ")"
		}
		clioutput.Success("%s", summary)
	}

	return scannedModels{models: scanned.Models, routeIgnores: scanned.RouteIgnores, modelIgnores: scanned.ModelIgnores}, nil
}

// reportIgnoreWarnings warns about the gst.yaml ignore rules of a scan that
// matched nothing, a stale rule letting a route or a table it once dropped
// silently come back; about rules without "from" matching models under
// several directories, likely swallowing the project's own re-declaration;
// and about ignored models whose routes stay enabled. gg gen prints them
// after its scan whether or not it runs quietly, as gg module copy runs it,
// and gg gen ts after its own; the scans that only fill protobuf tags or
// prune print none, so a command prints each warning once.
func reportIgnoreWarnings(scanned scannedModels) {
	for _, rule := range scanned.routeIgnores.Unmatched {
		clioutput.Warn("gst.yaml ignore rule matched no route: %s", rule.Raw)
	}
	for _, rule := range scanned.routeIgnores.MultiSourceRules {
		clioutput.Warn("gst.yaml ignore rule %q matched models under %s; add \"from\" to scope it to one directory", rule.Raw, strings.Join(rule.Dirs, ", "))
	}
	for _, rule := range scanned.modelIgnores.Unmatched {
		clioutput.Warn("gst.yaml model ignore rule matched no migrating model: %s", rule.Raw)
	}
	for _, rule := range scanned.modelIgnores.MultiSourceRules {
		clioutput.Warn("gst.yaml model ignore rule %q matched models under %s; add \"from\" to scope it to one directory", rule.Raw, strings.Join(rule.Dirs, ", "))
	}
	for _, match := range scanned.modelIgnores.LiveActionModels {
		clioutput.Warn("gst.yaml ignores registration of model %s (%s) but its routes stay enabled; add gen.routes.ignore entries or ensure another model owns its table", match.Model, match.File)
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
