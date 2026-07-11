package main

import (
	"bufio"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

// collectOrphanServiceDirs resolves service directory ownership and returns
// the orphan directories plus the helper directories kept because live
// service code still imports them. Directories in keptDirs hold service files
// of gst.yaml-ignored actions and are treated as owned; keptDirs may be nil.
func collectOrphanServiceDirs(allModels []*gen.ModelInfo, keptDirs map[string]bool) (orphans, keptHelpers []orphanServiceDir) {
	currentDirs := currentServiceDirs(allModels)
	for dir := range keptDirs {
		currentDirs.ModelDirs = append(currentDirs.ModelDirs, dir)
		addServiceDirAncestors(currentDirs.KnownDirs, dir)
	}
	sort.Strings(currentDirs.ModelDirs)

	helperDirs := importedServiceHelperDirs(currentDirs)
	keptHelpers = make([]orphanServiceDir, 0, len(helperDirs))
	for _, dir := range helperDirs {
		keptHelpers = append(keptHelpers, orphanServiceDir{
			Path:  dir,
			Files: unmanagedFilesUnderDir(dir),
		})
		currentDirs.ModelDirs = append(currentDirs.ModelDirs, dir)
		addServiceDirAncestors(currentDirs.KnownDirs, dir)
	}
	sort.Strings(currentDirs.ModelDirs)

	orphans = scanOrphanServiceDirs(currentDirs, getPruneOrphanIgnorePatterns())
	return orphans, keptHelpers
}

// importedServiceHelperDirs returns service directories that no model action
// owns but live service code under the owned directories still imports,
// directly or transitively. Module copy installs such shared helper packages
// (for example iam/adminauth); deleting them would break the build, so orphan
// cleanup must treat them as owned.
func importedServiceHelperDirs(currentDirs serviceDirSet) []string {
	modulePath := currentProjectModulePath()
	if modulePath == "" {
		return nil
	}
	importPrefix := modulePath + "/" + filepath.ToSlash(filepath.Clean(serviceDir))

	helperDirs := make([]string, 0)
	helperDirSet := make(map[string]bool)
	scanned := make(map[string]bool)
	scanQueue := make([]string, 0, len(currentDirs.ModelDirs))
	for _, dir := range currentDirs.ModelDirs {
		if !scanned[dir] {
			scanned[dir] = true
			scanQueue = append(scanQueue, dir)
		}
	}

	ownedOrDiscovered := func(dir string) bool {
		if currentDirs.KnownDirs[dir] || helperDirSet[dir] {
			return true
		}
		if isUnderCurrentModelServiceDir(dir, currentDirs.ModelDirs) {
			return true
		}
		return isUnderCurrentModelServiceDir(dir, helperDirs)
	}

	for len(scanQueue) > 0 {
		current := scanQueue[0]
		scanQueue = scanQueue[1:]
		for _, dir := range importedServiceDirsUnderDir(current, importPrefix) {
			if ownedOrDiscovered(dir) || scanned[dir] {
				continue
			}
			helperDirSet[dir] = true
			helperDirs = append(helperDirs, dir)
			scanned[dir] = true
			scanQueue = append(scanQueue, dir)
		}
	}

	sort.Strings(helperDirs)
	return helperDirs
}

// importedServiceDirsUnderDir parses the imports of every Go file under dir
// and returns the service directories referenced through project-local
// service imports. Files that fail to parse are skipped.
func importedServiceDirsUnderDir(dir string, importPrefix string) []string {
	dirs := make([]string, 0)
	seen := make(map[string]bool)
	fset := token.NewFileSet()

	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			//nolint:nilerr
			return nil
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			//nolint:nilerr
			return nil
		}
		for _, imp := range file.Imports {
			importPath, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				continue
			}
			imported, ok := serviceDirForImport(importPath, importPrefix)
			if !ok || seen[imported] {
				continue
			}
			seen[imported] = true
			dirs = append(dirs, imported)
		}
		return nil
	})

	sort.Strings(dirs)
	return dirs
}

// serviceDirForImport maps a project-local service import path to the service
// directory it points at.
func serviceDirForImport(importPath string, importPrefix string) (string, bool) {
	if !strings.HasPrefix(importPath, importPrefix+"/") {
		return "", false
	}
	rel := strings.TrimPrefix(importPath, importPrefix+"/")
	return filepath.Join(filepath.Clean(serviceDir), filepath.FromSlash(rel)), true
}

// handleOrphanServiceDirs reports or cleans service directories no model
// owns; see collectOrphanServiceDirs for the ownership rules.
func handleOrphanServiceDirs(allModels []*gen.ModelInfo, keptDirs map[string]bool) {
	orphans, keptHelpers := collectOrphanServiceDirs(allModels, keptDirs)
	reportKeptServiceHelperDirs(keptHelpers)
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
		clioutput.Item("", "%s (no current model maps to this directory)", orphan.Path)
		for _, file := range orphan.Files {
			clioutput.Line(clioutput.StyleMuted, "    - %s", file)
		}
	}
}

// reportKeptServiceHelperDirs explains why unmanaged helper directories
// survived orphan cleanup: live service code still imports them.
func reportKeptServiceHelperDirs(keptHelpers []orphanServiceDir) {
	if len(keptHelpers) == 0 {
		return
	}
	clioutput.Section("Service Helper Directories Kept")
	for _, helper := range keptHelpers {
		clioutput.Item("", "%s (imported by live service files)", helper.Path)
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
