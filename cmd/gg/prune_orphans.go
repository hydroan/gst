package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/types/consts"
)

const cleanOrphansConfirmation = "delete orphan service leftovers"

type serviceDirSet struct {
	KnownDirs map[string]bool
	ModelDirs []string
}

type orphanServiceDir struct {
	Path  string
	Files []string
}

func validServicePhaseFiles() map[string]bool {
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
	}
}

func matchesPrunePattern(path string, pattern string) bool {
	if matched, err := regexp.MatchString(pattern, path); err == nil && matched {
		return true
	}
	return strings.Contains(path, pattern)
}

func currentServiceDirs(allModels []*gen.ModelInfo) serviceDirSet {
	knownDirs := map[string]bool{
		filepath.Clean(serviceDir): true,
	}
	modelDirs := make([]string, 0, len(allModels))
	modelDirSet := make(map[string]bool)

	for _, m := range allModels {
		m.Design.Range(func(route string, act *dsl.Action) {
			if !act.Enabled || !act.Service {
				return
			}
			dir := filepath.Clean(gen.ServiceTarget(m, act, modelDir, serviceDir).Dir)
			if !modelDirSet[dir] {
				modelDirSet[dir] = true
				modelDirs = append(modelDirs, dir)
			}
			addServiceDirAncestors(knownDirs, dir)
		})
	}

	sort.Strings(modelDirs)
	return serviceDirSet{
		KnownDirs: knownDirs,
		ModelDirs: modelDirs,
	}
}

func addServiceDirAncestors(knownDirs map[string]bool, dir string) {
	root := filepath.Clean(serviceDir)
	for {
		knownDirs[dir] = true
		if dir == root || dir == "." || dir == string(filepath.Separator) {
			return
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

func scanOrphanServiceDirs(currentDirs serviceDirSet, ignorePatterns []string) []orphanServiceDir {
	root := filepath.Clean(serviceDir)
	dirs := make([]string, 0)

	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			//nolint:nilerr
			return nil
		}
		if path == root || !info.IsDir() {
			return nil
		}
		dirs = append(dirs, filepath.Clean(path))
		return nil
	})

	sort.Slice(dirs, func(i, j int) bool {
		leftDepth := directoryDepth(root, dirs[i])
		rightDepth := directoryDepth(root, dirs[j])
		if leftDepth == rightDepth {
			return dirs[i] < dirs[j]
		}
		return leftDepth < rightDepth
	})

	orphans := make([]orphanServiceDir, 0)
	for _, dir := range dirs {
		if currentDirs.KnownDirs[dir] || isUnderCurrentModelServiceDir(dir, currentDirs.ModelDirs) || isUnderOrphanServiceDir(dir, orphans) {
			continue
		}
		if ignoredByPrunePatterns(dir, ignorePatterns) {
			continue
		}

		files := unmanagedFilesUnderDir(dir)
		if len(files) == 0 {
			continue
		}

		orphans = append(orphans, orphanServiceDir{
			Path:  dir,
			Files: files,
		})
	}

	return orphans
}

func ignoredByPrunePatterns(path string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchesPrunePattern(path, pattern) {
			return true
		}
	}
	return false
}

func isUnderCurrentModelServiceDir(dir string, modelDirs []string) bool {
	for _, modelDir := range modelDirs {
		if isPathInsideDir(dir, modelDir) {
			return true
		}
	}
	return false
}

func isUnderOrphanServiceDir(dir string, orphanDirs []orphanServiceDir) bool {
	for _, orphan := range orphanDirs {
		if isPathInsideDir(dir, orphan.Path) {
			return true
		}
	}
	return false
}

func isPathInsideDir(path string, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func unmanagedFilesUnderDir(dir string) []string {
	files := make([]string, 0)
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			//nolint:nilerr
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !isManagedServiceFile(path) {
			files = append(files, path)
		}
		return nil
	})

	sort.Strings(files)
	return files
}

func isManagedServiceFile(path string) bool {
	if !strings.HasSuffix(path, ".go") {
		return false
	}
	fileName := filepath.Base(path)
	if strings.HasSuffix(fileName, "_test.go") {
		return false
	}
	if validServicePhaseFiles()[fileName] {
		return true
	}
	return gen.IsActionServiceSource(path)
}

// handleOrphanServiceDirs reports or cleans service directories no model
// owns. Directories in keptDirs hold service files of gst.yaml-ignored
// actions and are treated as owned; keptDirs may be nil.
func handleOrphanServiceDirs(allModels []*gen.ModelInfo, keptDirs map[string]bool) {
	currentDirs := currentServiceDirs(allModels)
	for dir := range keptDirs {
		currentDirs.ModelDirs = append(currentDirs.ModelDirs, dir)
		addServiceDirAncestors(currentDirs.KnownDirs, dir)
	}
	sort.Strings(currentDirs.ModelDirs)

	orphans := scanOrphanServiceDirs(currentDirs, getPruneOrphanIgnorePatterns())
	if len(orphans) == 0 {
		return
	}

	if cleanOrphans {
		reportOrphanServiceDirs("Unmanaged Orphan Service Directories", orphans)
		if !confirmCleanOrphanServiceDirs() {
			clioutput.Item("", "Orphan service directory cleanup canceled")
			return
		}
		cleanOrphanServiceDirs(orphans)
		return
	}

	reportOrphanServiceDirs("Unmanaged Orphan Service Directories Kept", orphans)
}

func reportOrphanServiceDirs(section string, orphans []orphanServiceDir) {
	clioutput.Section(section)
	for _, orphan := range orphans {
		clioutput.Item("", "%s", orphan.Path)
		clioutput.Item("", "no current model maps to this service directory")
		clioutput.Item("", "contains unmanaged files:")
		for _, file := range orphan.Files {
			clioutput.Item("", "%s", file)
		}
	}
}

func confirmCleanOrphanServiceDirs() bool {
	clioutput.Warn("", "This will delete unmanaged files that gg cannot prove it owns.")
	clioutput.Prompt("Type %q to continue: ", cleanOrphansConfirmation)

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil && len(response) == 0 {
		return false
	}
	return strings.TrimSpace(response) == cleanOrphansConfirmation
}

func cleanOrphanServiceDirs(orphans []orphanServiceDir) {
	for _, orphan := range orphans {
		for _, file := range orphan.Files {
			if err := os.Remove(file); err != nil {
				clioutput.Error("", "Failed to delete %s: %v", file, err)
			} else {
				clioutput.Success("", "Deleted %s", file)
			}
		}
	}
	removeEmptyDirectories(serviceDir)
}
