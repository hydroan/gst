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

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/codegen/gen"
	"golang.org/x/tools/go/packages"
)

type moduleServiceMergeInput struct {
	SourcePath            string
	Source                []byte
	TargetPath            string
	Target                []byte
	ModuleName            string
	TargetModelImportPath string
}

// generateTargetServiceShell builds the same service file shell that gg gen will
// create in the current project for all actions sharing one service filename.
func generateTargetServiceShell(actions []moduleCopyAction) ([]byte, error) {
	if len(actions) == 0 {
		return nil, errors.New("failed to generate service shell: no actions")
	}
	var file *ast.File
	for _, action := range actions {
		next := gen.GenerateServiceWithPackage(action.ModelInfo, action.Action, action.Action.Phase, moduleCopyServicePackageName(action))
		if next == nil {
			return nil, fmt.Errorf("failed to generate service shell for %s", action.Action.ServiceFilename())
		}
		if file == nil {
			file = next
			continue
		}
		mergeImports(file, next.Imports)
		appendGeneratedServiceDecls(file, next)
	}
	fset := token.NewFileSet()
	code, err := gen.FormatNodeExtraWithFileSet(file, fset, true)
	if err != nil {
		return nil, err
	}
	return []byte(code), nil
}

func moduleCopyServicePackageName(action moduleCopyAction) string {
	if action.Action != nil && action.Action.Flatten && action.ModelInfo != nil {
		return action.ModelInfo.ModelPkgName
	}
	if action.ModelInfo == nil {
		return ""
	}
	return strings.ToLower(action.ModelInfo.ModelName)
}

func appendGeneratedServiceDecls(targetFile *ast.File, generatedFile *ast.File) {
	targetStruct := findServiceStructName(targetFile)
	for _, decl := range generatedFile.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				continue
			}
			filtered := filterSourceSpecs(d, targetStruct)
			if filtered != nil {
				targetFile.Decls = append(targetFile.Decls, filtered)
			}
		case *ast.FuncDecl:
			if d.Recv != nil && receiverTypeName(d) == targetStruct && findMethod(targetFile, targetStruct, d.Name.Name) != nil {
				continue
			}
			targetFile.Decls = append(targetFile.Decls, d)
		default:
			targetFile.Decls = append(targetFile.Decls, d)
		}
	}
}

// mergeModuleServiceSource overlays the framework service source onto a generated
// current-project service shell. The target shell owns package naming, imports,
// service struct identity, and generated action signatures. The source file owns
// business logic, hooks, receiver helper methods, ordinary declarations, and comments.
func mergeModuleServiceSource(input moduleServiceMergeInput) ([]byte, error) {
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

	structDoc := retargetDocLines(commentGroupLines(serviceStructDoc(sourceFile, sourceStruct)), sourceStruct, targetStruct)
	sourceComments := ast.NewCommentMap(fset, sourceFile, sourceFile.Comments)

	mergeImports(targetFile, sourceFile.Imports)
	docInserts := mergeSourceServiceDecls(targetFile, sourceFile, sourceStruct, targetStruct, sourceComments)

	code, err := gen.FormatNodeExtraWithFileSet(targetFile, fset, true)
	if err != nil {
		return nil, err
	}
	code = insertStructDoc(code, targetStruct, structDoc)
	for _, declDoc := range docInserts.decls {
		code = insertDeclDoc(code, declDoc, targetStruct)
	}
	for _, functionName := range sortedDocNames(docInserts.functions) {
		code = insertFunctionDoc(code, functionName, docInserts.functions[functionName])
	}
	for _, methodName := range sortedDocNames(docInserts.methods) {
		code = insertMethodDoc(code, targetStruct, methodName, docInserts.methods[methodName])
	}
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

func retargetDocLines(docLines []string, sourceName string, targetName string) []string {
	if len(docLines) == 0 || sourceName == "" || targetName == "" || sourceName == targetName {
		return docLines
	}
	retargeted := append([]string{}, docLines...)
	sourcePrefix := "// " + sourceName
	if suffix, ok := strings.CutPrefix(retargeted[0], sourcePrefix); ok {
		retargeted[0] = "// " + targetName + suffix
	}
	return retargeted
}

func insertStructDoc(code string, typeName string, docLines []string) string {
	if len(docLines) == 0 {
		return code
	}
	lines := strings.Split(code, "\n")
	typePrefix := "type " + typeName + " struct"
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), typePrefix) {
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

func insertFunctionDoc(code string, functionName string, docLines []string) string {
	if len(docLines) == 0 {
		return code
	}
	lines := strings.Split(code, "\n")
	funcPrefix := "func " + functionName + "("
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), funcPrefix) {
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

type sourceDocInserts struct {
	decls     []declDocInsert
	functions map[string][]string
	methods   map[string][]string
}

type declDocInsert struct {
	kind token.Token
	name string
	doc  []string
}

func newSourceDocInserts() sourceDocInserts {
	return sourceDocInserts{
		functions: make(map[string][]string),
		methods:   make(map[string][]string),
	}
}

func insertDeclDoc(code string, insertDoc declDocInsert, serviceStruct string) string {
	if len(insertDoc.doc) == 0 || len(insertDoc.name) == 0 || insertDoc.name == serviceStruct {
		return code
	}
	lines := strings.Split(code, "\n")
	prefix := strings.ToLower(insertDoc.kind.String()) + " " + insertDoc.name
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), prefix) {
			continue
		}
		if i > 0 && strings.TrimSpace(lines[i-1]) == insertDoc.doc[len(insertDoc.doc)-1] {
			return code
		}
		doc := append([]string{}, insertDoc.doc...)
		lines = append(lines[:i], append(doc, lines[i:]...)...)
		return strings.Join(lines, "\n")
	}
	return code
}

func mergeSourceServiceDecls(
	targetFile *ast.File,
	sourceFile *ast.File,
	sourceStruct string,
	targetStruct string,
	sourceComments ast.CommentMap,
) sourceDocInserts {
	docInserts := newSourceDocInserts()
	for _, decl := range sourceFile.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				continue
			}
			filtered := filterSourceSpecs(d, sourceStruct)
			if filtered != nil {
				docInserts.decls = append(docInserts.decls, declDocInsert{
					kind: d.Tok,
					name: firstGenDeclSpecName(filtered),
					doc:  commentGroupLines(d.Doc),
				})
				targetFile.Decls = append(targetFile.Decls, filtered)
			}
		case *ast.FuncDecl:
			if d.Recv != nil && receiverTypeName(d) == sourceStruct {
				if targetMethod := findMethod(targetFile, targetStruct, d.Name.Name); targetMethod != nil {
					sourceRecv := methodReceiverName(d)
					targetRecv := methodReceiverName(targetMethod)
					if sourceRecv != "" && targetRecv != "" && sourceRecv != targetRecv && d.Body != nil {
						renameIdent(d.Body, sourceRecv, targetRecv)
					}
					// The generated target shell owns method signatures. When a
					// source body is grafted onto that signature, source parameter
					// names like "data" must be retargeted to generated names like
					// "userroles" so the copied body still compiles.
					retargetMethodBodySignatureNames(d, targetMethod)
					docInserts.methods[d.Name.Name] = commentGroupLines(d.Doc)
					targetMethod.Doc = nil
					targetMethod.Body = d.Body
					appendSourceComments(targetFile, sourceComments, d.Body)
					continue
				}
				retargetReceiver(d, targetStruct)
				docInserts.methods[d.Name.Name] = commentGroupLines(d.Doc)
				d.Doc = nil
			} else {
				docInserts.functions[d.Name.Name] = commentGroupLines(d.Doc)
				d.Doc = nil
			}
			appendSourceComments(targetFile, sourceComments, d.Body)
			targetFile.Decls = append(targetFile.Decls, d)
		default:
			targetFile.Decls = append(targetFile.Decls, d)
		}
	}
	return docInserts
}

func appendSourceComments(targetFile *ast.File, sourceComments ast.CommentMap, node ast.Node) {
	if targetFile == nil || sourceComments == nil || node == nil {
		return
	}
	targetFile.Comments = append(targetFile.Comments, sourceComments.Filter(node).Comments()...)
}

func firstGenDeclSpecName(decl *ast.GenDecl) string {
	if decl == nil {
		return ""
	}
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			return s.Name.Name
		case *ast.ValueSpec:
			if len(s.Names) > 0 {
				return s.Names[0].Name
			}
		}
	}
	return ""
}

func sortedDocNames(methodDocs map[string][]string) []string {
	names := make([]string, 0, len(methodDocs))
	for name := range methodDocs {
		if len(methodDocs[name]) == 0 {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func retargetReceiver(fn *ast.FuncDecl, targetStruct string) {
	if fn == nil || fn.Recv == nil || len(fn.Recv.List) == 0 {
		return
	}
	recv := fn.Recv.List[0]
	switch typ := recv.Type.(type) {
	case *ast.StarExpr:
		if ident, ok := typ.X.(*ast.Ident); ok {
			ident.Name = targetStruct
		}
	case *ast.Ident:
		typ.Name = targetStruct
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

func serviceStructDoc(file *ast.File, structName string) *ast.CommentGroup {
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name == nil || typeSpec.Name.Name != structName || !isServiceTypeSpec(typeSpec) {
				continue
			}
			if typeSpec.Doc != nil {
				return typeSpec.Doc
			}
			return genDecl.Doc
		}
	}
	return nil
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

func retargetMethodBodySignatureNames(sourceMethod *ast.FuncDecl, targetMethod *ast.FuncDecl) {
	if sourceMethod == nil || targetMethod == nil || sourceMethod.Body == nil || sourceMethod.Type == nil || targetMethod.Type == nil {
		return
	}
	renameFieldListIdents(sourceMethod.Body, sourceMethod.Type.Params, targetMethod.Type.Params)
	renameFieldListIdents(sourceMethod.Body, sourceMethod.Type.Results, targetMethod.Type.Results)
}

func renameFieldListIdents(body ast.Node, sourceFields *ast.FieldList, targetFields *ast.FieldList) {
	sourceNames := fieldListNames(sourceFields)
	targetNames := fieldListNames(targetFields)
	for idx := 0; idx < len(sourceNames) && idx < len(targetNames); idx++ {
		sourceName := sourceNames[idx]
		targetName := targetNames[idx]
		if sourceName == "" || targetName == "" || sourceName == targetName {
			continue
		}
		renameIdent(body, sourceName, targetName)
	}
}

func fieldListNames(fields *ast.FieldList) []string {
	if fields == nil {
		return nil
	}
	names := make([]string, 0, len(fields.List))
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			names = append(names, "")
			continue
		}
		for _, name := range field.Names {
			if name == nil {
				names = append(names, "")
				continue
			}
			names = append(names, name.Name)
		}
	}
	return names
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
