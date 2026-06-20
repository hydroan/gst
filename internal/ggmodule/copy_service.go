package ggmodule

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
	"golang.org/x/tools/go/packages"
)

type moduleActionMergeInput struct {
	SourcePath            string
	Source                []byte
	TargetPath            string
	Target                []byte
	ModuleName            string
	TargetModelImportPath string
	MethodName            string
}

// generateTargetServiceShell builds the same empty service action file that gg
// gen will create in the current project. The merge step then keeps its struct
// name, method signature, package, and current-project model import.
func generateTargetServiceShell(modelInfo *gen.ModelInfo, action *dsl.Action) ([]byte, error) {
	file := gen.GenerateService(modelInfo, action, action.Phase)
	if file == nil {
		return nil, fmt.Errorf("failed to generate service shell for %s", action.ServiceFilename())
	}
	fset := token.NewFileSet()
	code, err := gen.FormatNodeExtraWithFileSet(file, fset, true)
	if err != nil {
		return nil, err
	}
	return []byte(code), nil
}

// mergeModuleActionServiceSource overlays source business logic onto a generated
// current-project service shell. It copies method docs and bodies, ordinary
// declarations from the source action file, and imports needed by that logic,
// while preserving the target service struct and method signature.
func mergeModuleActionServiceSource(input moduleActionMergeInput) ([]byte, error) {
	fset := token.NewFileSet()
	targetFile, err := parser.ParseFile(fset, input.TargetPath, input.Target, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	sourceFile, err := parser.ParseFile(fset, input.SourcePath, input.Source, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	sourceModelNames := rewriteModuleServiceFile(sourceFile, input.ModuleName, input.TargetModelImportPath)
	rewriteSelectorPackages(sourceFile, sourceModelNames, input.ModuleName)

	sourceStruct := findServiceStructName(sourceFile)
	if sourceStruct == "" {
		return nil, fmt.Errorf("source action service file %s has no service struct", input.SourcePath)
	}
	targetStruct := findServiceStructName(targetFile)
	if targetStruct == "" {
		return nil, fmt.Errorf("target action service file %s has no service struct", input.TargetPath)
	}

	sourceMethod := findMethod(sourceFile, sourceStruct, input.MethodName)
	if sourceMethod == nil {
		return nil, fmt.Errorf("source action service file %s has no %s method", input.SourcePath, input.MethodName)
	}
	targetMethod := findMethod(targetFile, targetStruct, input.MethodName)
	if targetMethod == nil {
		return nil, fmt.Errorf("target action service file %s has no %s method", input.TargetPath, input.MethodName)
	}

	targetRecv := methodReceiverName(targetMethod)
	sourceRecv := methodReceiverName(sourceMethod)
	if sourceRecv != "" && targetRecv != "" && sourceRecv != targetRecv && sourceMethod.Body != nil {
		renameIdent(sourceMethod.Body, sourceRecv, targetRecv)
	}
	methodDoc := commentGroupLines(sourceMethod.Doc)
	targetMethod.Doc = nil
	targetMethod.Body = sourceMethod.Body

	mergeImports(targetFile, sourceFile.Imports)
	appendSourceOrdinaryDecls(targetFile, sourceFile, sourceStruct)

	code, err := gen.FormatNodeExtraWithFileSet(targetFile, fset, true)
	if err != nil {
		return nil, err
	}
	code = insertMethodDoc(code, targetStruct, input.MethodName, methodDoc)
	return []byte(code), nil
}

func commentGroupLines(doc *ast.CommentGroup) []string {
	if doc == nil {
		return nil
	}
	lines := make([]string, 0, len(doc.List))
	for _, comment := range doc.List {
		lines = append(lines, comment.Text)
	}
	return lines
}

func insertMethodDoc(code string, receiverType string, methodName string, docLines []string) string {
	if len(docLines) == 0 {
		return code
	}
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "func (") || !strings.Contains(trimmed, " "+methodName+"(") {
			continue
		}
		if !strings.Contains(trimmed, "*"+receiverType+")") && !strings.Contains(trimmed, " "+receiverType+")") {
			continue
		}
		if i > 0 && strings.TrimSpace(lines[i-1]) == docLines[len(docLines)-1] {
			return code
		}
		insert := append([]string{}, docLines...)
		lines = append(lines[:i], append(insert, lines[i:]...)...)
		return strings.Join(lines, "\n")
	}
	return code
}

func appendSourceOrdinaryDecls(targetFile *ast.File, sourceFile *ast.File, sourceStruct string) {
	// Action service files use near-whole-file copy semantics for local business
	// declarations, but the generated target service shell owns the service struct
	// and receiver methods.
	for _, decl := range sourceFile.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				continue
			}
			filtered := filterSourceSpecs(d, sourceStruct)
			if filtered != nil {
				targetFile.Decls = append(targetFile.Decls, filtered)
			}
		case *ast.FuncDecl:
			if d.Recv != nil && receiverTypeName(d) == sourceStruct {
				continue
			}
			targetFile.Decls = append(targetFile.Decls, d)
		default:
			targetFile.Decls = append(targetFile.Decls, d)
		}
	}
}

func filterSourceSpecs(decl *ast.GenDecl, sourceStruct string) *ast.GenDecl {
	if decl.Tok != token.TYPE {
		return decl
	}
	specs := make([]ast.Spec, 0, len(decl.Specs))
	for _, spec := range decl.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if ok && typeSpec.Name.Name == sourceStruct && isServiceTypeSpec(typeSpec) {
			continue
		}
		specs = append(specs, spec)
	}
	if len(specs) == 0 {
		return nil
	}
	copied := *decl
	copied.Specs = specs
	return &copied
}

func mergeImports(targetFile *ast.File, imports []*ast.ImportSpec) {
	if len(imports) == 0 {
		return
	}
	seen := make(map[string]bool)
	var targetImportDecl *ast.GenDecl
	for _, decl := range targetFile.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.IMPORT {
			continue
		}
		if targetImportDecl == nil {
			targetImportDecl = genDecl
		}
		for _, spec := range genDecl.Specs {
			imp, ok := spec.(*ast.ImportSpec)
			if !ok {
				continue
			}
			seen[importKey(imp)] = true
		}
	}
	if targetImportDecl == nil {
		targetImportDecl = &ast.GenDecl{Tok: token.IMPORT}
		targetFile.Decls = append([]ast.Decl{targetImportDecl}, targetFile.Decls...)
	}
	for _, imp := range imports {
		key := importKey(imp)
		if seen[key] {
			continue
		}
		seen[key] = true
		targetImportDecl.Specs = append(targetImportDecl.Specs, cloneImportSpec(imp))
	}
}

func importKey(imp *ast.ImportSpec) string {
	name := ""
	if imp.Name != nil {
		name = imp.Name.Name
	}
	return name + ":" + imp.Path.Value
}

func cloneImportSpec(imp *ast.ImportSpec) *ast.ImportSpec {
	cloned := &ast.ImportSpec{
		Path: &ast.BasicLit{Kind: token.STRING, Value: imp.Path.Value},
	}
	if imp.Name != nil {
		cloned.Name = ast.NewIdent(imp.Name.Name)
	}
	return cloned
}

func findServiceStructName(file *ast.File) string {
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || !isServiceTypeSpec(typeSpec) {
				continue
			}
			return typeSpec.Name.Name
		}
	}
	return ""
}

func countServiceStructsInFile(path string) (int, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return 0, err
	}
	var count int
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if ok && isServiceTypeSpec(typeSpec) {
				count++
			}
		}
	}
	return count, nil
}

func sourceServiceMethodExists(path string, methodName string) bool {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return false
	}
	serviceStruct := findServiceStructName(file)
	return serviceStruct != "" && findMethod(file, serviceStruct, methodName) != nil
}

func isServiceTypeSpec(typeSpec *ast.TypeSpec) bool {
	structType, ok := typeSpec.Type.(*ast.StructType)
	if !ok || structType.Fields == nil {
		return false
	}
	for _, field := range structType.Fields.List {
		if len(field.Names) > 0 {
			continue
		}
		if isServiceBaseExpr(field.Type) {
			return true
		}
	}
	return false
}

func isServiceBaseExpr(expr ast.Expr) bool {
	var x ast.Expr
	switch e := expr.(type) {
	case *ast.IndexExpr:
		x = e.X
	case *ast.IndexListExpr:
		x = e.X
	default:
		return false
	}
	sel, ok := x.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Base" {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == "service"
}

func findMethod(file *ast.File, recvType string, methodName string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != methodName {
			continue
		}
		if receiverTypeName(fn) == recvType {
			return fn
		}
	}
	return nil
}

func methodReceiverName(fn *ast.FuncDecl) string {
	if fn == nil || fn.Recv == nil || len(fn.Recv.List) == 0 || len(fn.Recv.List[0].Names) == 0 {
		return ""
	}
	return fn.Recv.List[0].Names[0].Name
}

func renameIdent(node ast.Node, oldName string, newName string) {
	ast.Inspect(node, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if ok && ident.Name == oldName {
			ident.Name = newName
		}
		return true
	})
}

// moduleCopyHelperDependencyFiles uses go/packages type information instead of
// name matching. If selected action/helper files reference any top-level object
// declared in another helper file in the same package, that whole file is added.
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
		if !isModuleCopyGoSource(filepath.Base(file)) {
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

func canonicalModuleCopyPath(baseDir string, path string) (string, error) {
	if !filepath.IsAbs(path) && baseDir != "" {
		path = filepath.Join(baseDir, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	realPath, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return realPath, nil
	}
	return abs, nil
}
