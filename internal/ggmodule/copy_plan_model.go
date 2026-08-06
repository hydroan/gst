package ggmodule

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/hydroan/gst/internal/codegen/constants"
	"github.com/hydroan/gst/internal/codegen/gen"
)

func (p *CopyPlan) findModels() ([]*gen.ModelInfo, error) {
	allModels := make([]*gen.ModelInfo, 0)
	if err := filepath.Walk(p.SourceModelDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		base := filepath.Base(path)
		if path != p.SourceModelDir && (base == constants.DirVendor || base == constants.DirTestData) {
			return filepath.SkipDir
		}
		if info.IsDir() || !isGoSourceFile(info.Name()) || p.ignoredSourcePath(path) {
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

func (p *CopyPlan) collectExtraModelFiles() error {
	info, err := os.Stat(p.TargetModelDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", p.TargetModelDir)
	}

	// Model copy is a SourceModelDir -> TargetModelDir mirror after applying
	// module.json copy.excludeSourceFiles rules and source normalization. At this
	// point p.Files already contains every model file that this copy plan will
	// write, so comparing TargetModelDir against those planned model targets gives
	// a precise stale-file warning without treating excluded or project-owned
	// files as something module copy can delete automatically.
	expectedTargets := make(map[string]bool)
	for _, file := range p.Files {
		if file.Kind != moduleCopyFileModel {
			continue
		}
		rel, relErr := filepath.Rel(p.TargetModelDir, file.TargetPath)
		if relErr != nil {
			return relErr
		}
		expectedTargets[rel] = true
	}

	targetFiles, err := goFilesInDir(p.TargetModelDir)
	if err != nil {
		return err
	}
	for _, targetPath := range targetFiles {
		rel, err := filepath.Rel(p.TargetModelDir, targetPath)
		if err != nil {
			return err
		}
		if !expectedTargets[rel] {
			p.ExtraModelFiles = append(p.ExtraModelFiles, targetPath)
		}
	}
	sort.Strings(p.ExtraModelFiles)
	return nil
}

func (p *CopyPlan) targetModelInfo(source *gen.ModelInfo) (*gen.ModelInfo, error) {
	// Reuse gg gen's service generator by projecting the framework model into
	// the current project's model layout. The source model still drives action
	// DSL; only module/package/path metadata changes.
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
