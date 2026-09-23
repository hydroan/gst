// Package columns generates the typed column references of a project's
// models, so filters name columns through the compiler instead of through
// string literals. Every model file gets a generated file of the same name
// declaring a <Model>Cols var per model it declares: model/sample/record.go,
// declaring Record, gets model/sample/record.gen.go with RecordCols. The
// columns come from gorm's own schema parser, run by a program compiled
// inside the project's module against the project's real model types.
package columns

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
)

// Result lists the column files a Generate run changed on disk.
type Result struct {
	// Written lists the files written because they were missing or their
	// content changed; a file that is already current is not rewritten.
	Written []string

	// Removed lists the generated files removed because their model source is
	// gone.
	Removed []string
}

// Generate writes one .gen.go file per model source file and removes the
// generated files whose source is gone, and returns the files it wrote and
// removed. Generated files are framework-owned for their whole life cycle:
// projects never create, edit, or clean them up. When it fails, the result
// still lists what it wrote and removed before the failure.
func Generate(module string, modelDir string, models []*gen.ModelInfo, ignore gghelper.ProjectIgnore) (Result, error) {
	var result Result

	// Resolving columns compiles a program that imports the project's models,
	// which only works once the project depends on the framework. A project
	// that does not cannot hold column references either, so there is nothing
	// to generate yet.
	dependsOnGst, err := gghelper.RequiresFramework(".")
	if err != nil {
		return result, err
	}
	if !dependsOnGst {
		return result, nil
	}

	program := buildColumnsProgram(module, models)

	// Compiling and running the inspection program costs seconds, which would
	// otherwise be paid on every gg gen even when nothing that affects columns
	// changed. The cache key covers every such input, so a hit skips the build
	// entirely and a miss is unavoidable work.
	cacheKey, err := columnsCacheKey(program, modelDir, ignore)
	if err != nil {
		return result, err
	}
	resolved, cached := readColumnsCache(cacheKey)
	if !cached {
		// The inspection build compiles the model packages before this run
		// writes their column references, so it stubs out the previous
		// generation and leaves out the handwritten code that reads it.
		overlay, overlayErr := columnInspectionOverlay(module, modelDir, models, ignore)
		if overlayErr != nil {
			return result, overlayErr
		}
		if resolved, err = inspectColumns(program, overlay); err != nil {
			return result, err
		}
		if err = writeColumnsCache(cacheKey, resolved); err != nil {
			return result, err
		}
	}

	// The scan that drives generation already knows which file declares each
	// model, so the resolved columns only need to be matched to it.
	sources := make(map[string]string, len(models))
	for _, m := range models {
		sources[modelPkgPath(m)+"."+m.ModelName] = m.ModelFilePath
	}

	byFile := groupColumnsByFile(resolved, sources)

	wanted := make(map[string]struct{}, len(byFile))
	for file, entries := range byFile {
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
		target := columnsFileName(file)
		wanted[target] = struct{}{}
		content, renderErr := renderColumnsFile(module, entries[0].PkgName, file, entries)
		if renderErr != nil {
			return result, renderErr
		}
		written, writeErr := writeGeneratedFileIfChanged(target, content)
		if writeErr != nil {
			return result, writeErr
		}
		if written {
			result.Written = append(result.Written, target)
		}
	}

	result.Removed, err = removeOrphanColumnFiles(modelDir, wanted)
	return result, err
}

// groupColumnsByFile matches resolved models to the source files that declare
// them. A model that resolved no columns is dropped, because an empty Cols
// var would reference nothing; a model without a source file here was
// registered from outside the project's model directory, such as a framework
// module, and has no file to generate alongside.
func groupColumnsByFile(resolved []modelColumns, sources map[string]string) map[string][]modelColumns {
	byFile := make(map[string][]modelColumns)
	for _, m := range resolved {
		if len(m.Columns) == 0 {
			continue
		}
		file, ok := sources[m.PkgPath+"."+m.Name]
		if !ok {
			continue
		}
		byFile[file] = append(byFile[file], m)
	}
	return byFile
}

// columnsFileName returns the generated file that belongs to a model source
// file: model/sample/record.go becomes model/sample/record.gen.go.
func columnsFileName(source string) string {
	return strings.TrimSuffix(source, ggconst.ExtensionGo) + ggconst.SuffixGenGo
}

// writeGeneratedFileIfChanged writes content only when it differs from what is
// on disk, so an unchanged model does not churn file timestamps, and reports
// whether it wrote.
func writeGeneratedFileIfChanged(path string, content string) (bool, error) {
	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == content {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, errors.Wrapf(err, "read %s", path)
	}
	if err = os.WriteFile(path, []byte(content), ggconst.FileModeGenerated); err != nil {
		return false, errors.Wrapf(err, "write %s", path)
	}
	return true, nil
}

// removeOrphanColumnFiles deletes generated column files whose model source no
// longer declares any model, which happens when a model is deleted, renamed,
// or moved to another file, and returns the files it deleted. Only files
// carrying the generated header are removed, so a hand-written file is
// reported instead of destroyed.
func removeOrphanColumnFiles(dir string, wanted map[string]struct{}) ([]string, error) {
	var removed []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !isColumnFileCandidate(path) {
			return nil
		}
		if _, keep := wanted[path]; keep {
			return nil
		}
		content, err := os.ReadFile(path) //nolint:gosec // path comes from the model directory walk.
		if err != nil {
			return errors.Wrapf(err, "read %s", path)
		}
		if !strings.HasPrefix(string(content), consts.CodeGeneratedComment()) {
			return errors.Newf("%s uses the generated file suffix but was not generated by gst; rename it", path)
		}
		// #nosec G122 -- path comes from walking the project's own model
		// directory, and only files carrying the generated header reach here.
		if err = os.Remove(path); err != nil {
			return errors.Wrapf(err, "remove orphan generated file %s", path)
		}
		removed = append(removed, path)
		return nil
	})
	return removed, err
}

// isColumnFileCandidate reports whether path can be a generated column file
// at all: it carries the generated suffix and is not one of the files another
// generation step owns, such as the model registration and apidoc files:
// model/sample/record.gen.go can be one, while model/model.gen.go,
// model/apidoc.gen.go and model/sample/record.go cannot.
func isColumnFileCandidate(path string) bool {
	if !strings.HasSuffix(path, ggconst.SuffixGenGo) {
		return false
	}
	base := filepath.Base(path)
	return base != ggconst.FileModelGen && base != ggconst.FileAPIDocGen
}

// modelPkgPath rebuilds the import path of the package declaring a model:
// tmpapp/model/sample for a model of model/sample in module tmpapp.
func modelPkgPath(m *gen.ModelInfo) string {
	return packageImportPath(m.ModulePath, m.ModelFileDir)
}

// packageImportPath rebuilds the import path of the project package in dir, a
// directory relative to the module root: in module tmpapp, model/sample gives
// tmpapp/model/sample, and the module root, "." or "", gives tmpapp.
func packageImportPath(module string, dir string) string {
	dir = strings.Trim(filepath.ToSlash(filepath.Clean(dir)), "/")
	if dir == "" || dir == "." {
		return module
	}
	return module + "/" + dir
}
