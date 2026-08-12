package ggmodule

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/codegen/gen"
)

type moduleCopyRewriteConfig struct {
	ModuleName        string
	ProjectModulePath string
	ModelDir          string
	ServiceDir        string
	TargetPackage     string
}

// normalizeModuleModelSource converts framework model files into the current
// project package layout. The model directory name is the package name, so
// internal/model/copytest package modelcopytest becomes model/copytest package copytest.
// Copied model files can reference sibling model packages in the same framework
// module, so those internal model imports must also be rewritten to the target
// project's model/<module> tree.
func normalizeModuleModelSource(filename string, src []byte, config moduleCopyRewriteConfig) ([]byte, error) {
	// Model files intentionally only rewrite copied model imports. If a model file
	// imports a copied service package, keeping that import untouched preserves the
	// architecture violation instead of hiding it in generated project code.
	return normalizeModuleCopySource(filename, src, config, false)
}

// normalizeModuleServiceSource rewrites helper files into the current service
// package and maps framework internal model/service imports to the current
// project's copied module package tree.
func normalizeModuleServiceSource(filename string, src []byte, config moduleCopyRewriteConfig) ([]byte, error) {
	return normalizeModuleCopySource(filename, src, config, true)
}

// normalizeModuleMiddlewareSource rewrites manifest-declared middleware files
// into the current project's middleware package. Middleware can legitimately
// depend on copied model/service packages, so it uses the same import rewrite
// rules as service helpers while keeping the target package owned by the
// middleware destination directory.
func normalizeModuleMiddlewareSource(filename string, src []byte, config moduleCopyRewriteConfig) ([]byte, error) {
	return normalizeModuleCopySource(filename, src, config, true)
}

func normalizeModuleCopySource(filename string, src []byte, config moduleCopyRewriteConfig, includeServiceImports bool) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	selectors := rewriteSelectorPackages(file, rewriteModuleCopyFile(file, config, includeServiceImports))
	if err = selectors.requireUnshadowed(filename, fset, file); err != nil {
		return nil, err
	}
	if err = requirePublicFrameworkImports(filename, file); err != nil {
		return nil, err
	}

	code, err := gen.FormatNodeExtraWithFileSet(file, fset, true)
	if err != nil {
		return nil, err
	}
	return []byte(code), nil
}

// requirePublicFrameworkImports rejects a copied source that still imports a
// framework internal package after the module-owned rewrites ran. Inside the
// framework such an import compiles, but the copied file lives in the consumer
// project, where Go forbids it; failing the copy names the file instead of
// shipping code that cannot build.
func requirePublicFrameworkImports(filename string, file *ast.File) error {
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if strings.HasPrefix(path, frameworkModulePath+"/internal/") {
			return errors.Newf("module copy source %s imports framework internal package %q; copied files must import public framework packages", filename, path)
		}
	}
	return nil
}

func rewriteModuleCopyFile(file *ast.File, config moduleCopyRewriteConfig, includeServiceImports bool) map[string]string {
	file.Name.Name = config.TargetPackage

	rewrites := make([]moduleCopyImportRewrite, 0)
	usedNames := make(map[string]bool)
	// An import left as-is still occupies its local name, so the alias search for
	// rewritten imports must not hand that name out again.
	markUsedName := func(imp *ast.ImportSpec, path string) {
		name := importLocalName(imp, path)
		if name != "" && name != "." && name != "_" {
			usedNames[name] = true
		}
	}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		rewrite, ok := buildModuleCopyImportRewrite(imp, path, config)
		if !ok {
			markUsedName(imp, path)
			continue
		}
		if rewrite.kind == moduleCopyImportService && !includeServiceImports {
			markUsedName(imp, path)
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
		// A name the copy leaves alone needs no selector work, and skipping it keeps
		// requireUnshadowed from blaming copy for shadowing the module source already had.
		if !rewrite.keepSpecialName && rewrite.oldName != "" && newName != rewrite.oldName {
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
		selected := indexes[0]
		for _, index := range indexes {
			if rewrites[index].kind == moduleCopyImportModel {
				selected = index
				break
			}
		}
		preferred[selected] = true
	}
	return preferred
}

// moduleCopyImportKind is which copied package tree an import points at. The
// string values are not cosmetic: moduleCopyImportAlias prefixes generated
// aliases with them, so changing a value changes the copied source.
type moduleCopyImportKind string

const (
	moduleCopyImportModel   moduleCopyImportKind = "model"
	moduleCopyImportService moduleCopyImportKind = "service"
)

type moduleCopyImportRewrite struct {
	spec            *ast.ImportSpec
	oldName         string
	newPath         string
	desiredName     string
	kind            moduleCopyImportKind
	keepSpecialName bool
}

func buildModuleCopyImportRewrite(imp *ast.ImportSpec, sourcePath string, config moduleCopyRewriteConfig) (moduleCopyImportRewrite, bool) {
	modelPrefix := frameworkModulePath + "/internal/model/" + config.ModuleName
	servicePrefix := frameworkModulePath + "/internal/service/" + config.ModuleName
	switch {
	case sourcePath == modelPrefix || strings.HasPrefix(sourcePath, modelPrefix+"/"):
		return newModuleCopyImportRewrite(imp, sourcePath, modelPrefix, projectModuleCopyImportRoot(config.ProjectModulePath, config.ModelDir, config.ModuleName), moduleCopyImportModel), true
	case sourcePath == servicePrefix || strings.HasPrefix(sourcePath, servicePrefix+"/"):
		return newModuleCopyImportRewrite(imp, sourcePath, servicePrefix, projectModuleCopyImportRoot(config.ProjectModulePath, config.ServiceDir, config.ModuleName), moduleCopyImportService), true
	default:
		return moduleCopyImportRewrite{}, false
	}
}

func newModuleCopyImportRewrite(imp *ast.ImportSpec, sourcePath string, sourcePrefix string, targetPrefix string, kind moduleCopyImportKind) moduleCopyImportRewrite {
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

func moduleCopyImportAlias(kind moduleCopyImportKind, packageName string) string {
	return sanitizeModuleCopyIdentifier(string(kind) + packageName)
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
	first, _ := firstRune(result)
	if first == '_' || unicode.IsLetter(first) {
		return result
	}
	return "copied" + result
}

func firstRune(value string) (rune, int) {
	for i, r := range value {
		return r, i
	}
	return 0, -1
}

func moduleCopyPackageName(dir string) string {
	return sanitizeModuleCopyIdentifier(filepath.Base(dir))
}

// moduleCopySelectors records every selector whose package qualifier carries a
// copied import's new local name, in source order, flagging the ones the rewrite
// produced. Copy renames a module's own imports (modeliamsession becomes
// session), so a qualifier that was unmistakable in the module source can land
// on a name the file already binds. requireUnshadowed needs both halves to tell
// those apart: a flagged selector must reach the package, while an unflagged one
// was already written against a declaration and is none of copy's business.
type moduleCopySelectors struct {
	copiedNames map[string]bool
	fromRewrite []bool
}

func rewriteSelectorPackages(node ast.Node, names map[string]string) moduleCopySelectors {
	selectors := moduleCopySelectors{copiedNames: make(map[string]bool, len(names))}
	if len(names) == 0 {
		return selectors
	}
	for _, newName := range names {
		selectors.copiedNames[newName] = true
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
		if newName, rewritten := names[ident.Name]; rewritten {
			ident.Name = newName
			selectors.fromRewrite = append(selectors.fromRewrite, true)
		} else if selectors.copiedNames[ident.Name] {
			selectors.fromRewrite = append(selectors.fromRewrite, false)
		}
		return true
	})
	return selectors
}

// requireUnshadowed reports a rewritten package qualifier that a declaration in
// the same file swallows. go/parser applies the real Go scope rules while it
// parses, so rendering the rewritten file and reading it back says exactly which
// qualifiers still reach the package: an identifier resolved to a declaration
// carries an ast.Object, one left for the linker to bind does not. Rendering the
// file also keeps the walks in lockstep — the round trip preserves the tree, so
// the n-th matching selector here is the n-th one rewriteSelectorPackages saw.
func (s moduleCopySelectors) requireUnshadowed(filename string, fset *token.FileSet, file *ast.File) error {
	if len(s.fromRewrite) == 0 {
		return nil
	}

	var rendered bytes.Buffer
	if err := format.Node(&rendered, fset, file); err != nil {
		return err
	}
	resolvedFset := token.NewFileSet()
	resolved, err := parser.ParseFile(resolvedFset, filename, rendered.Bytes(), 0)
	if err != nil {
		return err
	}

	var shadowed error
	index := 0
	ast.Inspect(resolved, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || !s.copiedNames[ident.Name] {
			return true
		}
		matched := index
		index++
		if shadowed != nil || matched >= len(s.fromRewrite) || !s.fromRewrite[matched] || ident.Obj == nil {
			return true
		}
		shadowed = errors.Newf(
			"%s: copied package reference %s.%s resolves to a declaration named %q in the same file; rename that declaration in the module source",
			filename, ident.Name, sel.Sel.Name, ident.Name,
		)
		return true
	})
	return shadowed
}
