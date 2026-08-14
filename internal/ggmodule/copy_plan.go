package ggmodule

import (
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
)

const (
	defaultModelDir      = "model"
	defaultServiceDir    = "service"
	defaultMiddlewareDir = "middleware"
)

// CopyOptions configures the local-source copy workflow.
type CopyOptions struct {
	Force bool
}

// CopyPlan describes the final files and source mappings for one module copy.
type CopyPlan struct {
	Name              string
	ProjectModulePath string
	FrameworkRoot     string

	ModelDir   string
	ServiceDir string

	SourceModelDir   string
	SourceServiceDir string

	TargetModelDir        string
	TargetServiceDir      string
	TargetMiddlewareDir   string
	TargetModelImportPath string

	ExcludeSourceFiles []string
	IncludeSourceFiles []string
	PostNotes          []string

	Actions    []moduleCopyAction
	Middleware []moduleCopyMiddleware
	Files      []moduleCopyFile

	// StaleModelFiles lists Go files already present in TargetModelDir that do
	// not have a matching source file under SourceModelDir in this copy plan.
	// They are stale copies left behind by an older framework version, and the
	// copy execution deletes them so the target directory keeps mirroring the
	// framework module source. Test files and generated files never enter the
	// list; see staleTargetFiles for the exemptions.
	StaleModelFiles []string
	// StaleServiceFiles lists target service files that are already present
	// but are not produced by this copy plan. The copy execution deletes them
	// under the same mirror contract as StaleModelFiles: project-owned code
	// belongs outside copied service directories, as the module postNotes
	// instruct, so a non-exempt leftover here is stale module code.
	StaleServiceFiles []string
}

// moduleCopyAction connects one DSL action to the framework service file that
// supplies its business logic and the current-project service file that gg gen
// will create for it.
type moduleCopyAction struct {
	Route      string
	Action     *dsl.Action
	SourcePath string
	TargetPath string
	ModelInfo  *gen.ModelInfo
}

// moduleCopyFile stores final target content. Conflict checks run against this
// final content before any file is written, so pre-existing files only need
// --force when the copy would actually change them.
type moduleCopyFile struct {
	Kind        moduleCopyFileKind
	TargetPath  string
	Content     []byte
	Preexisting bool
}

type moduleCopyFileKind string

const (
	moduleCopyFileModel      moduleCopyFileKind = "model"
	moduleCopyFileService    moduleCopyFileKind = "service"
	moduleCopyFileHelper     moduleCopyFileKind = "helper"
	moduleCopyFileMiddleware moduleCopyFileKind = "middleware"
)

// BuildCopyPlan is the copy-ready preflight. It resolves framework source
// paths, parses source DSL through the same codegen model parser used by gg gen,
// computes final rewritten file contents, and checks target conflicts.
func BuildCopyPlan(name string, opts CopyOptions) (*CopyPlan, error) {
	// Module copy addresses a top-level framework module by name, such as
	// "copytest"; nested paths such as "<module>/<subpackage>" are outside the
	// command contract.
	if err := validateModuleCommandName(name, "module copy"); err != nil {
		return nil, err
	}
	if _, err := os.Stat("go.mod"); err != nil {
		return nil, fmt.Errorf("gg module copy must run from the project root: %w", err)
	}

	projectModule, err := readProjectModulePath()
	if err != nil {
		return nil, err
	}

	frameworkRoot, err := findFrameworkRoot()
	if err != nil {
		return nil, err
	}

	plan := &CopyPlan{
		Name:                  name,
		ProjectModulePath:     projectModule,
		FrameworkRoot:         frameworkRoot,
		ModelDir:              defaultModelDir,
		ServiceDir:            defaultServiceDir,
		SourceModelDir:        filepath.Join(frameworkRoot, "internal", "model", name),
		SourceServiceDir:      filepath.Join(frameworkRoot, "internal", "service", name),
		TargetModelDir:        filepath.Join(defaultModelDir, name),
		TargetServiceDir:      filepath.Join(defaultServiceDir, name),
		TargetMiddlewareDir:   defaultMiddlewareDir,
		TargetModelImportPath: filepath.Join(projectModule, defaultModelDir, name),
	}

	if sourceDirErr := plan.checkSourceDirs(); sourceDirErr != nil {
		return nil, sourceDirErr
	}
	manifest, err := loadModuleManifest(filepath.Join(frameworkRoot, "module", name))
	if err != nil {
		return nil, err
	}
	plan.PostNotes = manifest.Copy.PostNotes
	plan.ExcludeSourceFiles = manifest.Copy.ExcludeSourceFiles
	if includeErr := plan.resolveIncludeSourceFiles(manifest.Copy.IncludeSourceFiles); includeErr != nil {
		return nil, includeErr
	}
	middleware, err := plan.resolveMiddleware(manifest.Copy.Middleware)
	if err != nil {
		return nil, err
	}
	plan.Middleware = middleware

	if registerErr := checkModuleNotRegistered(name); registerErr != nil {
		return nil, registerErr
	}

	models, err := plan.findModels()
	if err != nil {
		return nil, err
	}

	if addModelErr := plan.addModelFiles(); addModelErr != nil {
		return nil, addModelErr
	}
	if staleModelErr := plan.collectStaleModelFiles(); staleModelErr != nil {
		return nil, staleModelErr
	}

	actions, err := plan.collectActions(models)
	if err != nil {
		return nil, err
	}
	plan.Actions = actions

	// Precompute final service/helper contents during preflight so conflict checks
	// compare against what will actually be written. The execution phase still
	// runs gg gen for real before writing these merged files.
	helperFiles, err := plan.collectHelperDependencyFiles(actions)
	if err != nil {
		return nil, err
	}

	if addServiceErr := plan.addServiceFiles(helperFiles); addServiceErr != nil {
		return nil, addServiceErr
	}
	if staleServiceErr := plan.collectStaleServiceFiles(); staleServiceErr != nil {
		return nil, staleServiceErr
	}
	if addMiddlewareErr := plan.addMiddlewareFiles(); addMiddlewareErr != nil {
		return nil, addMiddlewareErr
	}
	if conflictErr := plan.checkConflicts(opts.Force); conflictErr != nil {
		return nil, conflictErr
	}
	return plan, nil
}

func (p *CopyPlan) checkSourceDirs() error {
	if err := requireDir(filepath.Join(p.FrameworkRoot, "module", p.Name)); err != nil {
		return fmt.Errorf("module %q not found: %w", p.Name, err)
	}
	if err := requireDir(p.SourceModelDir); err != nil {
		return fmt.Errorf("module %q model source not found: %w", p.Name, err)
	}
	if err := requireDir(p.SourceServiceDir); err != nil {
		return fmt.Errorf("module %q service source not found: %w", p.Name, err)
	}
	return nil
}

// staleTargetFiles lists Go files already present in dir that this copy plan
// does not produce. The caller passes the plan file kinds that land in dir,
// because copy output is not a mirror of the framework source tree: action
// service files come from DSL ServiceFilename(), helper files from dependency
// discovery, and excluded sources produce no target at all.
//
// Two kinds of project-owned files never count as stale: test files, which
// goFilesInDir already skips, and generated files, which belong to their
// generator (gg gen Cols files in particular) rather than to module copy.
func (p *CopyPlan) staleTargetFiles(dir string, kinds ...moduleCopyFileKind) ([]string, error) {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}

	expectedTargets := make(map[string]bool)
	for _, file := range p.Files {
		if !slices.Contains(kinds, file.Kind) {
			continue
		}
		rel, relErr := filepath.Rel(dir, file.TargetPath)
		if relErr != nil {
			return nil, relErr
		}
		expectedTargets[rel] = true
	}

	targetFiles, err := goFilesInDir(dir)
	if err != nil {
		return nil, err
	}
	stale := make([]string, 0)
	for _, targetPath := range targetFiles {
		rel, err := filepath.Rel(dir, targetPath)
		if err != nil {
			return nil, err
		}
		if expectedTargets[rel] {
			continue
		}
		generated, err := isGeneratedFile(targetPath)
		if err != nil {
			return nil, err
		}
		if generated {
			continue
		}
		stale = append(stale, targetPath)
	}
	sort.Strings(stale)
	return stale, nil
}

func (p *CopyPlan) checkConflicts(force bool) error {
	for _, file := range p.Files {
		if !file.Preexisting {
			continue
		}
		old, err := os.ReadFile(file.TargetPath)
		if err != nil {
			return err
		}
		if string(old) == string(file.Content) {
			continue
		}
		if !force {
			return fmt.Errorf("%s already exists; use --force to overwrite", file.TargetPath)
		}
	}
	return nil
}

// resolveIncludeSourceFiles validates manifest include entries and stores them
// on the plan. Includes are forced helper copies, so every entry must be a real
// non-test file under the module's service source tree, must not also be
// excluded, and must not declare a service struct: action service files are
// copied through their DSL actions, not through the manifest.
func (p *CopyPlan) resolveIncludeSourceFiles(includes []string) error {
	servicePrefix := pathpkg.Join("internal", "service", p.Name) + "/"
	for _, rel := range includes {
		if !strings.HasPrefix(rel, servicePrefix) {
			return fmt.Errorf("includeSourceFiles entry %q must live under %s", rel, servicePrefix)
		}
		if strings.HasSuffix(rel, "_test.go") {
			return fmt.Errorf("includeSourceFiles entry %q must not be a test file", rel)
		}
		if slices.Contains(p.ExcludeSourceFiles, rel) {
			return fmt.Errorf("includeSourceFiles entry %q is also listed in excludeSourceFiles", rel)
		}
		sourcePath := filepath.Join(p.FrameworkRoot, filepath.FromSlash(rel))
		if _, err := os.Stat(sourcePath); err != nil {
			return fmt.Errorf("includeSourceFiles entry %q not found: %w", rel, err)
		}
		count, err := countServiceStructsInFile(sourcePath)
		if err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("includeSourceFiles entry %q declares a service struct; action service files are copied through their DSL actions", rel)
		}
	}
	p.IncludeSourceFiles = includes
	return nil
}

// includeSourceFilePaths returns the manifest-included service files as
// absolute framework paths, ready to join helper dependency discovery.
func (p *CopyPlan) includeSourceFilePaths() []string {
	paths := make([]string, 0, len(p.IncludeSourceFiles))
	for _, rel := range p.IncludeSourceFiles {
		paths = append(paths, filepath.Join(p.FrameworkRoot, filepath.FromSlash(rel)))
	}
	return paths
}

// ignoredSourcePath matches module.json copy.excludeSourceFiles against source files
// relative to the framework root. This keeps the manifest stable across projects:
// "internal/model/copytest/ignored.go" means the same source file no matter where
// the current app's model/service directories are configured.
func (p *CopyPlan) ignoredSourcePath(sourcePath string) bool {
	if len(p.ExcludeSourceFiles) == 0 {
		return false
	}
	rel, err := filepath.Rel(p.FrameworkRoot, sourcePath)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	return slices.Contains(p.ExcludeSourceFiles, rel)
}

// ModelTargets returns current-project model files that copy will write.
func (p *CopyPlan) ModelTargets() []string {
	return p.targetsByKind(moduleCopyFileModel)
}

// ServiceTargets returns current-project action service files that copy will merge.
func (p *CopyPlan) ServiceTargets() []string {
	return p.targetsByKind(moduleCopyFileService)
}

// HelperTargets returns current-project helper service files that copy will write.
func (p *CopyPlan) HelperTargets() []string {
	return p.targetsByKind(moduleCopyFileHelper)
}

// MiddlewareTargets returns manifest-declared middleware files copied into the
// current project's middleware package.
func (p *CopyPlan) MiddlewareTargets() []string {
	return p.targetsByKind(moduleCopyFileMiddleware)
}

// StaleModelTargets returns current-project model files that the copy
// execution will delete: files an older copy produced that the current
// framework module source no longer contains. Preview and execution consume
// this same list, so the files shown as pending deletion are exactly the
// files the prune phase removes.
func (p *CopyPlan) StaleModelTargets() []string {
	return append([]string(nil), p.StaleModelFiles...)
}

// StaleServiceTargets returns current-project service files that the copy
// execution will delete, under the same preview/execution contract as
// StaleModelTargets.
func (p *CopyPlan) StaleServiceTargets() []string {
	return append([]string(nil), p.StaleServiceFiles...)
}

func (p *CopyPlan) targetsByKind(kind moduleCopyFileKind) []string {
	targets := make([]string, 0)
	for _, file := range p.Files {
		if file.Kind == kind {
			targets = append(targets, file.TargetPath)
		}
	}
	return targets
}
