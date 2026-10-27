package goast

import (
	"go/ast"
	"go/token"
	"slices"

	"github.com/hydroan/gst/internal/ggconst"
)

// IsServiceBase reports whether field embeds the framework's service
// base, service.Base[M, REQ, RSP], referred to the way file imports the
// framework service package: under a name the file imports the package by,
// or bare under a dot import (see ImportedNames). With
//
//	import svc "github.com/hydroan/gst/service"
//
// the embedded field svc.Base[*model.User, *model.UserReq, *model.UserRsp]
// reports true, while a named field, service.Base when service names a
// package other than the framework's, and a Base without its three type
// arguments report false.
func IsServiceBase(file *ast.File, field *ast.Field) bool {
	if file == nil || field == nil || len(field.Names) != 0 {
		return false
	}
	instance, ok := field.Type.(*ast.IndexListExpr)
	if !ok || len(instance.Indices) != 3 {
		return false
	}
	return ImportedNames(file, ggconst.ImportPathService, ggconst.PkgService).Refers(instance.X, "Base")
}

// ServiceStructNames returns the names of the struct types file declares
// that embed the framework's service base (see IsServiceBase), in
// declaration order: Creator for a file declaring
//
//	type Creator struct {
//		service.Base[*model.User, *model.UserReq, *model.UserRsp]
//	}
//
//	type options struct{ verbose bool }
//
// with the framework's service package imported as service, and nil for a
// file declaring no such struct.
func ServiceStructNames(file *ast.File) []string {
	var names []string
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if ok && structType.Fields != nil && slices.ContainsFunc(structType.Fields.List, func(field *ast.Field) bool {
				return IsServiceBase(file, field)
			}) {
				names = append(names, typeSpec.Name.Name)
			}
		}
	}
	return names
}
