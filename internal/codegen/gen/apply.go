package gen

import (
	"fmt"
	"go/ast"
	"go/token"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/constants"
)

// applyServiceRoleName renames the service struct type and all associated receiver
// types and variable names to match the action's RoleName.
// This is needed when Filename is set, causing the struct name to differ from the
// default Phase-based name (e.g., "Creator" → "Archive").
//
// It performs three updates:
//  1. Renames the struct type declaration (e.g., type Creator struct → type Archive struct)
//  2. Renames receiver types in all methods (e.g., func (c *Creator) → func (a *Archive))
//  3. Renames receiver variable names and all references in method bodies
//     (e.g., "c" → "a", c.WithContext → a.WithContext)
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
	// even when the struct name already matches (e.g., struct is "Archive" but receiver is still "r").
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

// ApplyServiceFile brings an existing service file in line with action: the
// package clause with servicePkgName, the service struct and its receivers
// with the role name of an action declaring a Filename, the type parameters of
// the service.Base embedding and the request and result types of the action
// method with the action's Payload and Result, and the gst model import with
// whether a model.Empty request or result needs it. For example, once the
// Create action declares Payload[*UserReq]() and Result[*UserRsp](),
//
//	service.Base[*model.User, *model.User, *model.User]
//	func (u *Creator) Create(ctx *gst.ServiceContext, req *model.User) (rsp *model.User, err error)
//
// becomes
//
//	service.Base[*model.User, *model.UserReq, *model.UserRsp]
//	func (u *Creator) Create(ctx *gst.ServiceContext, req *model.UserReq) (rsp *model.UserRsp, err error)
//
// Method bodies are left alone. The servicePkgName parameter specifies the
// expected package name for the service file. This should match the package
// name used in service registration to maintain consistency. It reports
// whether it changed anything.
func ApplyServiceFile(file *ast.File, action *dsl.Action, servicePkgName string) bool {
	return applyServiceFile(file, action, servicePkgName, "")
}

// applyServiceFile is ApplyServiceFile that also points the first type
// parameter of the service.Base embedding at correctModelName, the current
// model, when it is not empty: service.Base[*model.Account, ...] becomes
// service.Base[*model.User, ...] for User.
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
				if applyServiceMethod4(funcDecl, action, serviceModelPackageName(file)) {
					changed = true
				}
			}
		}
	}

	// Keep the gst model import in sync with the action types: a rewritten
	// *model.Empty request or result needs the import, while pure business
	// action types must not leave it behind unused.
	if isEmptyPayload(action.Payload) || isEmptyPayload(action.Result) {
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
// Shape: func (r *recv) Method(ctx *gst.ServiceContext, req *<pkg>.<Req>) (*<pkg>.<Rsp>, error)
//
//	func (r *recv) Method(ctx *gst.ServiceContext, req <pkg>.<Req>) (<pkg>.<Rsp>, error)
//
// isServiceMethod4 only recognizes the parameter/result shape, not the function name, so a
// hand-written helper that happens to match the same shape as the real action method must not
// be rewritten. Incident: a Patch action's Payload/Result were changed to
// RecordPatchReq/RecordPatchRsp and gg gen was re-run; Patcher.validate, a plain
// validation helper with the same (ctx *gst.ServiceContext, req *pkg.Req) (*pkg.X, error)
// shape as Patch, was mistaken for the action method and had its return type rewritten to
// *sample.RecordPatchRsp, corrupting the function body and breaking the build. Requiring
// fn.Name to equal action.Phase.MethodName() (e.g. "Patch") ensures only the actual action
// method for the current DSL phase is ever rewritten.
func applyServiceMethod4(fn *ast.FuncDecl, action *dsl.Action, modelPkg string) bool {
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

	// Update the first result type based on action.Result, resolving the
	// dsl.PayloadEmpty sentinel the same way as the request side.
	if fn.Type != nil && fn.Type.Results != nil && len(fn.Type.Results.List) >= 1 && action.Result != "" {
		res := fn.Type.Results.List[0]
		targetPkg, targetType := payloadTypeTarget(action.Result, modelPkg)
		if expr, c := applyTypeRef(res.Type, targetPkg, targetType); c {
			res.Type = expr
			changed = true
		}
	}

	return changed
}

// applyTypeRef rewrites a *pkg.Type or pkg.Type expression to reference
// targetPkg and actionType, transcribing the declared form: a leading '*' in
// actionType selects the pointer form and a bare name selects the value form
// (the form itself is enforced by gg checks). When targetPkg is empty the
// current package qualifier is kept. It returns the possibly replaced
// expression and whether anything changed. For example, with targetPkg
// sample it rewrites *model.User to sample.UserRsp for the action type
// UserRsp.
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
// It transforms: type Creator struct { service.Base[*model.User, *model.User, *model.User] }
// into:          type Creator struct { service.Base[*model.User, *model.UserReq, *model.UserRsp] }
// transcribing the declared form of the action's Payload/Result. When
// correctModelName is provided, it also corrects the first generic parameter
// to the current model.
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
						// Handle third parameter (Result), resolving the
						// dsl.PayloadEmpty sentinel the same way as Payload.
						if action.Result != "" {
							targetPkg, targetType := payloadTypeTarget(action.Result, modelPkg)
							if changed3 := applyServiceTypeParam(indexListExpr, 2, targetPkg, targetType); changed3 {
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
// to reference targetPkg and actionType, transcribing the declared form; an
// empty targetPkg keeps the current package qualifier.
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
// embedding), a struct renamed away from the role name is renamed back when
// the action declares no Filename (with Filename set the
// applyServiceRoleName rename path owns the name), and a deleted struct is
// regenerated so gg gen always converges on a registrable service struct.
// It reports whether the file was modified.
func forceCanonicalServiceStruct(file *ast.File, action *dsl.Action, modelInfo *ModelInfo) bool {
	if file == nil || action == nil || modelInfo == nil {
		return false
	}
	roleName := action.RoleName()
	if len(roleName) == 0 {
		return false
	}
	qualifier := serviceModelQualifier(modelInfo, action.Phase)

	renamed := false
	spec := findStructTypeSpec(file, roleName)
	if spec == nil {
		existing := findServiceTypeName(file)
		if len(existing) == 0 {
			file.Decls = append(file.Decls, types(qualifier, modelInfo.ModelName, action.Payload, action.Result, roleName))
			ensureServiceImportSpec(file)
			ensureModelImportSpec(file, modelInfo.ImportPath(), qualifier)
			return true
		}
		// With Filename set, a well-formed service struct under another name
		// is out of rewrite scope: it belongs to the applyServiceRoleName
		// rename path, and module-copied services legitimately use their own
		// struct names (they register manually instead of through generated
		// registration code).
		if len(action.Filename) > 0 {
			return false
		}
		// Without Filename no rename path covers the struct, yet the
		// generated registration code references the phase role name, so a
		// renamed struct is restored to the canonical name. Receiver
		// variable names are generation defaults, not framework assets, and
		// stay untouched.
		spec = renameServiceStruct(file, existing, roleName)
		if spec == nil {
			return false
		}
		renamed = true
	}

	structType, ok := spec.Type.(*ast.StructType)
	if !ok {
		return renamed
	}
	if isCanonicalServiceStructBody(structType) {
		return renamed
	}

	baseField := generatedServiceBaseField(qualifier, modelInfo.ModelName, action, roleName)
	if baseField == nil {
		return false
	}
	removeStructInteriorComments(file, structType)
	if structType.Fields == nil {
		structType.Fields = &ast.FieldList{}
	}
	structType.Fields.List = []*ast.Field{baseField}
	ensureServiceImportSpec(file)
	ensureModelImportSpec(file, modelInfo.ImportPath(), qualifier)
	return true
}

// renameServiceStruct renames the service struct declaration from oldName to
// newName and retargets the receiver type of every method bound to it, so
// the struct matches the name referenced by generated registration code.
// Receiver variable names and method bodies are user-visible code and are
// left untouched. It returns the renamed type spec, or nil when no struct
// named oldName exists.
func renameServiceStruct(file *ast.File, oldName, newName string) *ast.TypeSpec {
	spec := findStructTypeSpec(file, oldName)
	if spec == nil {
		return nil
	}
	spec.Name = ast.NewIdent(newName)
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
			continue
		}
		starExpr, ok := funcDecl.Recv.List[0].Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		if ident, ok := starExpr.X.(*ast.Ident); ok && ident.Name == oldName {
			ident.Name = newName
		}
	}
	return spec
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
// exactly as generated service code declares it for the action, referring to
// the model package by modelQualifier (see serviceModelQualifier), as in
// service.Base[*model_service.Item, *model_service.Item, *model_service.Item].
func generatedServiceBaseField(modelQualifier, modelName string, action *dsl.Action, roleName string) *ast.Field {
	decl := types(modelQualifier, modelName, action.Payload, action.Result, roleName)
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

// ensureModelImportSpec inserts the import of the model package at
// importPath that a restored service.Base embedding refers to by
// modelQualifier, named after the qualifier when it is not the last path
// segment, as in model_service "helloworld/model/service" (see
// modelImportSpec).
func ensureModelImportSpec(file *ast.File, importPath, modelQualifier string) {
	if file == nil || findImportSpec(file, importPath) != nil {
		return
	}
	insertImportSpec(file, modelImportSpec(importPath, modelQualifier))
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
// A model package named like a framework package the file imports is referred
// to through an alias (see serviceModelQualifier), so renaming "model/sample"
// to "model/service" turns
//
//	"myproject/model/sample"
//	service.Base[*sample.User, *sample.User, *sample.User]
//
// into
//
//	model_service "myproject/model/service"
//	service.Base[*model_service.User, *model_service.User, *model_service.User]
//
// A file that refers to the model package by the name of a framework package
// it imports cannot build, and nothing tells which package a reference
// through that name means, so it is rejected before anything is rewritten.
// An earlier gg generated such files for a model package named like a
// framework package:
//
//	import (
//		"myproject/model/service"
//		"github.com/hydroan/gst/service"
//	)
//
//	type Creator struct {
//		service.Base[*service.User, *service.User, *service.User]
//	}
//
// The error tells how to fix the file: import the model package as
// model_service "myproject/model/service" and refer to it through
// model_service, or delete the file for gg gen to generate it again. Only the
// framework packages count (see frameworkImportNamed): a file importing
// "github.com/cespare/xxhash/v2", a package named xxhash, next to a model
// package named v2 builds, and is synced like any other.
//
// Parameters:
// - file: The AST file to process
// - action: The DSL action configuration
// - servicePkgName: The expected service package name
// - modelDir: The root model directory, such as model
// - modelInfo: The correct model generation context
//
// It returns whether any changes were made to the file, and the error of a
// file it rejects.
func ApplyServiceFileWithModelSync(file *ast.File, action *dsl.Action, servicePkgName, modelDir string, modelInfo *ModelInfo) (bool, error) {
	if file == nil || action == nil {
		return false, nil
	}
	if modelInfo != nil {
		if name := serviceModelName(file, modelInfo); name != "" {
			if frameworkPath := frameworkImportNamed(file, action.Phase, name); frameworkPath != "" {
				qualifier := serviceModelQualifier(modelInfo, action.Phase)
				return false, errors.Newf("refers to both the model package and %q as %s, so it cannot build; import the model package as %s %q and refer to it through %s, or delete the file for gg gen to generate it again",
					frameworkPath, name, qualifier, modelInfo.ImportPath(), qualifier)
			}
		}
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
		return changed, nil
	}

	// The qualifier the service struct refers to the model package by is
	// mapped to the one it should use, so only the main model import and its
	// references are rewritten, never other sibling model packages the user
	// code imports. The qualifier to use is the package name, or an alias
	// when a framework package the file imports takes that name (see
	// serviceModelQualifier).
	currentQualifier := serviceModelPackageName(file)
	correctQualifier := serviceModelQualifier(modelInfo, action.Phase)
	if currentQualifier == "" || currentQualifier == correctQualifier {
		// No import changes needed
		return changed, nil
	}
	importMapping := map[string]string{currentQualifier: correctQualifier}

	// The import declares a name only for an alias: otherwise the name of
	// the package it imports is the qualifier.
	importName := ""
	if correctQualifier != modelInfo.ModelPkgName {
		importName = correctQualifier
	}

	// Update import statements
	if syncModelImports(file, path.Join(modelInfo.ModulePath, modelDir), modelInfo.ImportPath(), importName, importMapping) {
		changed = true
	}

	// Update package references in the code (e.g., identity.Login -> iam.Login)
	if syncModelPackageReferences(file, importMapping) {
		changed = true
	}

	return changed, nil
}

// serviceModelName returns the name the file refers to the model package by:
// the qualifier of the service struct's service.Base embedding, or, with no
// such struct, the name of the model import, which is the model package name
// unless the import declares another. It returns "" when the file shows
// neither.
func serviceModelName(file *ast.File, modelInfo *ModelInfo) string {
	if name := serviceModelPackageName(file); name != "" {
		return name
	}
	spec := findImportSpec(file, modelInfo.ImportPath())
	switch {
	case spec == nil:
		return ""
	case spec.Name != nil:
		return spec.Name.Name
	default:
		return modelInfo.ModelPkgName
	}
}

// frameworkImportNamed returns the path of the package of
// serviceScaffoldImports that the service file of an action with phase
// imports under name, or "" when none of them takes that name. Their names
// are known without reading them, the last segment of their paths unless the
// import declares another, so the file
//
//	"myproject/model/service"
//	"github.com/hydroan/gst/service"
//
// imports "github.com/hydroan/gst/service" under service. Any other package
// imported without a name is left out: only the package itself tells its
// name, as with "github.com/cespare/xxhash/v2", named xxhash.
func frameworkImportNamed(file *ast.File, phase consts.Phase, name string) string {
	frameworkPaths := serviceScaffoldImports(phase)
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
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil || !slices.Contains(frameworkPaths, importPath) {
				continue
			}
			importName := path.Base(importPath)
			if importSpec.Name != nil {
				importName = importSpec.Name.Name
			}
			if importName == name {
				return importPath
			}
		}
	}
	return ""
}

// serviceModelPackageName returns the qualifier the service struct of the
// file refers to the model package by: the package of the first type
// parameter of its service.Base embedding, as in sample for
// service.Base[*sample.User, *sample.UserReq, *sample.UserRsp]. It returns ""
// when the file declares no service struct.
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

// selectorPackageName returns the package qualifier of a qualified type,
// looking through one pointer: sample for *sample.User and sample.User, and
// "" for any other expression.
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

// syncModelImports points the model import whose name is a key of mapping at
// correctModelImportPath, declaring importName as its name, or no name when
// importName is empty. Only imports of the model packages under modelRoot,
// such as "myproject/model", count (see modelImportName); an import of any
// other package is never touched, even when its path ends in the same name.
// For example, with mapping {"v2": "v3"},
//
//	"myproject/model/api/v2"
//	"github.com/cespare/xxhash/v2"
//
// becomes
//
//	"myproject/model/api/v3"
//	"github.com/cespare/xxhash/v2"
//
// and with mapping {"sample": "model_service"} and importName model_service,
// "myproject/model/sample" becomes model_service "myproject/model/service".
// It modifies only the import paths and names in the AST, preserving all
// other aspects of the code including formatting, comments, and structure,
// and returns true if any imports were updated.
func syncModelImports(file *ast.File, modelRoot, correctModelImportPath, importName string, mapping map[string]string) bool {
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
			if _, found := mapping[modelImportName(importSpec, modelRoot)]; !found {
				continue
			}

			importSpec.Path.Value = strconv.Quote(correctModelImportPath)
			importSpec.Name = nil
			if importName != "" {
				importSpec.Name = ast.NewIdent(importName)
			}
			changed = true
		}
	}

	return changed
}

// modelImportName returns the name a file refers to the model package it
// imports through spec by: the name the import declares, or else the name
// the package declares, which gg check pins to its directory name (see
// ModelPackageName). It returns "" for an import of any package outside
// modelRoot, or one that gives the file no name to refer to it by:
//
//	"myproject/model/sample"                 // sample
//	"myproject/model/record_item"            // recorditem
//	model_service "myproject/model/service"  // model_service
//	"github.com/cespare/xxhash/v2"           // "", not a model package
func modelImportName(spec *ast.ImportSpec, modelRoot string) string {
	importPath, err := strconv.Unquote(spec.Path.Value)
	if err != nil || (importPath != modelRoot && !strings.HasPrefix(importPath, modelRoot+"/")) {
		return ""
	}
	if spec.Name != nil {
		if spec.Name.Name == "_" || spec.Name.Name == "." {
			return ""
		}
		return spec.Name.Name
	}
	return ModelPackageName(path.Base(importPath))
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
