package ggmodule

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/codegen/gen"
)

type moduleServiceMergeInput struct {
	SourcePath string
	Source     []byte
	TargetPath string
	Target     []byte
	Rewrite    moduleCopyRewriteConfig
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

	selectorNames := rewriteModuleCopyFile(sourceFile, input.Rewrite, true)
	rewriteSelectorPackages(sourceFile, selectorNames)

	sourceStructs := serviceStructNames(sourceFile)
	if len(sourceStructs) == 0 {
		return nil, fmt.Errorf("source action service file %s has no service struct", input.SourcePath)
	}
	targetStruct := findServiceStructName(targetFile)
	if targetStruct == "" {
		return nil, fmt.Errorf("target action service file %s has no service struct", input.TargetPath)
	}
	if uniqueErr := requireUniqueSourceMethods(sourceFile, sourceStructs, input.SourcePath); uniqueErr != nil {
		return nil, uniqueErr
	}

	sourceStruct := sourceStructs[0]
	structDoc := retargetDocLines(commentGroupLines(serviceStructDoc(sourceFile, sourceStruct)), sourceStruct, targetStruct)
	sourceComments := ast.NewCommentMap(fset, sourceFile, sourceFile.Comments)

	mergeImports(targetFile, sourceFile.Imports)
	docInserts := mergeSourceServiceDecls(targetFile, sourceFile, sourceStructs, targetStruct, sourceComments)

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
	for _, key := range sortedMethodDocKeys(docInserts.methods) {
		code = insertMethodDoc(code, key.receiver, key.name, docInserts.methods[key])
	}
	return []byte(code), nil
}

// requireUniqueSourceMethods rejects a source file that declares the same method
// name on more than one service struct. Every source service struct is merged
// onto the single generated target struct, so two same-named methods would
// either overwrite each other or emit a duplicate declaration that stops the
// copied project from compiling. Failing here keeps the copy a preflight error
// instead of a broken project.
func requireUniqueSourceMethods(file *ast.File, sourceStructs []string, sourcePath string) error {
	sourceStructSet := make(map[string]bool, len(sourceStructs))
	for _, sourceStruct := range sourceStructs {
		sourceStructSet[sourceStruct] = true
	}
	owners := make(map[string]string)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name == nil {
			continue
		}
		receiver := receiverTypeName(fn)
		if !sourceStructSet[receiver] {
			continue
		}
		if owner, seen := owners[fn.Name.Name]; seen {
			return fmt.Errorf("source action service file %s declares method %s on both %s and %s; module copy merges every service struct onto one target struct", sourcePath, fn.Name.Name, owner, receiver)
		}
		owners[fn.Name.Name] = receiver
	}
	return nil
}

func mergeSourceServiceDecls(
	targetFile *ast.File,
	sourceFile *ast.File,
	sourceStructs []string,
	targetStruct string,
	sourceComments ast.CommentMap,
) sourceDocInserts {
	docInserts := newSourceDocInserts()
	sourceStructSet := make(map[string]bool, len(sourceStructs))
	for _, sourceStruct := range sourceStructs {
		sourceStructSet[sourceStruct] = true
	}
	for _, decl := range sourceFile.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				continue
			}
			filtered := filterSourceSpecs(d, sourceStructSet)
			if filtered != nil {
				docInserts.decls = append(docInserts.decls, declDocInsert{
					kind: d.Tok,
					name: firstGenDeclSpecName(filtered),
					doc:  commentGroupLines(d.Doc),
				})
				targetFile.Decls = append(targetFile.Decls, filtered)
			}
		case *ast.FuncDecl:
			switch {
			case d.Recv != nil && sourceStructSet[receiverTypeName(d)]:
				if targetMethod := findMethod(targetFile, targetStruct, d.Name.Name); targetMethod != nil {
					sourceRecv := methodReceiverName(d)
					targetRecv := methodReceiverName(targetMethod)
					if sourceRecv != "" && targetRecv != "" && sourceRecv != targetRecv && d.Body != nil {
						renameIdent(d.Body, sourceRecv, targetRecv)
					}
					// The generated target shell owns method signatures. When a
					// source body is grafted onto that signature, the source
					// parameter and result names must be retargeted to the
					// generated ones so the copied body still compiles.
					retargetMethodBodySignatureNames(d, targetMethod)
					docInserts.methods[methodDocKey{receiver: targetStruct, name: d.Name.Name}] = commentGroupLines(d.Doc)
					targetMethod.Doc = nil
					targetMethod.Body = d.Body
					appendSourceComments(targetFile, sourceComments, d.Body)
					continue
				}
				retargetReceiver(d, targetStruct)
				docInserts.methods[methodDocKey{receiver: targetStruct, name: d.Name.Name}] = commentGroupLines(d.Doc)
				d.Doc = nil
			case d.Recv != nil:
				// A method on an ordinary source type keeps its own receiver, so
				// its doc must be re-inserted before that receiver's method
				// rather than looked up as a plain function.
				docInserts.methods[methodDocKey{receiver: receiverTypeName(d), name: d.Name.Name}] = commentGroupLines(d.Doc)
				d.Doc = nil
			default:
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

func filterSourceSpecs(decl *ast.GenDecl, sourceStructSet map[string]bool) *ast.GenDecl {
	if decl.Tok != token.TYPE {
		return decl
	}
	specs := make([]ast.Spec, 0, len(decl.Specs))
	for _, spec := range decl.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if ok && sourceStructSet[typeSpec.Name.Name] && isServiceTypeSpec(typeSpec) {
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

func receiverTypeName(fn *ast.FuncDecl) string {
	if fn == nil || fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	switch typ := fn.Recv.List[0].Type.(type) {
	case *ast.Ident:
		return typ.Name
	case *ast.StarExpr:
		if ident, ok := typ.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}

func methodReceiverName(fn *ast.FuncDecl) string {
	if fn == nil || fn.Recv == nil || len(fn.Recv.List) == 0 || len(fn.Recv.List[0].Names) == 0 {
		return ""
	}
	return fn.Recv.List[0].Names[0].Name
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
	targetStructSet := map[string]bool{targetStruct: true}
	for _, decl := range generatedFile.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				continue
			}
			filtered := filterSourceSpecs(d, targetStructSet)
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

func serviceStructNames(file *ast.File) []string {
	names := make([]string, 0)
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
			names = append(names, typeSpec.Name.Name)
		}
	}
	return names
}

func findServiceStructName(file *ast.File) string {
	names := serviceStructNames(file)
	if len(names) > 0 {
		return names[0]
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
	return len(serviceStructNames(file)), nil
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
