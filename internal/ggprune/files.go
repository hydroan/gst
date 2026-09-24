// Package ggprune works out what gg prune deletes from a project's service
// directory, and deletes it: the service files of disabled actions, the
// unmanaged files of service directories no model owns, and the directories
// that leaves empty. Asking before deleting and printing what happened stay
// with the gg command. cmd/gg/PRUNE.md lays out the whole cleanup, step by
// step, with flowcharts.
//
// Of the ignore rules, prune goes by gst.yaml's prune.ignore alone. The
// service directory belongs to gg and is kept clean: a file there that
// nothing needs is litter even when the project's Git ignore rules or the go
// command's ignores (testdata, vendor, names beginning with "." or "_",
// nested modules, the directories go.mod ignores) cover it. So prune reads
// the directory whole, and prune.ignore is the one way to keep a path on
// purpose. gg check and gg gen, which read the project's code, go by both
// instead (see gghelper.ProjectIgnore).
package ggprune

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
)

// ScanServiceFiles lists the service files under serviceDir that gg manages:
// the standard phase files such as create.go and list.go, and any other .go
// file embedding service.Base[...], which a DSL Filename("x") produces. Only
// test files are left out; a file the project's Git ignore rules or the go
// command ignore is listed like any other. A walk error ends the scan, and the
// files found before it come back with it.
func ScanServiceFiles(serviceDir string) ([]string, error) {
	var files []string

	// Check if service directory exists
	if _, err := os.Stat(serviceDir); os.IsNotExist(err) {
		return files, nil
	}

	// Walk through the service directory
	err := filepath.Walk(serviceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && isManagedServiceFile(path) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// isManagedServiceFile reports whether path is a service file gg manages (see
// ScanServiceFiles): service/sample/record/create.go is, and so is
// service/sample/record/archive.go declaring a service that embeds
// service.Base[...], while service/sample/record/create_test.go and a
// service/sample/record/util.go declaring no such service are not.
func isManagedServiceFile(path string) bool {
	if !strings.HasSuffix(path, ".go") {
		return false
	}
	fileName := filepath.Base(path)
	if strings.HasSuffix(fileName, "_test.go") {
		return false
	}
	if phaseFileNames()[fileName] {
		return true
	}
	return gen.IsActionServiceSource(path)
}

// phaseFileNames returns the names of the standard phase files, create.go
// through sse.go.
func phaseFileNames() map[string]bool {
	return map[string]bool{
		consts.PHASE_CREATE.Filename():      true,
		consts.PHASE_DELETE.Filename():      true,
		consts.PHASE_UPDATE.Filename():      true,
		consts.PHASE_PATCH.Filename():       true,
		consts.PHASE_LIST.Filename():        true,
		consts.PHASE_GET.Filename():         true,
		consts.PHASE_CREATE_MANY.Filename(): true,
		consts.PHASE_DELETE_MANY.Filename(): true,
		consts.PHASE_UPDATE_MANY.Filename(): true,
		consts.PHASE_PATCH_MANY.Filename():  true,
		consts.PHASE_IMPORT.Filename():      true,
		consts.PHASE_EXPORT.Filename():      true,
		consts.PHASE_SSE.Filename():         true,
	}
}

// FilePlan is what prune intends for the service files of disabled actions.
type FilePlan struct {
	// Delete lists the files to delete, in the order they were scanned.
	Delete []string

	// Ignored lists the files a gst.yaml prune.ignore entry keeps.
	Ignored []string
}

// PlanFiles works out which existing service files prune deletes: the ones no
// enabled action of models expects any more, except the ones in kept, which
// belong to gst.yaml-ignored actions and must stay on disk, and the ones a
// prune.ignore entry in protect covers, which it lists as ignored instead.
func PlanFiles(existing []string, models []*gen.ModelInfo, kept map[string]bool, protect ggconfig.PruneConfig) FilePlan {
	// Get list of service files that should currently exist
	currentFiles := currentServiceFiles(models)

	// Find files to delete (exist in old list but not in current list)
	filesToDelete := make([]string, 0)
	for _, oldFile := range existing {
		if !currentFiles[oldFile] && !kept[oldFile] {
			filesToDelete = append(filesToDelete, oldFile)
		}
	}

	// Keep the files gst.yaml prune.ignore protects
	filesToDelete, ignoredFiles := filterIgnoredFiles(filesToDelete, protect)
	return FilePlan{Delete: filesToDelete, Ignored: ignoredFiles}
}

// currentServiceFiles returns the service files the enabled Service() actions
// of allModels expect, such as service/sample/record/create.go for a Create.
func currentServiceFiles(allModels []*gen.ModelInfo) map[string]bool {
	current := make(map[string]bool)
	for _, m := range allModels {
		m.Design.Range(func(route string, act *dsl.Action) {
			if act.Enabled && act.Service {
				target := gen.ServiceTarget(m, act, ggconst.DirModel, ggconst.DirService)
				current[target.FilePath] = true
			}
		})
	}
	return current
}

// filterIgnoredFiles splits files into the ones prune may delete and the ones
// a gst.yaml prune.ignore entry in protect covers.
func filterIgnoredFiles(files []string, protect ggconfig.PruneConfig) (filtered []string, ignored []string) {
	for _, file := range files {
		if protect.Ignores(file) {
			ignored = append(ignored, file)
		} else {
			filtered = append(filtered, file)
		}
	}
	return filtered, ignored
}

// RemoveFiles deletes paths in order and reports each one to report, with the
// error when it could not be deleted; a failure does not stop the rest.
func RemoveFiles(paths []string, report func(path string, err error)) {
	for _, path := range paths {
		report(path, os.Remove(path))
	}
}

// RemoveEmptyDirs removes the empty directories below rootDir, deepest first,
// keeping those a gst.yaml prune.ignore entry in protect covers, and reports
// each one it removes to report. Nothing else keeps a directory: an empty
// testdata directory, or one the project's Git ignore rules exclude, is
// removed like any other.
func RemoveEmptyDirs(rootDir string, protect ggconfig.PruneConfig, report func(dir string)) {
	dirs := make([]string, 0)
	_ = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == rootDir || !info.IsDir() || protect.Ignores(path) {
			return nil
		}

		dirs = append(dirs, path)
		return nil
	})

	sort.Slice(dirs, func(i, j int) bool {
		return directoryDepth(rootDir, dirs[i]) > directoryDepth(rootDir, dirs[j])
	})

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		if len(entries) == 0 {
			// #nosec G122 -- dir comes from walking rootDir, and only a
			// directory found empty reaches here.
			if err := os.Remove(dir); err == nil {
				report(dir)
			}
		}
	}
}

// directoryDepth returns how many levels below rootDir path lies: under
// service, service/sample/record lies 2 levels down, and service itself 0.
func directoryDepth(rootDir, path string) int {
	rel, err := filepath.Rel(rootDir, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}
