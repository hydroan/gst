// Package columns generates the typed column references of a project's
// models, so filters name columns through the compiler instead of through
// string literals. Every model file gets a generated file of the same name
// declaring a <Model>Cols var per model it declares: model/sample/record.go,
// declaring Record, gets model/sample/record.gen.go with RecordCols. The
// columns come from gorm's own schema parser, run inside the project's module
// against the project's real model types by the inspection of package
// modelinspect.
package columns

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/modelinfo"
	"github.com/hydroan/gst/internal/modelinspect"
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
// projects never create, edit, or clean them up. A model file of the model
// directory named like one of the directory's own generated files is refused
// before anything is written, see registrationFileOwner. When it fails, the
// result still lists what it wrote and removed before the failure.
func Generate(module string, modelDir string, models []*modelinfo.Model, ignore gghelper.ProjectIgnore) (Result, error) {
	var result Result

	resolved, inspected, err := modelinspect.Inspect(module, modelDir, models, ignore, func() (map[string]string, error) {
		// The inspection build compiles the model packages before this run
		// writes their column references, so it stubs out the previous
		// generation and leaves out the handwritten code that reads it.
		return InspectionOverlay(module, modelDir, models, ignore)
	})
	if err != nil || !inspected {
		return result, err
	}

	// The scan that drives generation already knows which file declares each
	// model, so the resolved columns only need to be matched to it.
	sources := make(map[string]string, len(models))
	for _, m := range models {
		sources[m.ImportPath()+"."+m.ModelName] = m.ModelFilePath
	}

	byFile := groupColumnsByFile(resolved, sources)
	for _, file := range slices.Sorted(maps.Keys(byFile)) {
		if owner := registrationFileOwner(modelDir, columnsFileName(file)); owner != "" {
			return result, errors.Newf("the model file %s would get its column references at %s, the %s; rename the file", file, columnsFileName(file), owner)
		}
	}

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
func groupColumnsByFile(resolved []modelinspect.ModelColumns, sources map[string]string) map[string][]modelinspect.ModelColumns {
	byFile := make(map[string][]modelinspect.ModelColumns)
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
		if entry.IsDir() || !isColumnFileCandidate(dir, path) {
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
// of the model directory modelDir at all: it carries the generated suffix and
// is not one of the directory's own generated files, see
// registrationFileOwner: model/sample/record.gen.go and
// model/sample/model.gen.go can be one, while model/model.gen.go,
// model/apidoc.gen.go and model/sample/record.go cannot.
func isColumnFileCandidate(modelDir, path string) bool {
	return strings.HasSuffix(path, ggconst.SuffixGenGo) && registrationFileOwner(modelDir, path) == ""
}

// registrationFileOwner names the generation step owning path when it is one
// of the generated files of the model directory modelDir itself: the
// "registration file" for model/model.gen.go and the "api doc file" for
// model/apidoc.gen.go; "" for every other path, the same names in a
// subdirectory included, which are column files like any other.
func registrationFileOwner(modelDir, path string) string {
	if filepath.Dir(path) != filepath.Clean(modelDir) {
		return ""
	}
	switch filepath.Base(path) {
	case ggconst.FileModelGen:
		return "registration file"
	case ggconst.FileAPIDocGen:
		return "api doc file"
	}
	return ""
}
