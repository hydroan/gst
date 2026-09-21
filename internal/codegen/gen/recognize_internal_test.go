package gen

import (
	"go/ast"
	"testing"

	"github.com/hydroan/gst/consts"
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
	// Positive case: types transcribes the bare payload and result names as value
	// types, so the struct embeds service.Base[*model.User, model.User, model.User]
	gd := types("model", "User", "User", "User", consts.PHASE_CREATE, consts.PHASE_CREATE.RoleName())
	if len(gd.Specs) == 0 {
		t.Fatalf("types() returned no specs")
	}
	ts, ok := gd.Specs[0].(*ast.TypeSpec)
	if !ok {
		t.Fatalf("expected first spec to be *ast.TypeSpec")
	}
	if !isServiceType(ts) {
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
	if !isServiceType(pos2) {
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
	if isServiceType(neg1) {
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
	if isServiceType(neg2) {
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
	if isServiceType(neg3) {
		t.Fatalf("expected isServiceType to return false for wrong number of type params")
	}
}
