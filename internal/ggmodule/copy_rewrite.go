package ggmodule

import (
	"go/ast"
	"go/parser"
	"go/token"
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/hydroan/gst/internal/codegen/gen"
)

// normalizeModuleModelSource converts framework model files into the current
// project package layout. The model directory name is the package name, so
// internal/model/copytest package modelcopytest becomes model/copytest package copytest.
// Copied model files can reference sibling model packages in the same framework
// module, so those internal model imports must also be rewritten to the target
// project's model/<module> tree.
func normalizeModuleModelSource(filename string, src []byte, config moduleCopyRewriteConfig) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	selectorNames := rewriteModuleModelFile(file, config)
	rewriteSelectorPackages(file, selectorNames)

	code, err := gen.FormatNodeExtraWithFileSet(file, fset, true)
	if err != nil {
		return nil, err
	}
	return []byte(code), nil
}

func rewriteModuleModelFile(file *ast.File, config moduleCopyRewriteConfig) map[string]string {
	// Model files intentionally only rewrite copied model imports. If a model file
	// imports a copied service package, keeping that import untouched preserves the
	// architecture violation instead of hiding it in generated project code.
	return rewriteModuleCopyFile(file, config, false)
}

type moduleCopyRewriteConfig struct {
	ModuleName        string
	ProjectModulePath string
	ModelDir          string
	ServiceDir        string
	TargetPackage     string
}

// normalizeModuleServiceSource rewrites helper files into the current service
// package and maps framework internal model/service imports to the current
// project's copied module package tree.
func normalizeModuleServiceSource(filename string, src []byte, config moduleCopyRewriteConfig) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	selectorNames := rewriteModuleServiceFile(file, config)
	rewriteSelectorPackages(file, selectorNames)

	code, err := gen.FormatNodeExtraWithFileSet(file, fset, true)
	if err != nil {
		return nil, err
	}
	return []byte(code), nil
}

func rewriteModuleServiceFile(file *ast.File, config moduleCopyRewriteConfig) map[string]string {
	return rewriteModuleCopyFile(file, config, true)
}

func rewriteModuleCopyFile(file *ast.File, config moduleCopyRewriteConfig, includeServiceImports bool) map[string]string {
	file.Name.Name = config.TargetPackage

	rewrites := make([]moduleCopyImportRewrite, 0)
	usedNames := make(map[string]bool)
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		rewrite, ok := buildModuleCopyImportRewrite(imp, path, config)
		if !ok {
			name := importLocalName(imp, path)
			if name != "" && name != "." && name != "_" {
				usedNames[name] = true
			}
			continue
		}
		if rewrite.kind == "service" && !includeServiceImports {
			name := importLocalName(imp, path)
			if name != "" && name != "." && name != "_" {
				usedNames[name] = true
			}
			continue
		}
		rewrites = append(rewrites, rewrite)
	}

	desiredCounts := make(map[string]int)
	for _, rewrite := range rewrites {
		desiredCounts[rewrite.desiredName]++
	}
	preferredUnaliased := preferredUnaliasedModuleCopyImports(rewrites, usedNames)

	selectorNames := make(map[string]string)
	for i, rewrite := range rewrites {
		newName := rewrite.desiredName
		if rewrite.keepSpecialName {
			newName = rewrite.oldName
		} else if usedNames[rewrite.desiredName] || (desiredCounts[rewrite.desiredName] > 1 && !preferredUnaliased[i]) {
			newName = moduleCopyImportAlias(rewrite.kind, rewrite.desiredName)
		}
		for usedNames[newName] && newName != rewrite.oldName {
			newName += "x"
		}
		usedNames[newName] = true

		rewrite.spec.Path.Value = strconv.Quote(rewrite.newPath)
		if rewrite.keepSpecialName || newName != rewrite.desiredName {
			rewrite.spec.Name = ast.NewIdent(newName)
		} else {
			rewrite.spec.Name = nil
		}
		if !rewrite.keepSpecialName && rewrite.oldName != "" {
			selectorNames[rewrite.oldName] = newName
		}
	}
	return selectorNames
}

func preferredUnaliasedModuleCopyImports(rewrites []moduleCopyImportRewrite, usedNames map[string]bool) map[int]bool {
	preferred := make(map[int]bool)
	byName := make(map[string][]int)
	for i, rewrite := range rewrites {
		if rewrite.keepSpecialName || usedNames[rewrite.desiredName] {
			continue
		}
		byName[rewrite.desiredName] = append(byName[rewrite.desiredName], i)
	}
	for _, indexes := range byName {
		if len(indexes) == 0 {
			continue
		}
		selected := indexes[0]
		for _, index := range indexes {
			if rewrites[index].kind == "model" {
				selected = index
				break
			}
		}
		preferred[selected] = true
	}
	return preferred
}

type moduleCopyImportRewrite struct {
	spec            *ast.ImportSpec
	oldName         string
	newPath         string
	desiredName     string
	kind            string
	keepSpecialName bool
}

func buildModuleCopyImportRewrite(imp *ast.ImportSpec, sourcePath string, config moduleCopyRewriteConfig) (moduleCopyImportRewrite, bool) {
	modelPrefix := frameworkModulePath + "/internal/model/" + config.ModuleName
	servicePrefix := frameworkModulePath + "/internal/service/" + config.ModuleName
	switch {
	case sourcePath == modelPrefix || strings.HasPrefix(sourcePath, modelPrefix+"/"):
		return newModuleCopyImportRewrite(imp, sourcePath, modelPrefix, projectModuleCopyImportRoot(config.ProjectModulePath, config.ModelDir, config.ModuleName), "model"), true
	case sourcePath == servicePrefix || strings.HasPrefix(sourcePath, servicePrefix+"/"):
		return newModuleCopyImportRewrite(imp, sourcePath, servicePrefix, projectModuleCopyImportRoot(config.ProjectModulePath, config.ServiceDir, config.ModuleName), "service"), true
	default:
		return moduleCopyImportRewrite{}, false
	}
}

func newModuleCopyImportRewrite(imp *ast.ImportSpec, sourcePath string, sourcePrefix string, targetPrefix string, kind string) moduleCopyImportRewrite {
	suffix := strings.TrimPrefix(sourcePath, sourcePrefix)
	newPath := targetPrefix + suffix
	name := importLocalName(imp, sourcePath)
	return moduleCopyImportRewrite{
		spec:            imp,
		oldName:         name,
		newPath:         newPath,
		desiredName:     pathpkg.Base(newPath),
		kind:            kind,
		keepSpecialName: name == "." || name == "_",
	}
}

func projectModuleCopyImportRoot(projectModulePath string, dir string, moduleName string) string {
	return filepath.ToSlash(filepath.Join(projectModulePath, dir, moduleName))
}

func importLocalName(imp *ast.ImportSpec, importPath string) string {
	if imp.Name != nil {
		return imp.Name.Name
	}
	return pathpkg.Base(importPath)
}

func moduleCopyImportAlias(kind string, packageName string) string {
	return sanitizeModuleCopyIdentifier(kind + packageName)
}

func sanitizeModuleCopyIdentifier(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	if builder.Len() == 0 {
		return "copied"
	}
	result := builder.String()
	first, _ := utf8FirstRune(result)
	if first == '_' || unicode.IsLetter(first) {
		return result
	}
	return "copied" + result
}

func utf8FirstRune(value string) (rune, int) {
	for i, r := range value {
		return r, i
	}
	return 0, -1
}

func moduleCopyPackageName(dir string) string {
	return sanitizeModuleCopyIdentifier(filepath.Base(dir))
}

func rewriteSelectorPackages(node ast.Node, names map[string]string) {
	if len(names) == 0 {
		return
	}
	ast.Inspect(node, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		newName, ok := names[ident.Name]
		if !ok {
			return true
		}
		ident.Name = newName
		return true
	})
}
