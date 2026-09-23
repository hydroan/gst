package ggprune

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
)

type serviceDirSet struct {
	KnownDirs map[string]bool
	ModelDirs []string
}

// OrphanDir is a service directory no model owns, with the unmanaged files in
// it: the files cleaning it deletes, or, for a helper directory kept because
// live project code imports it, the files it holds.
type OrphanDir struct {
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
		consts.PHASE_SSE.Filename():         true,
	}
}

func currentServiceDirs(allModels []*gen.ModelInfo) serviceDirSet {
	knownDirs := map[string]bool{
		filepath.Clean(ggconst.DirService): true,
	}
	modelDirs := make([]string, 0, len(allModels))
	modelDirSet := make(map[string]bool)

	for _, m := range allModels {
		m.Design.Range(func(route string, act *dsl.Action) {
			if !act.Enabled || !act.Service {
				return
			}
			dir := filepath.Clean(gen.ServiceTarget(m, act, ggconst.DirModel, ggconst.DirService).Dir)
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
	root := filepath.Clean(ggconst.DirService)
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

func scanOrphanServiceDirs(currentDirs serviceDirSet, protect ggconfig.PruneConfig, ignore gghelper.ProjectIgnore) []OrphanDir {
	root := filepath.Clean(ggconst.DirService)
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

	orphans := make([]OrphanDir, 0)
	for _, dir := range dirs {
		if currentDirs.KnownDirs[dir] || isUnderCurrentModelServiceDir(dir, currentDirs.ModelDirs) || isUnderOrphanServiceDir(dir, orphans) {
			continue
		}
		if protect.Ignores(dir) {
			continue
		}

		// A file prune.ignore covers stays out of the orphan's files, so
		// cleaning the orphan leaves it, and a directory whose unmanaged
		// files are all covered is no orphan at all.
		files := slices.DeleteFunc(unmanagedFilesUnderDir(dir, ignore), protect.Ignores)
		if len(files) == 0 {
			continue
		}

		orphans = append(orphans, OrphanDir{
			Path:  dir,
			Files: files,
		})
	}

	return orphans
}

func isUnderCurrentModelServiceDir(dir string, modelDirs []string) bool {
	for _, modelDir := range modelDirs {
		if isPathInsideDir(dir, modelDir) {
			return true
		}
	}
	return false
}

func isUnderOrphanServiceDir(dir string, orphanDirs []OrphanDir) bool {
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

func unmanagedFilesUnderDir(dir string, ignore gghelper.ProjectIgnore) []string {
	files := make([]string, 0)
	_ = ignore.Walk(dir, func(path string, info os.FileInfo) error {
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

// FindOrphanDirs resolves service directory ownership and returns
// the orphan directories plus the helper directories kept because live
// project code still imports them. Directories in keptDirs hold service files
// of gst.yaml-ignored actions and are treated as owned; keptDirs may be nil.
// What the gst.yaml prune.ignore entries in protect cover is never an orphan.
// It returns an error, and no directories, when part of the project cannot be
// read: an import it could not see might be all that keeps a directory.
func FindOrphanDirs(allModels []*gen.ModelInfo, keptDirs map[string]bool, modulePath string, protect ggconfig.PruneConfig, ignore gghelper.ProjectIgnore) (orphans, keptHelpers []OrphanDir, err error) {
	currentDirs := currentServiceDirs(allModels)
	for dir := range keptDirs {
		currentDirs.ModelDirs = append(currentDirs.ModelDirs, dir)
		addServiceDirAncestors(currentDirs.KnownDirs, dir)
	}
	sort.Strings(currentDirs.ModelDirs)

	helperDirs, err := importedServiceHelperDirs(currentDirs, modulePath, protect, ignore)
	if err != nil {
		return nil, nil, err
	}
	keptHelpers = make([]OrphanDir, 0, len(helperDirs))
	for _, dir := range helperDirs {
		keptHelpers = append(keptHelpers, OrphanDir{
			Path:  dir,
			Files: unmanagedFilesUnderDir(dir, ignore),
		})
		currentDirs.ModelDirs = append(currentDirs.ModelDirs, dir)
		addServiceDirAncestors(currentDirs.KnownDirs, dir)
	}
	sort.Strings(currentDirs.ModelDirs)

	orphans = scanOrphanServiceDirs(currentDirs, protect, ignore)
	return orphans, keptHelpers, nil
}

// importedServiceHelperDirs returns the service directories no model action
// owns but live project code still imports, directly or transitively.
// modulePath is the project's, which the command read before generating.
// Module copy installs such shared helper packages (for example
// iam/adminauth), and a cronjob or a middleware may import one as well;
// deleting them would break the build, so orphan cleanup must treat them as
// owned.
//
// Live code is every Go file of the project, test files and files a build
// constraint leaves out included, but for the .gen.go files gg generates,
// whose imports follow the models they were generated from, and the files of
// the service directories no model owns. A
// file of such a directory turns live once live code imports the directory,
// once it sits right in a directory between an imported one and the service
// root, which orphan cleanup leaves alone, or when a gst.yaml prune.ignore
// entry in protect covers it: prune deletes none of them. So a stale
// service/service.gen.go keeps nothing a deleted model left behind, and a
// leftover keeps nothing it imports. What the project's code walks leave out
// holds no live code: the directories gghelper.ExcludedDir names, the paths
// the Git ignore rules exclude, and what only a symbolic link leads to. An
// import of a service directory no longer on disk keeps nothing.
func importedServiceHelperDirs(currentDirs serviceDirSet, modulePath string, protect ggconfig.PruneConfig, ignore gghelper.ProjectIgnore) ([]string, error) {
	importPrefix := modulePath + "/" + filepath.ToSlash(filepath.Clean(ggconst.DirService))
	serviceRoot := filepath.Clean(ggconst.DirService)

	helperDirs := make([]string, 0)
	helperDirSet := make(map[string]bool)
	helperAncestors := make(map[string]bool)
	owned := func(dir string) bool {
		if currentDirs.KnownDirs[dir] || helperDirSet[dir] {
			return true
		}
		if isUnderCurrentModelServiceDir(dir, currentDirs.ModelDirs) {
			return true
		}
		return isUnderCurrentModelServiceDir(dir, helperDirs)
	}
	live := func(path string) bool {
		if strings.HasSuffix(path, ggconst.SuffixGenGo) {
			return false
		}
		dir := filepath.Dir(path)
		return !isPathInsideDir(dir, serviceRoot) || owned(dir) || helperAncestors[dir] || protect.Ignores(path)
	}

	queue, err := importedServiceDirsUnderDir(".", importPrefix, live, ignore)
	if err != nil {
		return nil, err
	}
	for len(queue) > 0 {
		dir := queue[0]
		queue = queue[1:]
		// An import of a directory no longer on disk leaves nothing to keep.
		if owned(dir) || !gghelper.FileExists(dir) {
			continue
		}
		helperDirSet[dir] = true
		helperDirs = append(helperDirs, dir)
		imported, err := importedServiceDirsUnderDir(dir, importPrefix, live, ignore)
		if err != nil {
			return nil, err
		}
		queue = append(queue, imported...)

		// The files right in the directories between the helper and the
		// service root turn live with it; the walk below each directory reads
		// only those, its subdirectories being settled on their own.
		for parent := filepath.Dir(dir); !owned(parent) && !helperAncestors[parent]; parent = filepath.Dir(parent) {
			helperAncestors[parent] = true
			imported, err := importedServiceDirsUnderDir(parent, importPrefix, func(path string) bool {
				return filepath.Dir(path) == parent && live(path)
			}, ignore)
			if err != nil {
				return nil, err
			}
			queue = append(queue, imported...)
		}
	}

	sort.Strings(helperDirs)
	return helperDirs, nil
}

// importedServiceDirsUnderDir parses the imports of the Go files under dir
// that live accepts and returns the service directories referenced through
// project-local service imports. It walks dir the way gg walks the project's
// code, leaving out what gghelper.ExcludedDir names and not following
// symbolic links. A file whose imports stop parsing still counts with the
// ones read before the error; a directory or file that cannot be read fails
// the walk.
func importedServiceDirsUnderDir(dir string, importPrefix string, live func(path string) bool, ignore gghelper.ProjectIgnore) ([]string, error) {
	dirs := make([]string, 0)
	seen := make(map[string]bool)
	fset := token.NewFileSet()

	err := ignore.Walk(dir, func(path string, info os.FileInfo) error {
		if info.IsDir() {
			if gghelper.ExcludedDir(dir, path) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || !live(path) {
			return nil
		}
		// A syntax error still leaves the imports read before it; only a
		// file that could not be read comes back without an AST.
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if file == nil {
			return err
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
	if err != nil {
		return nil, errors.Wrapf(err, "read the Go files under %s", dir)
	}

	sort.Strings(dirs)
	return dirs, nil
}

// serviceDirForImport maps a project-local service import path to the service
// directory it points at.
func serviceDirForImport(importPath string, importPrefix string) (string, bool) {
	if !strings.HasPrefix(importPath, importPrefix+"/") {
		return "", false
	}
	rel := strings.TrimPrefix(importPath, importPrefix+"/")
	return filepath.Join(filepath.Clean(ggconst.DirService), filepath.FromSlash(rel)), true
}
