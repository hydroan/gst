package ggmodule

import (
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/codegen/gen"
)

func (p *CopyPlan) findModels() ([]*gen.ModelInfo, error) {
	allModels := make([]*gen.ModelInfo, 0)
	if err := filepath.Walk(p.SourceModelDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			if skipModuleSourceDir(p.SourceModelDir, path, info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isGoSourceFile(info.Name()) || p.ignoredSourcePath(path) {
			return nil
		}

		models, err := gen.FindModels(frameworkModulePath, p.SourceModelDir, path)
		if err != nil {
			return err
		}
		for _, m := range models {
			m.ModelFilePath = path
			allModels = append(allModels, m)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return allModels, nil
}

func (p *CopyPlan) addModelFiles() error {
	files, err := goFilesInDir(p.SourceModelDir)
	if err != nil {
		return err
	}
	for _, sourcePath := range files {
		if p.ignoredSourcePath(sourcePath) {
			continue
		}
		rel, err := filepath.Rel(p.SourceModelDir, sourcePath)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(p.TargetModelDir, rel)
		src, err := os.ReadFile(sourcePath)
		if err != nil {
			return err
		}
		content, err := normalizeModuleModelSource(sourcePath, src, moduleCopyRewriteConfig{
			ModuleName:        p.Name,
			ProjectModulePath: p.ProjectModulePath,
			ModelDir:          p.ModelDir,
			ServiceDir:        p.ServiceDir,
			TargetPackage:     moduleCopyPackageName(filepath.Dir(targetPath)),
		})
		if err != nil {
			return err
		}
		p.Files = append(p.Files, moduleCopyFile{
			Kind:        moduleCopyFileModel,
			TargetPath:  targetPath,
			Content:     content,
			Preexisting: fileExists(targetPath),
		})
	}
	return nil
}

// requireExcludedModelFilesUnreferenced fails the plan when a model file the
// manifest excludes is still referenced by a model file the copy mirrors. The
// mirror ships every non-excluded file, so such a reference is guaranteed not
// to compile in the target project; reporting it at preflight names the broken
// manifest entry instead of shipping a broken package.
//
// The walk loads the model tree only when the manifest excludes files under
// it, and it covers references inside that tree. A copied service file
// referencing an excluded model symbol still surfaces through the copied
// project's build: the service closure walks the service tree only.
func (p *CopyPlan) requireExcludedModelFilesUnreferenced() error {
	modelTreePrefix := pathpkg.Join("internal", "model", p.Name) + "/"
	excludesModelSources := false
	for _, rel := range p.ExcludeSourceFiles {
		if strings.HasPrefix(rel, modelTreePrefix) {
			excludesModelSources = true
			break
		}
	}
	if !excludesModelSources {
		return nil
	}

	tree, err := loadModuleCopyPackageTree(p.SourceModelDir)
	if err != nil {
		return err
	}
	copied := make([]string, 0, len(tree.files))
	for path := range tree.files {
		if !p.canonicalIgnoredSourcePath(path) {
			copied = append(copied, path)
		}
	}
	sort.Strings(copied)
	for _, path := range copied {
		for _, declFile := range tree.referencedTreeFiles(path) {
			if p.canonicalIgnoredSourcePath(declFile) {
				return errors.Newf(
					"module copy: %s references %s, which excludeSourceFiles skips; remove the exclusion or the references",
					p.describeFrameworkPath(path), p.describeFrameworkPath(declFile),
				)
			}
		}
	}
	return nil
}

// collectStaleModelFiles records target model files this copy plan will not
// write. Model copy is a SourceModelDir -> TargetModelDir mirror, so a
// non-exempt leftover is a stale copy from an older framework version that
// the execution prune deletes.
func (p *CopyPlan) collectStaleModelFiles() error {
	stale, err := p.staleTargetFiles(p.TargetModelDir, moduleCopyFileModel)
	if err != nil {
		return err
	}
	p.StaleModelFiles = stale
	return nil
}

func (p *CopyPlan) targetModelInfo(source *gen.ModelInfo) (*gen.ModelInfo, error) {
	// Reuse gg gen's service generator by projecting the framework model into
	// the current project's model layout. The source model still drives action
	// DSL; only module/package/path metadata changes.
	//
	// BuildCopyPlan always fills SourceModelDir. The empty case is the plan a
	// test builds by hand to exercise action collection without a framework tree
	// on disk: it drops the model straight into TargetModelDir instead of
	// relocating it relative to the framework source root.
	if p.SourceModelDir == "" {
		target := *source
		target.ModulePath = p.ProjectModulePath
		if p.TargetModelDir != "" {
			target.ModelPkgName = moduleCopyPackageName(p.TargetModelDir)
			target.ModelFileDir = p.TargetModelDir
			target.ModelFilePath = filepath.Join(p.TargetModelDir, filepath.Base(source.ModelFilePath))
		}
		return &target, nil
	}
	targetPath, err := p.targetModelPath(source.ModelFilePath)
	if err != nil {
		return nil, err
	}
	target := *source
	target.ModulePath = p.ProjectModulePath
	target.ModelPkgName = moduleCopyPackageName(filepath.Dir(targetPath))
	target.ModelFileDir = filepath.Dir(targetPath)
	target.ModelFilePath = targetPath
	return &target, nil
}

func (p *CopyPlan) targetModelPath(sourcePath string) (string, error) {
	rel, err := filepath.Rel(p.SourceModelDir, sourcePath)
	if err != nil {
		return "", err
	}
	return filepath.Join(p.TargetModelDir, rel), nil
}
