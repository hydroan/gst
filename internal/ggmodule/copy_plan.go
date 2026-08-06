package ggmodule

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
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
	Force      bool
	ModelDir   string
	ServiceDir string
}

func (o CopyOptions) modelDir() string {
	if o.ModelDir == "" {
		return defaultModelDir
	}
	return o.ModelDir
}

func (o CopyOptions) serviceDir() string {
	if o.ServiceDir == "" {
		return defaultServiceDir
	}
	return o.ServiceDir
}

func copyPlanDirOrDefault(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
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
	PostNotes          []string

	Actions    []moduleCopyAction
	Middleware []moduleCopyMiddleware
	Files      []moduleCopyFile

	// ExtraModelFiles is warning-only upgrade guidance for files already
	// present in TargetModelDir that do not have a matching source file under
	// SourceModelDir in this copy plan. Module copy reports these files so
	// callers can clean up stale local copies after framework module changes,
	// but it must not delete them automatically because model directories can
	// intentionally contain project-owned files.
	ExtraModelFiles []string
	// ExtraServiceFiles is warning-only upgrade guidance for target service
	// files that are already present but are not produced by this copy plan.
	// Module copy must not delete them automatically because service packages can
	// intentionally contain project-owned adapters next to copied module code.
	ExtraServiceFiles []string
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
	if err := validateModuleCopyName(name); err != nil {
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
		ModelDir:              opts.modelDir(),
		ServiceDir:            opts.serviceDir(),
		SourceModelDir:        filepath.Join(frameworkRoot, "internal", "model", name),
		SourceServiceDir:      filepath.Join(frameworkRoot, "internal", "service", name),
		TargetModelDir:        filepath.Join(opts.modelDir(), name),
		TargetServiceDir:      filepath.Join(opts.serviceDir(), name),
		TargetMiddlewareDir:   defaultMiddlewareDir,
		TargetModelImportPath: filepath.Join(projectModule, opts.modelDir(), name),
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
	if extraModelErr := plan.collectExtraModelFiles(); extraModelErr != nil {
		return nil, extraModelErr
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
	if extraServiceErr := plan.collectExtraServiceFiles(); extraServiceErr != nil {
		return nil, extraServiceErr
	}
	if addMiddlewareErr := plan.addMiddlewareFiles(); addMiddlewareErr != nil {
		return nil, addMiddlewareErr
	}
	if conflictErr := plan.checkConflicts(opts.Force); conflictErr != nil {
		return nil, conflictErr
	}
	return plan, nil
}

// validateModuleCopyName intentionally rejects anything path-like. Module copy
// copies a top-level framework module addressed by name, such as "copytest";
// nested paths such as "<module>/<subpackage>" are outside the command contract.
func validateModuleCopyName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("module name is required")
	}
	if name != strings.TrimSpace(name) {
		return fmt.Errorf("module name %q must not contain surrounding whitespace", name)
	}
	if strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("module copy accepts a module name, not a path: %s", name)
	}
	if filepath.Clean(name) != name || filepath.Base(name) != name {
		return fmt.Errorf("module copy accepts a module name, not a path: %s", name)
	}
	return nil
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

// ExtraModelTargets returns current-project model files that are not part of
// the current copy plan. These are warnings only: copied model packages can
// contain project-owned files, and module copy cannot prove an extra file is
// obsolete just because the framework source no longer produces it.
func (p *CopyPlan) ExtraModelTargets() []string {
	return append([]string(nil), p.ExtraModelFiles...)
}

// ExtraServiceTargets returns current-project service files that are not part
// of the current copy plan. These are warnings only: copied service packages can
// contain project-owned adapters, and module copy cannot prove an extra file is
// obsolete just because the framework source no longer produces it.
func (p *CopyPlan) ExtraServiceTargets() []string {
	return append([]string(nil), p.ExtraServiceFiles...)
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
