package goast

import (
	"go/ast"

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
