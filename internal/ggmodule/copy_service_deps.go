package ggmodule

import (
	"fmt"
	"go/ast"
	"go/types"
	"path/filepath"
	"sort"

	"golang.org/x/tools/go/packages"
)

// moduleCopyHelperDependencyFiles uses go/packages type information instead of
// name matching. If selected action/helper files reference any top-level object
// declared in another helper file in the same service package, that whole file
// is added.
func moduleCopyHelperDependencyFiles(serviceDir string, selectedFiles []string) ([]string, error) {
	baseDir, err := filepath.Abs(serviceDir)
	if err != nil {
		return nil, err
	}

	selected := make(map[string]bool, len(selectedFiles))
	queue := make([]string, 0, len(selectedFiles))
	for _, file := range selectedFiles {
		clean, cleanErr := canonicalModuleCopyPath("", file)
		if cleanErr != nil {
			return nil, cleanErr
		}
		selected[clean] = true
		queue = append(queue, clean)
	}

	pkg, err := loadModuleCopyServicePackage(serviceDir)
	if err != nil {
		return nil, err
	}
	declFiles := packageDeclFiles(pkg, baseDir)
	helperCandidates := make(map[string]bool)
	for _, file := range pkg.GoFiles {
		if !isGoSourceFile(filepath.Base(file)) {
			continue
		}
		abs, err := canonicalModuleCopyPath(baseDir, file)
		if err != nil {
			return nil, err
		}
		if !selected[abs] {
			helperCandidates[abs] = true
		}
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		file := syntaxFileByPath(pkg, baseDir, current)
		if file == nil {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			ident, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			obj := pkg.TypesInfo.Uses[ident]
			if obj == nil || obj.Pkg() != pkg.Types {
				return true
			}
			declFile := declFiles[obj]
			if declFile == "" || selected[declFile] || !helperCandidates[declFile] {
				return true
			}
			selected[declFile] = true
			queue = append(queue, declFile)
			return true
		})
	}

	helpers := make([]string, 0)
	for file := range selected {
		if helperCandidates[file] {
			helpers = append(helpers, file)
		}
	}
	sort.Strings(helpers)
	return helpers, nil
}

func loadModuleCopyServicePackage(serviceDir string) (*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  serviceDir,
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, err
	}
	if len(pkgs) != 1 {
		return nil, fmt.Errorf("expected one service package in %s, found %d", serviceDir, len(pkgs))
	}
	if packages.PrintErrors(pkgs) > 0 {
		return nil, fmt.Errorf("failed to load service package %s", serviceDir)
	}
	return pkgs[0], nil
}

func packageDeclFiles(pkg *packages.Package, baseDir string) map[types.Object]string {
	files := make(map[types.Object]string)
	for ident, obj := range pkg.TypesInfo.Defs {
		if ident == nil || obj == nil {
			continue
		}
		if obj.Pkg() != pkg.Types {
			continue
		}
		pos := obj.Pos()
		for idx, syntax := range pkg.Syntax {
			if syntax.Pos() <= pos && pos <= syntax.End() {
				abs, err := canonicalModuleCopyPath(baseDir, pkg.GoFiles[idx])
				if err == nil {
					files[obj] = abs
				}
				break
			}
		}
	}
	return files
}

func syntaxFileByPath(pkg *packages.Package, baseDir string, path string) *ast.File {
	for idx, file := range pkg.GoFiles {
		abs, err := canonicalModuleCopyPath(baseDir, file)
		if err != nil {
			continue
		}
		if abs == path {
			return pkg.Syntax[idx]
		}
	}
	return nil
}
