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
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
)

// OrphanDir is a service directory no model owns, with the unmanaged files in
// it: the files cleaning it deletes, or, for a helper directory kept because
// live project code imports it, the files it holds.
type OrphanDir struct {
	Path  string
	Files []string
}

// FindOrphanDirs resolves service directory ownership and returns the orphan
// directories plus the helper directories kept because live project code
// still imports them. Directories in keptDirs hold service files of
// gst.yaml-ignored actions and are treated as owned; keptDirs may be nil.
// orphanFiles are the files outside the service directory that orphan cleanup
// deletes along with the orphans, such as the middleware a removed module
// left behind; what they import keeps nothing. What the gst.yaml prune.ignore
// entries in protect cover is never an orphan. It returns an error, and no
// directories, when the imports of part of the project cannot be read,
// because a directory or a file cannot be opened or its imports do not parse:
// an import it could not see might be all that keeps a directory.
func FindOrphanDirs(allModels []*gen.ModelInfo, keptDirs map[string]bool, orphanFiles []string, modulePath string, protect ggconfig.PruneConfig) (orphans, keptHelpers []OrphanDir, err error) {
	currentDirs := currentServiceDirs(allModels)
	for dir := range keptDirs {
		currentDirs.ownedDirs = append(currentDirs.ownedDirs, dir)
		addServiceDirAncestors(currentDirs.knownDirs, dir)
	}
	sort.Strings(currentDirs.ownedDirs)

	helperDirs, err := importedServiceHelperDirs(currentDirs, orphanFiles, modulePath, protect)
	if err != nil {
		return nil, nil, err
	}
	keptHelpers = make([]OrphanDir, 0, len(helperDirs))
	for _, dir := range helperDirs {
		keptHelpers = append(keptHelpers, OrphanDir{
			Path:  dir,
			Files: unmanagedFilesUnderDir(dir),
		})
		currentDirs.ownedDirs = append(currentDirs.ownedDirs, dir)
		addServiceDirAncestors(currentDirs.knownDirs, dir)
	}
	sort.Strings(currentDirs.ownedDirs)

	orphans = scanOrphanServiceDirs(currentDirs, protect)
	return orphans, keptHelpers, nil
}

// serviceDirSet is the part of the service directory orphan cleanup leaves
// alone.
type serviceDirSet struct {
	// knownDirs holds every owned directory and every directory between one
	// and the service root, the root included.
	knownDirs map[string]bool

	// ownedDirs lists the directories treated as owned, together with
	// everything below them: the ones the enabled Service() actions write to,
	// and the ones FindOrphanDirs adds for gst.yaml-ignored actions and for
	// imported helpers.
	ownedDirs []string
}

// currentServiceDirs returns the directories the enabled Service() actions of
// allModels write their service files to as the owned ones, each known
// together with the directories above it: a Create writing
// service/sample/record/create.go owns service/sample/record, which makes
// service/sample/record, service/sample and service known.
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
		knownDirs: knownDirs,
		ownedDirs: modelDirs,
	}
}

// addServiceDirAncestors adds dir and every directory above it, up to the
// service root, to knownDirs: service/sample/record adds itself,
// service/sample and service.
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
// whose imports follow the models they were generated from, orphanFiles,
// which orphan cleanup deletes, and the files of the service directories no
// model owns. A file of such a directory turns live once live code imports the
// directory, once it sits right in a directory between an imported one and the
// service root, which orphan cleanup leaves alone, or when a gst.yaml
// prune.ignore entry in protect covers it: prune deletes none of them. So a
// stale service/service.gen.go keeps nothing a deleted model left behind, and
// a leftover keeps nothing it imports. No ignore rule leaves a Go file out, as
// prune reads the project whole: a file the project's Git ignore rules
// exclude, or one below testdata or a directory named with a leading "_",
// counts like any other. Only what a symbolic link alone leads to is not
// read. An import of a service directory no longer on disk keeps nothing.
func importedServiceHelperDirs(currentDirs serviceDirSet, orphanFiles []string, modulePath string, protect ggconfig.PruneConfig) ([]string, error) {
	importPrefix := modulePath + "/" + filepath.ToSlash(filepath.Clean(ggconst.DirService))
	serviceRoot := filepath.Clean(ggconst.DirService)
	deleted := make(map[string]bool, len(orphanFiles))
	for _, file := range orphanFiles {
		deleted[filepath.Clean(file)] = true
	}

	helperDirs := make([]string, 0)
	helperDirSet := make(map[string]bool)
	helperAncestors := make(map[string]bool)
	owned := func(dir string) bool {
		if currentDirs.knownDirs[dir] || helperDirSet[dir] {
			return true
		}
		if isInsideAnyDir(dir, currentDirs.ownedDirs) {
			return true
		}
		return isInsideAnyDir(dir, helperDirs)
	}
	live := func(path string) bool {
		if strings.HasSuffix(path, ggconst.SuffixGenGo) || deleted[filepath.Clean(path)] {
			return false
		}
		dir := filepath.Dir(path)
		return !isPathInsideDir(dir, serviceRoot) || owned(dir) || helperAncestors[dir] || protect.Ignores(path)
	}

	queue, err := importedServiceDirsUnderDir(".", importPrefix, live)
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
		imported, err := importedServiceDirsUnderDir(dir, importPrefix, live)
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
			})
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
// project-local service imports. It walks dir whole, as prune reads the
// project, and does not follow symbolic links. A directory or file that cannot
// be read fails the walk, and so does a file whose imports do not parse.
func importedServiceDirsUnderDir(dir string, importPrefix string, live func(path string) bool) ([]string, error) {
	dirs := make([]string, 0)
	seen := make(map[string]bool)
	fset := token.NewFileSet()

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || !live(path) {
			return nil
		}
		// Imports that do not parse fail the walk like a file that cannot be
		// read: one the parser gave up on might be all that keeps a directory.
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
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
// directory it points at: under the prefix tmpapp/service,
// tmpapp/service/iam/adminauth maps to service/iam/adminauth, while
// tmpapp/model/sample maps to nothing.
func serviceDirForImport(importPath string, importPrefix string) (string, bool) {
	if !strings.HasPrefix(importPath, importPrefix+"/") {
		return "", false
	}
	rel := strings.TrimPrefix(importPath, importPrefix+"/")
	return filepath.Join(filepath.Clean(ggconst.DirService), filepath.FromSlash(rel)), true
}

// scanOrphanServiceDirs returns the orphan directories below the service
// root, visiting the shallower directories first: a directory that is not
// known, lies inside no owned directory and no orphan found already, is not
// covered by a prune.ignore entry in protect, and holds unmanaged files
// prune.ignore does not cover. Every directory is judged on its own, as prune
// reads the project whole: a testdata, vendor or hidden directory, a nested
// module, or a directory named with a leading "_" no model owns is an orphan
// like any other.
func scanOrphanServiceDirs(currentDirs serviceDirSet, protect ggconfig.PruneConfig) []OrphanDir {
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
		if currentDirs.knownDirs[dir] || isInsideAnyDir(dir, currentDirs.ownedDirs) || isUnderOrphanServiceDir(dir, orphans) {
			continue
		}
		if protect.Ignores(dir) {
			continue
		}

		// A file prune.ignore covers stays out of the orphan's files, so
		// cleaning the orphan leaves it, and a directory whose unmanaged
		// files are all covered is no orphan at all.
		files := slices.DeleteFunc(unmanagedFilesUnderDir(dir), protect.Ignores)
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

// isInsideAnyDir reports whether dir lies below one of dirs (see
// isPathInsideDir).
func isInsideAnyDir(dir string, dirs []string) bool {
	for _, parent := range dirs {
		if isPathInsideDir(dir, parent) {
			return true
		}
	}
	return false
}

// isUnderOrphanServiceDir reports whether dir lies below one of orphanDirs.
func isUnderOrphanServiceDir(dir string, orphanDirs []OrphanDir) bool {
	for _, orphan := range orphanDirs {
		if isPathInsideDir(dir, orphan.Path) {
			return true
		}
	}
	return false
}

// isPathInsideDir reports whether path lies below dir: service/sample/record
// lies below service/sample, while service/sample itself and service/samplex
// do not.
func isPathInsideDir(path string, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// unmanagedFilesUnderDir returns the files below dir, its subdirectories
// included, that gg does not manage (see isManagedServiceFile). No ignore rule
// leaves a file out: a testdata file or one the project's Git ignore rules
// exclude goes with dir.
func unmanagedFilesUnderDir(dir string) []string {
	files := make([]string, 0)
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
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
