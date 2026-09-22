package gen

import (
	"go/ast"
	"slices"

	"github.com/hydroan/gst/internal/goast"
)

// isServiceMethod1 checks whether a function declaration matches the shape
// genServiceMethod1 generates through serviceMethod1, which is:
//
//	func (r *recv) Method(ctx *gst.ServiceContext, m *<pkg>.<Model>) error
func isServiceMethod1(fn *ast.FuncDecl) bool {
	if fn == nil || fn.Recv == nil || fn.Type == nil || fn.Type.Params == nil || fn.Type.Results == nil {
		return false
	}
	// receiver must be pointer to an ident (e.g., *user)
	if !goast.IsPointerReceiver(fn.Recv) {
		return false
	}
	// params: (ctx *gst.ServiceContext, second *pkg.Model)
	if len(fn.Type.Params.List) != 2 {
		return false
	}
	if !isServiceContextParam(fn.Type.Params.List[0]) {
		return false
	}
	if !goast.IsPointerToQualified(fn.Type.Params.List[1].Type) {
		return false
	}
	// results: error
	if len(fn.Type.Results.List) != 1 {
		return false
	}
	if !goast.IsBuiltinError(fn.Type.Results.List[0].Type) {
		return false
	}
	return true
}

// isServiceMethod2 checks whether a function declaration matches the shape
// genServiceMethod2 generates through serviceMethod2, which is:
//
//	func (r *recv) Method(ctx *gst.ServiceContext, list *[]*<pkg>.<Model>) error
func isServiceMethod2(fn *ast.FuncDecl) bool {
	if fn == nil || fn.Recv == nil || fn.Type == nil || fn.Type.Params == nil || fn.Type.Results == nil {
		return false
	}
	if !goast.IsPointerReceiver(fn.Recv) {
		return false
	}
	if len(fn.Type.Params.List) != 2 {
		return false
	}
	if !isServiceContextParam(fn.Type.Params.List[0]) {
		return false
	}
	// Second param must be: *[]*pkg.Model
	if !isPointerToSliceOfPointers(fn.Type.Params.List[1]) {
		return false
	}
	// results: error
	if len(fn.Type.Results.List) != 1 {
		return false
	}
	if !goast.IsBuiltinError(fn.Type.Results.List[0].Type) {
		return false
	}
	return true
}

// isServiceMethod3 checks whether a function declaration matches the shape
// genServiceMethod3 generates through serviceMethod3, which is:
//
//	func (r *recv) Method(ctx *gst.ServiceContext, list ...*<pkg>.<Model>) error
func isServiceMethod3(fn *ast.FuncDecl) bool {
	if fn == nil || fn.Recv == nil || fn.Type == nil || fn.Type.Params == nil || fn.Type.Results == nil {
		return false
	}
	if !goast.IsPointerReceiver(fn.Recv) {
		return false
	}
	if len(fn.Type.Params.List) != 2 {
		return false
	}
	if !isServiceContextParam(fn.Type.Params.List[0]) {
		return false
	}
	// Second param must be: ...*pkg.Model
	if !isVariadicOfPointers(fn.Type.Params.List[1]) {
		return false
	}
	// results: error
	if len(fn.Type.Results.List) != 1 {
		return false
	}
	if !goast.IsBuiltinError(fn.Type.Results.List[0].Type) {
		return false
	}
	return true
}

// isServiceMethod4 checks whether a function declaration matches the shape
// genServiceMethod4 generates through serviceMethod4, which is:
//
//	func (r *recv) Method(ctx *gst.ServiceContext, req *<pkg>.<Req>) (*<pkg>.<Rsp>, error)
//	func (r *recv) Method(ctx *gst.ServiceContext, req <pkg>.<Req>) (<pkg>.<Rsp>, error)
func isServiceMethod4(fn *ast.FuncDecl) bool {
	if fn == nil || fn.Recv == nil || fn.Type == nil || fn.Type.Params == nil || fn.Type.Results == nil {
		return false
	}
	if !goast.IsPointerReceiver(fn.Recv) {
		return false
	}
	if len(fn.Type.Params.List) != 2 {
		return false
	}
	if !isServiceContextParam(fn.Type.Params.List[0]) {
		return false
	}
	// Second param must be: *pkg.Req or pkg.Req
	if !isQualifiedOrPointer(fn.Type.Params.List[1].Type) {
		return false
	}
	// results: (*pkg.Rsp, error) or (pkg.Rsp, error)
	if len(fn.Type.Results.List) != 2 {
		return false
	}
	if !isQualifiedOrPointer(fn.Type.Results.List[0].Type) {
		return false
	}
	if !goast.IsBuiltinError(fn.Type.Results.List[1].Type) {
		return false
	}
	return true
}

// ---------- helpers ----------

// isServiceContextParam checks the parameter is declared as
// *gst.ServiceContext, the way every generated service method declares its
// first parameter.
func isServiceContextParam(field *ast.Field) bool {
	if field == nil {
		return false
	}
	star, ok := field.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return pkg.Name == "gst" && sel.Sel != nil && sel.Sel.Name == "ServiceContext"
}

// isQualifiedOrPointer checks the type is qualified by a package name, by
// value or behind one pointer, as in model.User and *model.User.
func isQualifiedOrPointer(expr ast.Expr) bool {
	return goast.IsQualified(expr) || goast.IsPointerToQualified(expr)
}

// isPointerToSliceOfPointers checks the parameter is declared as a pointer
// to a slice of pointers to a qualified type, as in *[]*model.User.
func isPointerToSliceOfPointers(field *ast.Field) bool {
	if field == nil {
		return false
	}
	star, ok := field.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	arr, ok := star.X.(*ast.ArrayType)
	return ok && goast.IsPointerToQualified(arr.Elt)
}

// isVariadicOfPointers checks the parameter is variadic over pointers to a
// qualified type, as in ...*model.User.
func isVariadicOfPointers(field *ast.Field) bool {
	if field == nil {
		return false
	}
	ell, ok := field.Type.(*ast.Ellipsis)
	return ok && goast.IsPointerToQualified(ell.Elt)
}

// isServiceType checks if a type declaration of file is a service struct: a
// struct embedding service.Base over the model, request and response types
// (see isServiceBaseEmbedding), as in
//
//	type Creator struct {
//		service.Base[*model.User, *model.UserReq, *model.UserRsp]
//	}
func isServiceType(file *ast.File, spec *ast.TypeSpec) bool {
	if spec == nil || spec.Type == nil {
		return false
	}
	structType, ok := spec.Type.(*ast.StructType)
	if !ok || structType.Fields == nil {
		return false
	}
	return slices.ContainsFunc(structType.Fields.List, func(field *ast.Field) bool {
		return isServiceBaseEmbedding(file, field)
	})
}

// isServiceBaseEmbedding checks the field embeds the framework's service.Base
// (see goast.IsServiceBase) over type arguments the generator can
// rewrite, each qualified by a package name by value or behind one pointer,
// as in service.Base[*model.User, *model.UserReq, model.UserRsp].
func isServiceBaseEmbedding(file *ast.File, field *ast.Field) bool {
	if !goast.IsServiceBase(file, field) {
		return false
	}
	instance, ok := field.Type.(*ast.IndexListExpr)
	return ok && !slices.ContainsFunc(instance.Indices, func(arg ast.Expr) bool {
		return !isQualifiedOrPointer(arg)
	})
}
