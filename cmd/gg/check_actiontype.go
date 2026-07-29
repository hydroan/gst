package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/hydroan/gst/dsl"
)

// CheckActionTypeForm checks the explicit DSL Payload and Result type
// declarations of every model package. The type argument must be a named type
// declared in the same package; struct types must use the pointer form, slice
// and map types must use the value form, and an empty struct type is the
// delegation marker for actions without data, so it may only pair with an
// empty peer side (an omitted side counts as empty).
func CheckActionTypeForm(ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return violations
	}

	// Files are grouped per directory because the declared types may live in a
	// different file of the package that references them.
	var packageDirs []string
	packageFiles := make(map[string][]string)
	err := walkProjectDir(modelDir, ignore, func(path string, info os.FileInfo) error {
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if isGeneratedFileName(path) {
			return nil
		}
		dir := filepath.Dir(path)
		if _, seen := packageFiles[dir]; !seen {
			packageDirs = append(packageDirs, dir)
		}
		packageFiles[dir] = append(packageFiles[dir], path)
		return nil
	})
	if err != nil {
		violations = append(violations, fmt.Sprintf("walking model directory: %v", err))
	}

	for _, dir := range packageDirs {
		violations = append(violations, checkPackageActionTypeForm(packageFiles[dir])...)
	}

	return violations
}

// actionTypeKind classifies the underlying type of one named action type.
type actionTypeKind int

const (
	actionTypeNotFound actionTypeKind = iota
	actionTypeStruct
	actionTypeEmptyStruct
	actionTypeSliceOrMap
	actionTypeOther
)

// checkPackageActionTypeForm checks the DSL action type declarations of one
// model package.
func checkPackageActionTypeForm(paths []string) []string {
	var violations []string

	fset := token.NewFileSet()
	type parsedFile struct {
		path string
		node *ast.File
	}
	files := make([]parsedFile, 0, len(paths))
	for _, path := range paths {
		node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			continue
		}
		files = append(files, parsedFile{path: path, node: node})
	}

	// Collect every named type declaration of the package so action types can
	// be resolved across files.
	typeExprs := make(map[string]ast.Expr)
	for _, file := range files {
		for _, decl := range file.node.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}
			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || typeSpec.Name == nil {
					continue
				}
				typeExprs[typeSpec.Name.Name] = typeSpec.Type
			}
		}
	}
	resolve := func(name string) actionTypeKind {
		seen := make(map[string]bool)
		expr, ok := typeExprs[name]
		if !ok {
			return actionTypeNotFound
		}
		for {
			switch t := expr.(type) {
			case *ast.StructType:
				if t.Fields == nil || len(t.Fields.List) == 0 {
					return actionTypeEmptyStruct
				}
				return actionTypeStruct
			case *ast.ArrayType, *ast.MapType:
				return actionTypeSliceOrMap
			case *ast.StarExpr:
				expr = t.X
			case *ast.Ident:
				if seen[t.Name] {
					return actionTypeOther
				}
				seen[t.Name] = true
				next, ok := typeExprs[t.Name]
				if !ok {
					return actionTypeNotFound
				}
				expr = next
			default:
				return actionTypeOther
			}
		}
	}

	for _, file := range files {
		relPath := relativePath(file.path)

		// The parser silently drops unsupported type arguments, so reject them
		// at the AST level before judging the parsed action strings.
		for _, decl := range file.node.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Name == nil || funcDecl.Name.Name != "Design" || funcDecl.Recv == nil || funcDecl.Body == nil {
				continue
			}
			ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				kind, typeExpr, ok := dslActionTypeCall(call.Fun)
				if !ok {
					return true
				}
				if _, ok := localActionTypeName(typeExpr); !ok {
					pos := fset.Position(call.Pos())
					violations = append(violations, fmt.Sprintf(
						"%s:%d: %s type argument must be a named type declared in the same model package",
						relPath, pos.Line, kind,
					))
				}
				return true
			})
		}

		// Judge the parsed action type strings per action, so the empty
		// struct pair rule sees both sides of one action together.
		designs := dsl.Parse(file.node, "")
		for _, modelName := range slices.Sorted(maps.Keys(designs)) {
			designs[modelName].Range(func(_ string, action *dsl.Action) {
				violations = append(violations, checkActionTypePair(relPath, action, resolve)...)
			})
		}
	}

	return violations
}

// checkActionTypePair checks the Payload and Result type strings of one
// action against the package type table.
func checkActionTypePair(relPath string, action *dsl.Action, resolve func(string) actionTypeKind) []string {
	var violations []string

	actionName := action.Phase.MethodName()
	sideEmpty := func(raw string) bool {
		if raw == dsl.PayloadEmpty {
			return true
		}
		return resolve(strings.TrimPrefix(raw, "*")) == actionTypeEmptyStruct
	}
	bothEmpty := sideEmpty(action.Payload) && sideEmpty(action.Result)

	sides := []struct {
		kind string
		raw  string
	}{
		{kind: "Payload", raw: action.Payload},
		{kind: "Result", raw: action.Result},
	}
	for _, side := range sides {
		kind, raw := side.kind, side.raw
		if raw == dsl.PayloadEmpty || raw == "" {
			continue
		}
		name := strings.TrimPrefix(raw, "*")
		pointer := strings.HasPrefix(raw, "*")

		switch resolve(name) {
		case actionTypeNotFound:
			violations = append(violations, fmt.Sprintf(
				"%s: %s action declares %s[%s] but the type is not declared in the model package",
				relPath, actionName, kind, raw,
			))
		case actionTypeStruct:
			if !pointer {
				violations = append(violations, fmt.Sprintf(
					"%s: %s action declares %s[%s] with the value form; a struct action type must use the pointer form %s[*%s]",
					relPath, actionName, kind, name, kind, name,
				))
			}
		case actionTypeEmptyStruct:
			if !bothEmpty {
				violations = append(violations, fmt.Sprintf(
					"%s: %s action declares %s[%s] whose type is an empty struct; remove the declaration so the framework defaults this side to *model.Empty",
					relPath, actionName, kind, raw,
				))
				continue
			}
			if !pointer {
				violations = append(violations, fmt.Sprintf(
					"%s: %s action declares %s[%s] with the value form; a struct action type must use the pointer form %s[*%s]",
					relPath, actionName, kind, name, kind, name,
				))
			}
		case actionTypeSliceOrMap:
			if pointer {
				violations = append(violations, fmt.Sprintf(
					"%s: %s action declares %s[*%s] with the pointer form; a slice or map action type must use the value form %s[%s]",
					relPath, actionName, kind, name, kind, name,
				))
			}
		case actionTypeOther:
			// The underlying type cannot be classified inside this package
			// (for example an alias to another package's type); no form
			// verdict is possible.
		}
	}

	return violations
}
