package ggcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/goast"
)

// ModelTableNameDeclaration requires every database model to declare its table
// name as a string literal, or as a constant holding one.
var ModelTableNameDeclaration = Check{
	Name: "Model table name declaration",
	Rule: "model structs embedding model.Base or model.AutoBase must declare TableName() string on the struct itself, as a single return of a non-empty string literal",
	run:  checkModelTableNameDeclaration,
}

// checkModelTableNameDeclaration reports business model structs that leave
// their table name undeclared or declare it as anything but a single return
// of a non-empty string literal, or of a constant of the package holding
// one. A model embedding model.Base or model.AutoBase must declare
// TableName() string itself: gorm's Tabler and the framework read the same
// method, and the base default "" is rejected at startup and inside gg
// migrate, so a missing declaration is a guaranteed runtime failure this
// check surfaces at development time instead. The literal form is required
// because the method runs on zero-value instances; a name computed from
// runtime state could hand different names to different callers, where a
// constant is as fixed as a literal. The method and the constant may live
// in any file of the model's package, so declarations, methods and
// constants aggregate per directory before matching. Model subtrees owned
// by copyable framework modules are skipped, since copied module code is
// owned by the framework repository.
func checkModelTableNameDeclaration(ignore gghelper.ProjectIgnore) []string {
	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return nil
	}

	owned, err := copyableModuleOwners()
	if err != nil {
		return []string{fmt.Sprintf("listing copyable framework modules: %v", err)}
	}

	type modelDecl struct {
		dir      string
		name     string
		embedded string
		position token.Position
	}
	type methodDecl struct {
		literal  bool   // the method returns a non-empty string literal
		constant string // the constant the method returns instead, "" for none
		position token.Position
	}
	models := make([]modelDecl, 0)
	methods := make(map[string]methodDecl)
	// constants holds the non-empty string constants of each directory.
	constants := make(map[string]string)
	methodKey := func(dir, receiver string) string { return dir + "\x00" + receiver }

	var violations []string
	walkErr := ignore.Walk(ggconst.DirModel, func(path string, info os.FileInfo) error {
		if info.IsDir() {
			if moduleOwnedPath(owned, ggconst.DirModel, path) {
				return filepath.SkipDir
			}
			return nil
		}
		base := filepath.Base(path)
		if !strings.HasSuffix(base, ".go") || strings.HasSuffix(base, "_test.go") || isGeneratedFileName(path) {
			return nil
		}

		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s has parse error: %v", gghelper.RelativePath(path), err))
			return nil
		}
		dir := filepath.Dir(path)
		modelNames := goast.ImportedNames(node, ggconst.ImportPathModel, ggconst.PkgModel)

		for _, decl := range node.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					if valueSpec, ok := spec.(*ast.ValueSpec); ok && d.Tok == token.CONST {
						for i, name := range valueSpec.Names {
							if i < len(valueSpec.Values) {
								if value, ok := nonEmptyStringLiteral(valueSpec.Values[i]); ok {
									constants[methodKey(dir, name.Name)] = value
								}
							}
						}
						continue
					}
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok || typeSpec.Name == nil {
						continue
					}
					structType, ok := typeSpec.Type.(*ast.StructType)
					if !ok || structType.Fields == nil {
						continue
					}
					embedded := embeddedBaseName(structType, modelNames)
					if embedded == "" {
						continue
					}
					models = append(models, modelDecl{
						dir:      dir,
						name:     typeSpec.Name.Name,
						embedded: embedded,
						position: fset.Position(typeSpec.Pos()),
					})
				}
			case *ast.FuncDecl:
				if d.Name == nil || d.Name.Name != "TableName" || d.Recv == nil || len(d.Recv.List) == 0 {
					continue
				}
				receiver, ok := typeBaseName(d.Recv.List[0].Type)
				if !ok {
					continue
				}
				literal, constant := tableNameReturn(d)
				methods[methodKey(dir, receiver)] = methodDecl{
					literal:  literal,
					constant: constant,
					position: fset.Position(d.Pos()),
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		violations = append(violations, fmt.Sprintf("walking model directory: %v", walkErr))
	}

	for _, model := range models {
		method, ok := methods[methodKey(model.dir, model.name)]
		if !ok {
			violations = append(violations, fmt.Sprintf(
				"%s:%d: model '%s' embeds %s but declares no TableName() string; declare it on the struct returning a non-empty string literal — the base default \"\" fails at startup and inside gg migrate",
				gghelper.RelativePath(model.position.Filename), model.position.Line, model.name, model.embedded,
			))
			continue
		}
		if !method.literal && (method.constant == "" || constants[methodKey(model.dir, method.constant)] == "") {
			violations = append(violations, fmt.Sprintf(
				"%s:%d: TableName of model '%s' must be a single return of a non-empty string literal, or of a constant holding one: the framework and gg migrate call it on zero-value instances",
				gghelper.RelativePath(method.position.Filename), method.position.Line, model.name,
			))
		}
	}
	return violations
}

// embeddedBaseName returns the framework base a struct embeds by value
// ("model.Base" or "model.AutoBase"), or "" when it embeds neither. Virtual
// models embed model.Empty and have no table, so they never report here, and
// a base embedded through a pointer is no model the framework recognizes: the
// DSL design rules report that embedding.
func embeddedBaseName(structType *ast.StructType, modelNames goast.PackageNames) string {
	for _, field := range structType.Fields.List {
		if len(field.Names) != 0 {
			continue
		}
		switch {
		case modelNames.Refers(field.Type, ggconst.FieldBase):
			return "model.Base"
		case modelNames.Refers(field.Type, ggconst.FieldAutoBase):
			return "model.AutoBase"
		}
	}
	return ""
}

// tableNameReturn reads the single return of the method body: whether it
// returns a non-empty string literal, and the name of the constant it
// returns instead, "" for none.
func tableNameReturn(decl *ast.FuncDecl) (literal bool, constant string) {
	if decl.Body == nil || len(decl.Body.List) != 1 {
		return false, ""
	}
	ret, ok := decl.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false, ""
	}
	if _, ok := nonEmptyStringLiteral(ret.Results[0]); ok {
		return true, ""
	}
	if ident, ok := ret.Results[0].(*ast.Ident); ok {
		return false, ident.Name
	}
	return false, ""
}

// nonEmptyStringLiteral returns the value of expr when it is a non-empty
// string literal.
func nonEmptyStringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	return value, err == nil && len(value) > 0
}
