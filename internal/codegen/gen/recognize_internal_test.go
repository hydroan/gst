package gen

import (
	"go/ast"
	"go/token"
	"strconv"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/ggconst"
)

func TestIsServiceMethod1(t *testing.T) {
	fn1 := serviceMethod1("u", "User", "model", consts.PHASE_CREATE_BEFORE, "Creator")
	fn2 := serviceMethod2("u", "User", "model", consts.PHASE_LIST_BEFORE, "Lister")
	if !isServiceMethod1(fn1) {
		t.Fatalf("expected isServiceMethod1 to return true for ServiceMethod1-generated func")
	}
	if isServiceMethod1(fn2) {
		t.Fatalf("expected isServiceMethod1 to return false for non-matching func (ServiceMethod2)")
	}
}

func TestIsServiceMethod2(t *testing.T) {
	fn := serviceMethod2("u", "User", "model", consts.PHASE_LIST_BEFORE, "Lister")
	fnNeg := serviceMethod3("u", "User", "model", consts.PHASE_CREATE_MANY_BEFORE, "ManyCreator")
	if !isServiceMethod2(fn) {
		t.Fatalf("expected isServiceMethod2 to return true for ServiceMethod2-generated func")
	}
	if isServiceMethod2(fnNeg) {
		t.Fatalf("expected isServiceMethod2 to return false for non-matching func (ServiceMethod3)")
	}
}

func TestIsServiceMethod3(t *testing.T) {
	fn := serviceMethod3("u", "User", "model", consts.PHASE_CREATE_MANY_BEFORE, "ManyCreator")
	fnNeg := serviceMethod1("u", "User", "model", consts.PHASE_CREATE_BEFORE, "Creator")
	if !isServiceMethod3(fn) {
		t.Fatalf("expected isServiceMethod3 to return true for ServiceMethod3-generated func")
	}
	if isServiceMethod3(fnNeg) {
		t.Fatalf("expected isServiceMethod3 to return false for non-matching func (ServiceMethod1)")
	}
}

func TestIsServiceMethod4(t *testing.T) {
	fn := serviceMethod4("u", "model", "*UserReq", "*UserRsp", consts.PHASE_CREATE, "Creator")
	fnNeg := serviceMethod3("u", "User", "model", consts.PHASE_CREATE_MANY_BEFORE, "ManyCreator")
	if !isServiceMethod4(fn) {
		t.Fatalf("expected isServiceMethod4 to return true for ServiceMethod4-generated func")
	}
	if isServiceMethod4(fnNeg) {
		t.Fatalf("expected isServiceMethod4 to return false for non-matching func (ServiceMethod3)")
	}
}

func TestIsServiceType(t *testing.T) {
	// The file imports the framework service package the way generated
	// service files do; the qualifier of service.Base is read from it.
	file := serviceImportFile("")

	// Positive case: types transcribes the bare payload and result names as value
	// types, so the struct embeds service.Base[*model.User, model.User, model.User]
	gd := types("model", "User", "User", "User", consts.PHASE_CREATE.RoleName())
	if len(gd.Specs) == 0 {
		t.Fatalf("types() returned no specs")
	}
	ts, ok := gd.Specs[0].(*ast.TypeSpec)
	if !ok {
		t.Fatalf("expected first spec to be *ast.TypeSpec")
	}
	if !isServiceType(file, ts) {
		t.Fatalf("expected isServiceType to return true for valid service.Base with a pointer model and value payload and result")
	}

	// Positive case 2: struct embeds service.Base with mixed pointer and non-pointer types
	pos2 := &ast.TypeSpec{
		Name: ast.NewIdent("userx"),
		Type: &ast.StructType{
			Fields: &ast.FieldList{List: []*ast.Field{
				{Type: &ast.IndexListExpr{
					X: &ast.SelectorExpr{X: ast.NewIdent("service"), Sel: ast.NewIdent("Base")},
					Indices: []ast.Expr{
						&ast.SelectorExpr{X: ast.NewIdent("model"), Sel: ast.NewIdent("User")},                      // non-pointer
						&ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent("model"), Sel: ast.NewIdent("UserReq")}}, // pointer
						&ast.SelectorExpr{X: ast.NewIdent("model"), Sel: ast.NewIdent("UserRsp")},                   // non-pointer
					},
				}},
			}},
		},
	}
	if !isServiceType(file, pos2) {
		t.Fatalf("expected isServiceType to return true for valid service.Base with mixed pointer and non-pointer types")
	}

	// Negative case 1: wrong selector name (service.Other)
	neg1 := &ast.TypeSpec{
		Name: ast.NewIdent("userx"),
		Type: &ast.StructType{
			Fields: &ast.FieldList{List: []*ast.Field{
				{Type: &ast.IndexListExpr{
					X: &ast.SelectorExpr{X: ast.NewIdent("service"), Sel: ast.NewIdent("Other")},
					Indices: []ast.Expr{
						&ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent("model"), Sel: ast.NewIdent("User")}},
						&ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent("model"), Sel: ast.NewIdent("User")}},
						&ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent("model"), Sel: ast.NewIdent("User")}},
					},
				}},
			}},
		},
	}
	if isServiceType(file, neg1) {
		t.Fatalf("expected isServiceType to return false for non-Base selector")
	}

	// Negative case 2: one of the type params is an invalid type (not pointer or selector)
	neg2 := &ast.TypeSpec{
		Name: ast.NewIdent("userx"),
		Type: &ast.StructType{
			Fields: &ast.FieldList{List: []*ast.Field{
				{Type: &ast.IndexListExpr{
					X: &ast.SelectorExpr{X: ast.NewIdent("service"), Sel: ast.NewIdent("Base")},
					Indices: []ast.Expr{
						&ast.BasicLit{Kind: 1, Value: "string"}, // invalid type - basic literal
						&ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent("model"), Sel: ast.NewIdent("User")}},
						&ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent("model"), Sel: ast.NewIdent("User")}},
					},
				}},
			}},
		},
	}
	if isServiceType(file, neg2) {
		t.Fatalf("expected isServiceType to return false when a type param is invalid")
	}

	// Negative case 3: incorrect number of type params (2 instead of 3)
	neg3 := &ast.TypeSpec{
		Name: ast.NewIdent("userx"),
		Type: &ast.StructType{
			Fields: &ast.FieldList{List: []*ast.Field{
				{Type: &ast.IndexListExpr{
					X: &ast.SelectorExpr{X: ast.NewIdent("service"), Sel: ast.NewIdent("Base")},
					Indices: []ast.Expr{
						&ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent("model"), Sel: ast.NewIdent("User")}},
						&ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent("model"), Sel: ast.NewIdent("User")}},
					},
				}},
			}},
		},
	}
	if isServiceType(file, neg3) {
		t.Fatalf("expected isServiceType to return false for wrong number of type params")
	}

	// Positive case 3: the framework service package imported under an alias
	aliased := &ast.TypeSpec{
		Name: ast.NewIdent("userx"),
		Type: &ast.StructType{
			Fields: &ast.FieldList{List: []*ast.Field{
				{Type: &ast.IndexListExpr{
					X: &ast.SelectorExpr{X: ast.NewIdent("svc"), Sel: ast.NewIdent("Base")},
					Indices: []ast.Expr{
						&ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent("model"), Sel: ast.NewIdent("User")}},
						&ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent("model"), Sel: ast.NewIdent("User")}},
						&ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent("model"), Sel: ast.NewIdent("User")}},
					},
				}},
			}},
		},
	}
	if !isServiceType(serviceImportFile("svc"), aliased) {
		t.Fatalf("expected isServiceType to return true for service.Base under an import alias")
	}

	// Negative case 4: a file that does not import the framework service
	// package embeds no service.Base of it
	if isServiceType(&ast.File{}, ts) {
		t.Fatalf("expected isServiceType to return false without the service import")
	}
}

// serviceImportFile returns a file importing the framework service package,
// under name when it is not empty.
func serviceImportFile(name string) *ast.File {
	spec := &ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(ggconst.ImportPathService)}}
	if name != "" {
		spec.Name = ast.NewIdent(name)
	}
	return &ast.File{Imports: []*ast.ImportSpec{spec}}
}
