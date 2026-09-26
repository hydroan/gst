package ggmodule

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
)

// moduleServiceClosureConfig parameterizes helper discovery over one module's
// service source tree.
type moduleServiceClosureConfig struct {
	// serviceRoot is the module service tree root on disk.
	serviceRoot string
	// importPrefix is the framework import path of serviceRoot, used to map
	// blank imports of tree subpackages back to directories.
	importPrefix string
	// actionFiles are the canonical action service sources the plan already
	// copies through DSL actions; references into them need no helper copy.
	actionFiles map[string]bool
	// handlerFiles are the module's middleware and interceptor sources the
	// plan copies, files outside the tree whose references into it seed the
	// walk the way the action files' do: what a handler reaches for in the
	// service packages has to be copied along, or the copied handler does
	// not compile.
	handlerFiles []string
	// isExcluded reports whether a canonical path is manifest-excluded.
	isExcluded func(string) bool
	// describe renders a canonical path for error messages.
	describe func(string) string
}

// moduleServiceHelperClosure discovers the helper files one module copy must
// carry: the fixed point of "a copied file references a top-level object whose
// declaring file is not copied yet", walked with type information over the
// whole service tree so references may cross package boundaries, always at
// whole-file granularity.
//
// Two rules guard the walk. A referenced file that the manifest excludes fails
// the copy: the exclusion would strand the reference and the copied project
// could not compile. A referenced file that declares a service struct without
// being copied through a DSL action fails the copy too: action files are
// copied only through their actions, so shared code must live in helper files.
//
// A blank import of a tree subpackage carries no identifier the reference walk
// could follow, but it is an explicit request for the package's init side
// effects, so every file of that package joins the copy — minus files excluded
// by the manifest and files declaring a service struct, which are skipped
// rather than reported because a package-level import proves no need for any
// single file.
//
// The handler files are read through their imports of tree packages (see
// referencedByImporter) rather than walked: they are copied by their own
// channel and belong to packages the tree does not type-check.
func moduleServiceHelperClosure(seeds []string, config moduleServiceClosureConfig) ([]string, error) {
	tree, err := loadModuleCopyPackageTree(config.serviceRoot)
	if err != nil {
		return nil, err
	}

	selected := make(map[string]bool, len(seeds))
	queue := make([]string, 0, len(seeds))
	helpers := make([]string, 0)
	// Seeds outside the action set are manifest includeSourceFiles: no other
	// channel copies them, so they are helper output themselves, not just
	// closure starting points.
	for _, seed := range seeds {
		if selected[seed] {
			continue
		}
		selected[seed] = true
		queue = append(queue, seed)
		if !config.actionFiles[seed] {
			helpers = append(helpers, seed)
		}
	}

	addHelper := func(path string) {
		selected[path] = true
		helpers = append(helpers, path)
		queue = append(queue, path)
	}
	// admit adds declFile, which the file at from references, under the two
	// rules above, unless the copy carries it already.
	admit := func(from, declFile string) error {
		if selected[declFile] || config.actionFiles[declFile] {
			return nil
		}
		if config.isExcluded(declFile) {
			return errors.Newf(
				"module copy: %s references %s, which excludeSourceFiles skips; remove the exclusion or the references",
				config.describe(from), config.describe(declFile),
			)
		}
		if len(serviceStructNames(tree.files[declFile].syntax)) > 0 {
			return errors.Newf(
				"module copy: %s references %s, which declares a service struct but is copied by no DSL action; move the shared code into a helper file",
				config.describe(from), config.describe(declFile),
			)
		}
		addHelper(declFile)
		return nil
	}

	for _, handler := range config.handlerFiles {
		referenced, referencedErr := tree.referencedByImporter(handler)
		if referencedErr != nil {
			return nil, referencedErr
		}
		for _, declFile := range referenced {
			if admitErr := admit(handler, declFile); admitErr != nil {
				return nil, admitErr
			}
		}
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		file, ok := tree.files[current]
		if !ok {
			continue
		}

		for _, declFile := range tree.referencedTreeFiles(current) {
			if admitErr := admit(current, declFile); admitErr != nil {
				return nil, admitErr
			}
		}

		for _, dir := range blankImportTreeDirs(file.syntax, config) {
			for _, packageFile := range tree.filesInDir(dir) {
				if selected[packageFile] || config.actionFiles[packageFile] || config.isExcluded(packageFile) {
					continue
				}
				if len(serviceStructNames(tree.files[packageFile].syntax)) > 0 {
					continue
				}
				addHelper(packageFile)
			}
		}
	}

	sort.Strings(helpers)
	return helpers, nil
}

// blankImportTreeDirs returns the canonical directories of tree subpackages
// the file imports blankly, for their init side effects.
func blankImportTreeDirs(file *ast.File, config moduleServiceClosureConfig) []string {
	dirs := make([]string, 0)
	for _, imp := range file.Imports {
		if imp.Name == nil || imp.Name.Name != "_" {
			continue
		}
		importPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if !importsTree(importPath, config.importPrefix) {
			continue
		}
		suffix := strings.TrimPrefix(strings.TrimPrefix(importPath, config.importPrefix), "/")
		dir, dirErr := canonicalModuleCopyPath(filepath.Join(config.serviceRoot, filepath.FromSlash(suffix)))
		if dirErr != nil {
			continue
		}
		dirs = append(dirs, dir)
	}
	return dirs
}

// importsTree reports whether importPath names the tree at importPrefix or
// a package under it.
func importsTree(importPath, importPrefix string) bool {
	return importPath == importPrefix || strings.HasPrefix(importPath, importPrefix+"/")
}

// fileImportsTree reports whether the Go file at path imports a package of
// the tree at importPrefix, reading its imports alone.
func fileImportsTree(path, importPrefix string) (bool, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		return false, err
	}
	for _, imp := range file.Imports {
		importPath, unquoteErr := strconv.Unquote(imp.Path.Value)
		if unquoteErr != nil {
			continue
		}
		if importsTree(importPath, importPrefix) {
			return true, nil
		}
	}
	return false, nil
}
