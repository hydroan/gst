package gen

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/constants"
)

// applyServiceRoleName renames the service struct type and all associated receiver
// types and variable names to match the action's RoleName.
// This is needed when Filename is set, causing the struct name to differ from the
// default Phase-based name (e.g., "Creator" → "Upload").
//
// It performs three updates:
//  1. Renames the struct type declaration (e.g., type Creator struct → type Upload struct)
//  2. Renames receiver types in all methods (e.g., func (a *Creator) → func (u *Upload))
//  3. Renames receiver variable names and all references in method bodies
//     (e.g., "a" → "u", a.WithContext → u.WithContext)
func applyServiceRoleName(file *ast.File, action *dsl.Action) bool {
	if file == nil || action == nil || len(action.Filename) == 0 {
		return false
	}

	newRoleName := action.RoleName()
	if len(newRoleName) == 0 {
		return false
	}
	newRecvVar := strings.ToLower(newRoleName[:1])

	// Find the current service struct name
	oldRoleName := findServiceTypeName(file)
	if len(oldRoleName) == 0 {
		return false
	}

	needRenameStruct := oldRoleName != newRoleName

	var changed bool

	// 1. Rename the struct type declaration (only if names differ)
	if needRenameStruct {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}
			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || !isServiceType(typeSpec) {
					continue
				}
				if typeSpec.Name.Name != newRoleName {
					typeSpec.Name = ast.NewIdent(newRoleName)
					changed = true
				}
			}
		}
	}

	// 2 & 3. Update receiver type and variable name in all methods.
	// The receiver type is renamed when the struct name changed.
	// The receiver variable name is always checked and updated to match newRecvVar,
	// even when the struct name already matches (e.g., struct is "Upload" but receiver is still "a").
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl == nil || funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
			continue
		}
		recv := funcDecl.Recv.List[0]

		starExpr, ok := recv.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		ident, ok := starExpr.X.(*ast.Ident)
		if !ok {
			continue
		}

		// Only process methods whose receiver type is the old or new role name
		if ident.Name != oldRoleName && ident.Name != newRoleName {
			continue
		}

		// Update receiver type if struct was renamed
		if needRenameStruct && ident.Name == oldRoleName {
			ident.Name = newRoleName
			changed = true
		}

		// Update receiver variable name to match the new role name
		if len(recv.Names) > 0 && recv.Names[0].Name != newRecvVar {
			oldName := recv.Names[0].Name
			recv.Names[0] = ast.NewIdent(newRecvVar)
			changed = true

			// Update all references to the old receiver variable in the method body
			if funcDecl.Body != nil {
				renameIdent(funcDecl.Body, oldName, newRecvVar)
			}
		}
	}

	return changed
}

// findServiceTypeName finds the name of the service struct type in the file.
// It looks for a struct that embeds service.Base[...] and returns its name.
func findServiceTypeName(file *ast.File) string {
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || !isServiceType(typeSpec) {
				continue
			}
			return typeSpec.Name.Name
		}
	}
	return ""
}

// renameIdent walks an AST node and renames all *ast.Ident nodes
// matching oldName to newName.
func renameIdent(node ast.Node, oldName, newName string) {
	ast.Inspect(node, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok && ident.Name == oldName {
			ident.Name = newName
		}
		return true
	})
}

// ApplyServiceFile will apply the dsl.Action to the ast.File.
// It will modify the struct type and struct methods if Payload
// or Result is changed, and returns true.
// Otherwise returns false.
// The servicePkgName parameter specifies the expected package name for the service file.
// This should match the package name used in service registration to maintain consistency.
func ApplyServiceFile(file *ast.File, action *dsl.Action, servicePkgName string) bool {
	return applyServiceFile(file, action, servicePkgName, "")
}

func applyServiceFile(file *ast.File, action *dsl.Action, servicePkgName, correctModelName string) bool {
	if file == nil || action == nil {
		return false
	}

	var changed bool

	// Apply package name correction
	if len(servicePkgName) > 0 && file.Name != nil && file.Name.Name != servicePkgName {
		file.Name.Name = servicePkgName
		changed = true
	}

	// Rename service struct type and receiver names when Filename is set
	if applyServiceRoleName(file, action) {
		changed = true
	}

	for _, decl := range file.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
			for _, spec := range genDecl.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok {
					if isServiceType(typeSpec) {
						if applyServiceType(typeSpec, action, correctModelName) {
							changed = true
						}
					}
				}
			}
		}
	}

	for _, decl := range file.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok && funcDecl != nil {
			if isServiceMethod1(funcDecl) {
				if applyServiceMethod1(funcDecl, action) {
					changed = true
				}
			}
			if isServiceMethod2(funcDecl) {
				if applyServiceMethod2(funcDecl, action) {
					changed = true
				}
			}
			if isServiceMethod3(funcDecl) {
				if applyServiceMethod3(funcDecl, action) {
					changed = true
				}
			}
			if isServiceMethod4(funcDecl) {
				if applyServiceMethod4(funcDecl, action) {
					changed = true
				}
			}
		}
	}

	// Keep the gst model import in sync with the request type: a rewritten
	// *model.Empty request needs the import, while a business request type
	// must not leave it behind unused.
	if isEmptyPayload(action.Payload) {
		if ensureEmptyReqImportSpec(file, serviceModelPackageName(file)) {
			changed = true
		}
	} else if pruneGstModelImportSpec(file) {
		changed = true
	}

	return changed
}

// applyServiceMethod1 updates functions that match the ServiceMethod1 shape according to DSL.
// Currently ServiceMethod1 does not rely on DSL configuration; keep empty for future extension.
func applyServiceMethod1(fn *ast.FuncDecl, action *dsl.Action) bool { return false }

// applyServiceMethod2 updates functions that match the ServiceMethod2 shape according to DSL.
// Currently ServiceMethod2 does not rely on DSL configuration; keep empty for future extension.
func applyServiceMethod2(fn *ast.FuncDecl, action *dsl.Action) bool { return false }

// applyServiceMethod3 updates functions that match the ServiceMethod3 shape according to DSL.
// Currently ServiceMethod3 does not rely on DSL configuration; keep empty for future extension.
func applyServiceMethod3(fn *ast.FuncDecl, action *dsl.Action) bool { return false }

// applyServiceMethod4 updates functions that match the ServiceMethod4 shape based on the DSL.
// It only updates the shape of *ast.FuncDecl (param/return types) and never touches the method body logic.
// Shape: func (r *recv) Method(ctx *types.ServiceContext, req *<pkg>.<Req>) (*<pkg>.<Rsp>, error)
//
//	func (r *recv) Method(ctx *types.ServiceContext, req <pkg>.<Req>) (<pkg>.<Rsp>, error)
//
// isServiceMethod4 only recognizes the parameter/result shape, not the function name, so a
// hand-written helper that happens to match the same shape as the real action method must not
// be rewritten. Incident: a Patch action's Payload/Result were changed to
// RecordPatchReq/RecordPatchRsp and gg gen was re-run; Patcher.validate, a plain
// validation helper with the same (ctx *types.ServiceContext, req *pkg.Req) (*pkg.X, error)
// shape as Patch, was mistaken for the action method and had its return type rewritten to
// *sample.RecordPatchRsp, corrupting the function body and breaking the build. Requiring
// fn.Name to equal action.Phase.MethodName() (e.g. "Patch") ensures only the actual action
// method for the current DSL phase is ever rewritten.
func applyServiceMethod4(fn *ast.FuncDecl, action *dsl.Action) bool {
	if fn == nil || action == nil || fn.Name == nil {
		return false
	}

	if !isServiceMethod4(fn) {
		return false
	}

	if fn.Name.Name != action.Phase.MethodName() {
		return false
	}

	var changed bool

	// The business model package qualifier is derived from the first result
	// type: Result never carries the dsl.PayloadEmpty sentinel, so it always
	// references the business model package.
	modelPkg := ""
	if fn.Type != nil && fn.Type.Results != nil && len(fn.Type.Results.List) >= 1 {
		modelPkg = selectorPackageName(fn.Type.Results.List[0].Type)
	}

	// Update the second parameter type based on action.Payload. The
	// dsl.PayloadEmpty sentinel switches the qualifier to the gst model
	// package; a business payload switches it back.
	if fn.Type != nil && fn.Type.Params != nil && len(fn.Type.Params.List) >= 2 && action.Payload != "" {
		param := fn.Type.Params.List[1]
		targetPkg, targetType := payloadTypeTarget(action.Payload, modelPkg)
		if expr, c := applyTypeRef(param.Type, targetPkg, targetType); c {
			param.Type = expr
			changed = true
		}
	}

	// Update the first result type based on action.Result
	if fn.Type != nil && fn.Type.Results != nil && len(fn.Type.Results.List) >= 1 && action.Result != "" {
		res := fn.Type.Results.List[0]
		if expr, c := applyTypeRef(res.Type, modelPkg, action.Result); c {
			res.Type = expr
			changed = true
		}
	}

	return changed
}

// applyTypeRef rewrites a *pkg.Type or pkg.Type expression to reference
// targetPkg and actionType. A leading '*' in actionType selects the pointer
// form. When targetPkg is empty the current package qualifier is kept.
// It returns the possibly replaced expression and whether anything changed.
func applyTypeRef(expr ast.Expr, targetPkg, actionType string) (ast.Expr, bool) {
	if actionType == "" {
		return expr, false
	}
	pointer := strings.HasPrefix(actionType, "*")
	typeName := strings.TrimPrefix(actionType, "*")

	var sel *ast.SelectorExpr
	switch t := expr.(type) {
	case *ast.StarExpr:
		s, ok := t.X.(*ast.SelectorExpr)
		if !ok {
			return expr, false
		}
		sel = s
	case *ast.SelectorExpr:
		sel = t
	default:
		return expr, false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return expr, false
	}

	var changed bool
	if targetPkg != "" && pkgIdent.Name != targetPkg {
		newIdent := ast.NewIdent(targetPkg)
		newIdent.NamePos = pkgIdent.NamePos
		sel.X = newIdent
		changed = true
	}
	if sel.Sel == nil || sel.Sel.Name != typeName {
		newIdent := ast.NewIdent(typeName)
		if sel.Sel != nil {
			newIdent.NamePos = sel.Sel.NamePos
		}
		sel.Sel = newIdent
		changed = true
	}

	_, isPointer := expr.(*ast.StarExpr)
	if isPointer == pointer {
		return expr, changed
	}
	if pointer {
		// Position the * just before the selector
		return &ast.StarExpr{Star: sel.Pos() - 1, X: sel}, true
	}
	return sel, true
}

// applyServiceType updates a service struct type to match the generated service generics.
// It transforms: type user struct { service.Base[*model.User, *model.User, *model.User] }
// into:         type user struct { service.Base[*model.User, *model.UserReq, *model.UserRsp] }
// or:           type user struct { service.Base[*model.User, model.UserReq, model.UserRsp] }
// depending on whether action.Payload/Result starts with '*'. When correctModelName
// is provided, it also corrects the first generic parameter to the current model.
func applyServiceType(spec *ast.TypeSpec, action *dsl.Action, correctModelName ...string) bool {
	if spec == nil || action == nil {
		return false
	}
	structType, ok := spec.Type.(*ast.StructType)
	if !ok || structType.Fields == nil {
		return false
	}

	var changed bool

	for _, field := range structType.Fields.List {
		if len(field.Names) == 0 { // Embedded field
			indexListExpr, ok := field.Type.(*ast.IndexListExpr)
			if !ok {
				continue
			}
			// ensure service.Base
			if sel, ok := indexListExpr.X.(*ast.SelectorExpr); ok {
				if pkgIdent, ok := sel.X.(*ast.Ident); ok && pkgIdent.Name == "service" && sel.Sel.Name == "Base" {
					if len(indexListExpr.Indices) == 3 {
						if len(correctModelName) > 0 && correctModelName[0] != "" {
							if changed1 := applyServiceTypeParam(indexListExpr, 0, "", "*"+correctModelName[0]); changed1 {
								changed = true
							}
						}
						// The first generic parameter always references the
						// business model package, so it provides the target
						// qualifier for the payload and result parameters.
						modelPkg := selectorPackageName(indexListExpr.Indices[0])
						// Handle second parameter (Payload)
						if action.Payload != "" {
							targetPkg, targetType := payloadTypeTarget(action.Payload, modelPkg)
							if changed2 := applyServiceTypeParam(indexListExpr, 1, targetPkg, targetType); changed2 {
								changed = true
							}
						}
						// Handle third parameter (Result)
						if action.Result != "" {
							if changed3 := applyServiceTypeParam(indexListExpr, 2, modelPkg, action.Result); changed3 {
								changed = true
							}
						}
					}
				}
			}
		}
	}

	return changed
}

// applyServiceTypeParam updates a specific type parameter in service.Base[T1, T2, T3]
// to reference targetPkg and actionType. A leading '*' in actionType selects
// the pointer form; an empty targetPkg keeps the current package qualifier.
func applyServiceTypeParam(indexListExpr *ast.IndexListExpr, paramIndex int, targetPkg, actionType string) bool {
	if paramIndex >= len(indexListExpr.Indices) || actionType == "" {
		return false
	}

	expr, changed := applyTypeRef(indexListExpr.Indices[paramIndex], targetPkg, actionType)
	if changed {
		indexListExpr.Indices[paramIndex] = expr
	}
	return changed
}

// forceCanonicalServiceStruct forces the action's service struct declaration
// back to the generated canonical form: a struct whose body is exactly the
// direct service.Base[...] embedding. The struct declaration is generated
// code that registration and logger injection depend on, so hand edits to it
// are corrected instead of preserved: extra fields are discarded together
// with interior comments (embedding another service is forbidden, and the
// framework injects the logger only through the direct service.Base
// embedding), and a deleted struct is regenerated so gg gen always converges
// on a registrable service struct. It reports whether the file was modified.
func forceCanonicalServiceStruct(file *ast.File, action *dsl.Action, modelInfo *ModelInfo) bool {
	if file == nil || action == nil || modelInfo == nil {
		return false
	}
	roleName := action.RoleName()
	if len(roleName) == 0 {
		return false
	}

	spec := findStructTypeSpec(file, roleName)
	if spec == nil {
		// A well-formed service struct under another name is out of rewrite
		// scope: with Filename set it belongs to the applyServiceRoleName
		// rename path, and module-copied services legitimately use their own
		// struct names (they register manually instead of through generated
		// registration code).
		if len(findServiceTypeName(file)) > 0 {
			return false
		}
		file.Decls = append(file.Decls, types(modelInfo.ModelPkgName, modelInfo.ModelName, action.Payload, action.Result, action.Phase, roleName, false))
		ensureServiceImportSpec(file)
		ensureModelImportSpec(file, modelInfo)
		return true
	}

	structType, ok := spec.Type.(*ast.StructType)
	if !ok {
		return false
	}
	if isCanonicalServiceStructBody(structType) {
		return false
	}

	baseField := generatedServiceBaseField(modelInfo, action, roleName)
	if baseField == nil {
		return false
	}
	removeStructInteriorComments(file, structType)
	if structType.Fields == nil {
		structType.Fields = &ast.FieldList{}
	}
	structType.Fields.List = []*ast.Field{baseField}
	ensureServiceImportSpec(file)
	ensureModelImportSpec(file, modelInfo)
	return true
}

// isCanonicalServiceStructBody reports whether the struct body is exactly the
// generated form: a single embedded service.Base[T1, T2, T3] field. Stale
// type parameters still count as canonical; applyServiceType syncs them
// separately without rewriting the body.
func isCanonicalServiceStructBody(structType *ast.StructType) bool {
	if structType.Fields == nil || len(structType.Fields.List) != 1 {
		return false
	}
	field := structType.Fields.List[0]
	return len(field.Names) == 0 && is_service_base_with_three_type_params(field.Type)
}

// removeStructInteriorComments drops every comment group positioned inside
// the struct braces: a force-rewritten body takes its comments with it, and a
// stale comment would otherwise interleave with the position-less replacement
// field when printing.
func removeStructInteriorComments(file *ast.File, structType *ast.StructType) {
	if structType.Fields == nil || !structType.Fields.Opening.IsValid() || !structType.Fields.Closing.IsValid() {
		return
	}
	kept := file.Comments[:0]
	for _, group := range file.Comments {
		if group.Pos() > structType.Fields.Opening && group.End() < structType.Fields.Closing {
			continue
		}
		kept = append(kept, group)
	}
	file.Comments = kept
}

// generatedServiceBaseField builds the service.Base[...] embedded field
// exactly as generated service code declares it for the action.
func generatedServiceBaseField(modelInfo *ModelInfo, action *dsl.Action, roleName string) *ast.Field {
	decl := types(modelInfo.ModelPkgName, modelInfo.ModelName, action.Payload, action.Result, action.Phase, roleName, false)
	typeSpec, ok := decl.Specs[0].(*ast.TypeSpec)
	if !ok {
		return nil
	}
	structType, ok := typeSpec.Type.(*ast.StructType)
	if !ok {
		return nil
	}
	return structType.Fields.List[0]
}

// findStructTypeSpec returns the struct type spec declared with the given
// name, or nil when the file declares no such struct.
func findStructTypeSpec(file *ast.File, name string) *ast.TypeSpec {
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name == nil || typeSpec.Name.Name != name {
				continue
			}
			if _, ok := typeSpec.Type.(*ast.StructType); ok {
				return typeSpec
			}
		}
	}
	return nil
}

// ensureServiceImportSpec inserts the gst service import so a restored
// service.Base embedding resolves.
func ensureServiceImportSpec(file *ast.File) {
	if file == nil || findImportSpec(file, constants.ImportPathService) != nil {
		return
	}
	insertImportSpec(file, &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: fmt.Sprintf("%q", constants.ImportPathService),
		},
	})
}

// ensureModelImportSpec inserts the business model import a restored
// service.Base embedding references, aliased when the package name differs
// from the last import path segment.
func ensureModelImportSpec(file *ast.File, modelInfo *ModelInfo) {
	importPath := filepath.Join(modelInfo.ModulePath, modelInfo.ModelFileDir)
	if file == nil || findImportSpec(file, importPath) != nil {
		return
	}

	spec := &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: fmt.Sprintf("%q", importPath),
		},
	}
	if fields := strings.Split(importPath, "/"); len(fields) > 0 && fields[len(fields)-1] != modelInfo.ModelPkgName {
		spec.Name = ast.NewIdent(modelInfo.ModelPkgName)
	}
	insertImportSpec(file, spec)
}

// insertImportSpec appends the import spec to the first import declaration,
// creating one at the top of the file when none exists.
func insertImportSpec(file *ast.File, spec *ast.ImportSpec) {
	for _, decl := range file.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.IMPORT {
			genDecl.Specs = append(genDecl.Specs, spec)
			return
		}
	}
	file.Decls = append([]ast.Decl{&ast.GenDecl{Tok: token.IMPORT, Specs: []ast.Spec{spec}}}, file.Decls...)
}

// ApplyServiceFileWithModelSync extends ApplyServiceFile to handle import path and package name updates.
// It will update import statements and package references when model packages are renamed.
//
// Design Philosophy:
// This function uses AST manipulation instead of regenerating files to preserve user's code formatting
// and custom modifications. Different developers have different code formatting preferences, and we
// should not force our formatting on their existing code. We only update the necessary parts
// (imports and type references) while keeping everything else intact.
//
// Example transformation when "model/oldpkg" is renamed to "model/newpkg":
// - Import statement: "myproject/model/oldpkg" -> "myproject/model/newpkg"
// - Type references: oldpkg.User -> newpkg.User, oldpkg.UserReq -> newpkg.UserReq, oldpkg.UserRsp -> newpkg.UserRsp
//
// Parameters:
// - file: The AST file to process
// - action: The DSL action configuration
// - servicePkgName: The expected service package name
// - modelInfo: The correct model generation context
//
// Returns true if any changes were made to the file.
func ApplyServiceFileWithModelSync(file *ast.File, action *dsl.Action, servicePkgName string, modelInfo *ModelInfo) bool {
	if file == nil || action == nil {
		return false
	}

	// Force the service struct body back to its canonical form before
	// anything else: the remaining apply steps only recognize service structs
	// through the direct service.Base embedding.
	changed := forceCanonicalServiceStruct(file, action, modelInfo)

	// First apply the original ApplyServiceFile logic
	correctModelName := ""
	if modelInfo != nil {
		correctModelName = modelInfo.ModelName
	}
	if applyServiceFile(file, action, servicePkgName, correctModelName) {
		changed = true
	}
	if modelInfo == nil || modelInfo.ModulePath == "" || modelInfo.ModelFileDir == "" || modelInfo.ModelPkgName == "" {
		return changed
	}

	// Build a map of old package names to new package names
	// by comparing current imports with the correct model import path
	correctModelImportPath := filepath.Join(modelInfo.ModulePath, modelInfo.ModelFileDir)
	correctModelPkgName := modelInfo.ModelPkgName
	importMapping := buildModelImportMapping(file, correctModelPkgName)

	if len(importMapping) == 0 {
		// No import changes needed
		return changed
	}

	// Update import statements
	if syncModelImports(file, correctModelImportPath, importMapping) {
		changed = true
	}

	// Update package references in the code (e.g., identity.Login -> iam.Login)
	if syncModelPackageReferences(file, importMapping) {
		changed = true
	}

	return changed
}

// buildModelImportMapping builds a mapping from the currently-used service model package name
// to correctModelPkgName (only when they differ), so we only rewrite the main model import/reference
// instead of accidentally rewriting other sibling model packages used in user code.
func buildModelImportMapping(file *ast.File, correctModelPkgName string) map[string]string {
	currentModelPkgName := serviceModelPackageName(file)
	if currentModelPkgName == "" || currentModelPkgName == correctModelPkgName {
		return nil
	}
	return map[string]string{currentModelPkgName: correctModelPkgName}
}

func serviceModelPackageName(file *ast.File) string {
	if file == nil {
		return ""
	}

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || !isServiceType(typeSpec) {
				continue
			}

			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok || structType.Fields == nil {
				continue
			}

			for _, field := range structType.Fields.List {
				if len(field.Names) != 0 {
					continue
				}
				indexListExpr, ok := field.Type.(*ast.IndexListExpr)
				if !ok {
					continue
				}
				sel, ok := indexListExpr.X.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				pkgIdent, ok := sel.X.(*ast.Ident)
				if !ok || pkgIdent.Name != "service" || sel.Sel == nil || sel.Sel.Name != "Base" {
					continue
				}
				if len(indexListExpr.Indices) != 3 {
					continue
				}

				return selectorPackageName(indexListExpr.Indices[0])
			}
		}
	}

	return ""
}

func selectorPackageName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return selectorPackageName(t.X)
	case *ast.SelectorExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}

// syncModelImports updates import statements based on the import mapping.
// This function precisely modifies only the import path values in the AST,
// preserving all other aspects of the code including formatting, comments, and structure.
// Returns true if any imports were updated.
func syncModelImports(file *ast.File, correctModelImportPath string, mapping map[string]string) bool {
	changed := false

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.IMPORT {
			continue
		}

		for _, spec := range genDecl.Specs {
			importSpec, ok := spec.(*ast.ImportSpec)
			if !ok || importSpec.Path == nil {
				continue
			}

			// Get the import path without quotes
			importPath := strings.Trim(importSpec.Path.Value, `"`)

			// Check if we need to update this import
			shouldUpdate := false

			// Check if the current import path's base name is in the mapping
			baseName := filepath.Base(importPath)
			if _, found := mapping[baseName]; found {
				shouldUpdate = true
			}

			// Also check if there's an explicit alias that's in the mapping
			if importSpec.Name != nil && importSpec.Name.Name != "" && importSpec.Name.Name != "_" {
				if _, found := mapping[importSpec.Name.Name]; found {
					shouldUpdate = true
				}
			}

			if shouldUpdate {
				// Update the import path to the correct one
				importSpec.Path.Value = `"` + correctModelImportPath + `"`
				changed = true

				// Remove the explicit alias since the import path now has the correct name
				importSpec.Name = nil
			}
		}
	}

	return changed
}

// syncModelPackageReferences updates package references in the code.
// This function walks the AST and updates only the package identifier names in selector expressions,
// preserving all other code structure, formatting, and logic.
//
// Example: oldpkg.User -> newpkg.User, oldpkg.UserReq -> newpkg.UserReq
// Returns true if any references were updated.
func syncModelPackageReferences(file *ast.File, mapping map[string]string) bool {
	changed := false

	// Walk the AST and update all SelectorExpr nodes where X is an Ident
	// that matches one of the old package names
	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				// Check if this identifier matches one of the old package names
				if newPkg, found := mapping[ident.Name]; found {
					ident.Name = newPkg
					changed = true
				}
			}
		}
		return true
	})

	return changed
}
