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

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/goast"
)

// ModelTableNameDeclaration requires every database model to declare its table
// name as a string literal.
var ModelTableNameDeclaration = Check{
	Name: "Model table name declaration",
	Rule: "model structs embedding model.Base or model.AutoBase must declare TableName() string on the struct itself, as a single return of a non-empty string literal",
	run:  checkModelTableNameDeclaration,
}

// checkModelTableNameDeclaration reports business model structs that leave
// their table name undeclared or declare it as anything but a single return
// of a non-empty string literal. A model embedding model.Base or
// model.AutoBase must declare TableName() string itself: gorm's Tabler and
// the framework read the same method, and the base default "" is rejected at
// startup and inside gg migrate, so a missing declaration is a guaranteed
// runtime failure this check surfaces at development time instead. The
// literal form is required because the method runs on zero-value instances;
// a name computed from runtime state could hand different names to different
// callers. The method may live in any file of the model's package, so
// declarations and methods aggregate per directory before matching. Model
// subtrees owned by copyable framework modules are skipped, since copied
// module code is owned by the framework repository.
func checkModelTableNameDeclaration(ignore gitignore.Matcher) []string {
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
		literal  bool
		position token.Position
	}
	models := make([]modelDecl, 0)
	methods := make(map[string]methodDecl)
	methodKey := func(dir, receiver string) string { return dir + "\x00" + receiver }

	var violations []string
	walkErr := walkProjectDir(ggconst.DirModel, ignore, func(path string, info os.FileInfo) error {
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
			violations = append(violations, fmt.Sprintf("%s has parse error: %v", relativePath(path), err))
			return nil
		}
		dir := filepath.Dir(path)
		modelNames := goast.ImportedNames(node, ggconst.ImportPathModel, ggconst.PkgModel)

		for _, decl := range node.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
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
				receiver, ok := actionTypeBaseName(d.Recv.List[0].Type)
				if !ok {
					continue
				}
				methods[methodKey(dir, receiver)] = methodDecl{
					literal:  isNonEmptyStringLiteralReturn(d),
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
				relativePath(model.position.Filename), model.position.Line, model.name, model.embedded,
			))
			continue
		}
		if !method.literal {
			violations = append(violations, fmt.Sprintf(
				"%s:%d: TableName of model '%s' must be a single return of a non-empty string literal: the framework and gg migrate call it on zero-value instances",
				relativePath(method.position.Filename), method.position.Line, model.name,
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

// isNonEmptyStringLiteralReturn reports whether the method body is exactly
// one return of a non-empty string literal.
func isNonEmptyStringLiteralReturn(decl *ast.FuncDecl) bool {
	if decl.Body == nil || len(decl.Body.List) != 1 {
		return false
	}
	ret, ok := decl.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	lit, ok := ret.Results[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	value, err := strconv.Unquote(lit.Value)
	return err == nil && len(value) > 0
}
