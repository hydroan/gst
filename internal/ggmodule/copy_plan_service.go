package ggmodule

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
)

func (p *CopyPlan) collectActions(models []*gen.ModelInfo) ([]moduleCopyAction, error) {
	actions := make([]moduleCopyAction, 0)
	for _, modelInfo := range models {
		if modelInfo.Design == nil {
			continue
		}
		targetModel, err := p.targetModelInfo(modelInfo)
		if err != nil {
			return nil, err
		}
		modelInfo.Design.Range(func(route string, action *dsl.Action) {
			if !action.Service {
				return
			}
			sourcePath, targetPath := p.actionServicePaths(modelInfo, targetModel, action)
			if p.ignoredSourcePath(sourcePath) {
				return
			}
			actions = append(actions, moduleCopyAction{
				Route:      route,
				Action:     action,
				SourcePath: sourcePath,
				TargetPath: targetPath,
				ModelInfo:  targetModel,
			})
		})
	}
	sort.Slice(actions, func(i, j int) bool {
		if actions[i].TargetPath == actions[j].TargetPath {
			return actions[i].Route < actions[j].Route
		}
		return actions[i].TargetPath < actions[j].TargetPath
	})

	for _, action := range actions {
		if err := requireServiceSourceFile(action); err != nil {
			return nil, err
		}
	}
	return actions, nil
}

// actionServicePaths maps one action to the framework service file that holds
// its logic and the project service file gg gen will create for it.
//
// BuildCopyPlan always fills FrameworkRoot, and both paths then come from gg
// gen's own ServiceTarget so a copied file lands exactly where a later gg gen
// would regenerate it. The empty case is the plan a test builds by hand: it maps
// an action straight onto <service dir>/<ServiceFilename()>, without the
// per-model subdirectory ServiceTarget adds for non-flattened actions.
func (p *CopyPlan) actionServicePaths(sourceModel *gen.ModelInfo, targetModel *gen.ModelInfo, action *dsl.Action) (sourcePath string, targetPath string) {
	if p.FrameworkRoot == "" {
		return filepath.Join(p.SourceServiceDir, action.ServiceFilename()), filepath.Join(p.TargetServiceDir, action.ServiceFilename())
	}
	sourceTarget := gen.ServiceTarget(sourceModel, action, p.frameworkModelDir(), p.frameworkServiceDir())
	targetTarget := gen.ServiceTarget(targetModel, action, p.ModelDir, p.ServiceDir)
	return sourceTarget.FilePath, targetTarget.FilePath
}

// requireServiceSourceFile enforces the module-copy convention that each action
// service source file declares at least one service struct. The whole service
// file is merged later, so hook-only files do not need to declare the action's
// main method, and one file may host multiple action service structs.
func requireServiceSourceFile(action moduleCopyAction) error {
	if _, err := os.Stat(action.SourcePath); err != nil {
		return fmt.Errorf("source action service file not found for %s: %w", action.Action.ServiceFilename(), err)
	}
	count, err := countServiceStructsInFile(action.SourcePath)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("source action service file %s must contain at least one service struct", action.SourcePath)
	}
	return nil
}

// collectHelperDependencyFiles resolves the helper files this copy must carry:
// the type-informed closure over the module service tree, seeded by the action
// service files and the manifest includeSourceFiles. See
// moduleServiceHelperClosure for the walk and its guard rules.
func (p *CopyPlan) collectHelperDependencyFiles(actions []moduleCopyAction) ([]string, error) {
	actionFiles := make(map[string]bool)
	seeds := make([]string, 0)
	for _, sourcePath := range actionSourcePaths(actions) {
		clean, err := canonicalModuleCopyPath(sourcePath)
		if err != nil {
			return nil, err
		}
		if !actionFiles[clean] {
			actionFiles[clean] = true
			seeds = append(seeds, clean)
		}
	}
	for _, includePath := range p.includeSourceFilePaths() {
		clean, err := canonicalModuleCopyPath(includePath)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(seeds, clean) {
			seeds = append(seeds, clean)
		}
	}
	// No action and no include sources means a middleware-only module: there
	// is nothing to discover, and the service tree may hold no loadable
	// package at all.
	if len(seeds) == 0 {
		return nil, nil
	}

	return moduleServiceHelperClosure(seeds, moduleServiceClosureConfig{
		serviceRoot:  p.SourceServiceDir,
		importPrefix: frameworkModulePath + "/internal/service/" + p.Name,
		actionFiles:  actionFiles,
		isExcluded:   p.canonicalIgnoredSourcePath,
		describe:     p.describeFrameworkPath,
	})
}

func (p *CopyPlan) addServiceFiles(helperFiles []string) error {
	for _, actions := range groupActionsByTargetPath(p.Actions) {
		first := actions[0]
		source, err := os.ReadFile(first.SourcePath)
		if err != nil {
			return err
		}
		target, err := generateTargetServiceShell(actions)
		if err != nil {
			return err
		}
		content, err := mergeModuleServiceSource(moduleServiceMergeInput{
			SourcePath: first.SourcePath,
			Source:     source,
			TargetPath: first.TargetPath,
			Target:     target,
			Rewrite:    p.rewriteConfig(first.TargetPath),
		})
		if err != nil {
			return err
		}
		p.Files = append(p.Files, moduleCopyFile{
			Kind:        moduleCopyFileService,
			TargetPath:  first.TargetPath,
			Content:     content,
			Preexisting: fileExists(first.TargetPath),
		})
	}

	for _, sourcePath := range helperFiles {
		targetPath, err := p.targetServicePath(sourcePath)
		if err != nil {
			return err
		}
		src, err := os.ReadFile(sourcePath)
		if err != nil {
			return err
		}
		content, err := normalizeModuleServiceSource(sourcePath, src, p.rewriteConfig(targetPath))
		if err != nil {
			return err
		}
		p.Files = append(p.Files, moduleCopyFile{
			Kind:        moduleCopyFileHelper,
			TargetPath:  targetPath,
			Content:     content,
			Preexisting: fileExists(targetPath),
		})
	}
	return nil
}

// collectStaleServiceFiles records target service files this copy plan will
// not write. Copied service packages hold module code only — project-owned
// code belongs outside them, as the module postNotes instruct — so a
// non-exempt leftover is a stale copy that the execution prune deletes.
func (p *CopyPlan) collectStaleServiceFiles() error {
	stale, err := p.staleTargetFiles(p.TargetServiceDir, moduleCopyFileService, moduleCopyFileHelper)
	if err != nil {
		return err
	}
	p.StaleServiceFiles = stale
	return nil
}

func groupActionsByTargetPath(actions []moduleCopyAction) [][]moduleCopyAction {
	if len(actions) == 0 {
		return nil
	}
	groups := make([][]moduleCopyAction, 0)
	for _, action := range actions {
		if len(groups) == 0 {
			groups = append(groups, []moduleCopyAction{action})
			continue
		}
		last := groups[len(groups)-1]
		if last[0].TargetPath == action.TargetPath {
			groups[len(groups)-1] = append(last, action)
			continue
		}
		groups = append(groups, []moduleCopyAction{action})
	}
	return groups
}

func actionSourcePaths(actions []moduleCopyAction) []string {
	paths := make([]string, 0, len(actions))
	seen := make(map[string]bool, len(actions))
	for _, action := range actions {
		if seen[action.SourcePath] {
			continue
		}
		seen[action.SourcePath] = true
		paths = append(paths, action.SourcePath)
	}
	sort.Strings(paths)
	return paths
}

func (p *CopyPlan) frameworkModelDir() string {
	return filepath.Join(p.FrameworkRoot, "internal", "model")
}

func (p *CopyPlan) frameworkServiceDir() string {
	return filepath.Join(p.FrameworkRoot, "internal", "service")
}

func (p *CopyPlan) targetServicePath(sourcePath string) (string, error) {
	rel, err := filepath.Rel(p.SourceServiceDir, sourcePath)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.Join(p.TargetServiceDir, rel), nil
	}

	sourceRoot, rootErr := canonicalModuleCopyPath(p.SourceServiceDir)
	cleanSource, sourceErr := canonicalModuleCopyPath(sourcePath)
	if rootErr != nil {
		return "", rootErr
	}
	if sourceErr != nil {
		return "", sourceErr
	}
	rel, err = filepath.Rel(sourceRoot, cleanSource)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("source service file %s is outside %s", sourcePath, p.SourceServiceDir)
	}
	return filepath.Join(p.TargetServiceDir, rel), nil
}

// rewriteConfig describes how a framework source file is retargeted at
// targetPath. Model, service helper and middleware copies share it: they differ
// only in whether service imports are rewritten, which is the normalize entry
// point's decision, not the config's.
func (p *CopyPlan) rewriteConfig(targetPath string) moduleCopyRewriteConfig {
	return moduleCopyRewriteConfig{
		ModuleName:        p.Name,
		ProjectModulePath: p.ProjectModulePath,
		ModelDir:          p.ModelDir,
		ServiceDir:        p.ServiceDir,
		TargetPackage:     moduleCopyPackageName(filepath.Dir(targetPath)),
	}
}
