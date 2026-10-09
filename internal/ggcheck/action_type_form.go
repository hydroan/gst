package ggcheck

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

	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
)

// ActionTypeForm holds explicit DSL Payload and Result type arguments to the
// forms the generated code relies on.
var ActionTypeForm = Check{
	Name: "Action type form",
	Rule: "explicit DSL Payload/Result types must be named types declared in the same model package: struct types use the pointer form, slice and map types use the value form, an empty struct type may only pair with an empty peer side, a Payload type is never an interface with methods, and a type defined over another named type (type A B) gives way to an alias (type A = B) or a struct of its own",
	run:  checkActionTypeForm,
}

// checkActionTypeForm checks the explicit DSL Payload and Result type
// declarations of every model package. The type argument must be a named type
// declared in the same package; struct types must use the pointer form, slice
// and map types must use the value form, and an empty struct type is the
// delegation marker for actions without data, so it may only pair with an
// empty peer side (an omitted side counts as empty). A Payload type must not
// be an interface with methods, which no request body decodes into, named by
// value or through a pointer; a Result is only encoded, so any interface
// serves there. A type defined over another named type, type SampleGetRsp
// SampleRsp, is refused: it is the shape of that type under a second
// identity, which costs a conversion at every use and a message and schema
// of its own in the generated artifacts for nothing an alias, type
// SampleGetRsp = SampleRsp, does not give; an action whose shape differs
// declares a struct of its own.
func checkActionTypeForm(ignore gghelper.ProjectIgnore) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return violations
	}

	// Files are grouped per directory because the declared types may live in a
	// different file of the package that references them.
	var packageDirs []string
	packageFiles := make(map[string][]string)
	err := ignore.Walk(ggconst.DirModel, func(path string, info os.FileInfo) error {
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
	actionTypeMethodInterface
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
	// be resolved across files, and the types defined over another named type
	// (type A B, not an alias), by the text of that type.
	typeExprs := make(map[string]ast.Expr)
	definedOver := make(map[string]string)
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
				if over, ok := namedTypeText(typeSpec.Type); ok && !typeSpec.Assign.IsValid() {
					definedOver[typeSpec.Name.Name] = over
				}
			}
		}
	}
	// A type defined over a name the package does not declare, a builtin or
	// a dot-imported type, is reported as undeclared below, not here.
	for name, over := range definedOver {
		if _, declared := typeExprs[over]; !declared && !strings.Contains(over, ".") {
			delete(definedOver, name)
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
			case *ast.InterfaceType:
				if interfaceDeclaresMethods(t, typeExprs, make(map[string]bool)) {
					return actionTypeMethodInterface
				}
				return actionTypeOther
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
		relPath := gghelper.RelativePath(file.path)

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
		designs := dsl.Parse(file.node)
		for _, modelName := range slices.Sorted(maps.Keys(designs)) {
			designs[modelName].Range(func(_ string, action *dsl.Action) {
				violations = append(violations, checkActionTypePair(relPath, action, resolve, definedOver)...)
			})
		}
	}

	return violations
}

// namedTypeText returns the text of expr when it names a type, SampleRsp or
// shared.SyncRsp, the shapes a defined type is written over; false for any
// other form, a struct, slice, map, pointer or interface literal.
func namedTypeText(expr ast.Expr) (string, bool) {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name, true
	case *ast.SelectorExpr:
		if pkg, ok := x.X.(*ast.Ident); ok && x.Sel != nil {
			return pkg.Name + "." + x.Sel.Name, true
		}
	}
	return "", false
}

// interfaceDeclaresMethods reports whether the interface expr declares a
// method, itself or through an interface it embeds. An embedded name the
// package declares is followed through typeExprs, the package's type
// declarations. An embedded interface of another package, such as
// fmt.Stringer, or one dot-imported, cannot be read here and counts as
// declaring methods, as do error and any embedded form besides a name or an
// interface literal; any declares none. For
//
//	type SampleBinder interface{ Bind() }
//	type SampleReq interface{ SampleBinder }
//	type SampleAny = interface{}
//	type SampleOpenReq interface{ SampleAny }
//
// it reports true for the type of SampleReq and false for that of
// SampleOpenReq. seen guards the names already followed against a cycle.
func interfaceDeclaresMethods(expr ast.Expr, typeExprs map[string]ast.Expr, seen map[string]bool) bool {
	switch t := expr.(type) {
	case *ast.InterfaceType:
		if t.Methods == nil {
			return false
		}
		for _, field := range t.Methods.List {
			if len(field.Names) > 0 || interfaceDeclaresMethods(field.Type, typeExprs, seen) {
				return true
			}
		}
		return false
	case *ast.Ident:
		next, declared := typeExprs[t.Name]
		if !declared {
			return t.Name != "any"
		}
		if seen[t.Name] {
			return false
		}
		seen[t.Name] = true
		return interfaceDeclaresMethods(next, typeExprs, seen)
	case *ast.ParenExpr:
		return interfaceDeclaresMethods(t.X, typeExprs, seen)
	default:
		return true
	}
}

// checkActionTypePair checks the Payload and Result type strings of one
// action against the package type table.
func checkActionTypePair(relPath string, action *dsl.Action, resolve func(string) actionTypeKind, definedOver map[string]string) []string {
	var violations []string

	actionName := action.Phase.Name()
	sideEmpty := func(raw string) bool {
		if raw == dsl.PayloadEmpty {
			return true
		}
		return resolve(strings.TrimPrefix(raw, "*")) == actionTypeEmptyStruct
	}
	bothEmpty := sideEmpty(action.Payload) && sideEmpty(action.Result)

	// A Stream action is told its streaming side as StreamingPayload or
	// StreamingResult (see actionTypeSides).
	for _, side := range actionTypeSides(action) {
		kind, raw := side.kind, side.raw
		name := strings.TrimPrefix(raw, "*")
		pointer := strings.HasPrefix(raw, "*")

		if over, defined := definedOver[name]; defined {
			violations = append(violations, fmt.Sprintf(
				"%s: %s action declares %s[%s] whose type is defined over %s; share the shape through an alias, type %s = %s, or declare a struct type of its own",
				relPath, actionName, kind, raw, over, name, over,
			))
			continue
		}
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
		case actionTypeMethodInterface:
			// A Result is only encoded, and any value encodes; a request
			// body decodes into no interface with methods, streamed or not.
			if dsl.PayloadKeyword(kind) {
				violations = append(violations, fmt.Sprintf(
					"%s: %s action declares %s[%s] whose type is an interface with methods, which no request body decodes into; declare a struct type and use the pointer form %s[*%s]",
					relPath, actionName, kind, raw, kind, name,
				))
			}
		case actionTypeOther:
			// The underlying type cannot be classified inside this package
			// (for example an alias to another package's type), or takes no
			// particular form (an interface without methods, which holds any
			// JSON value); no form verdict applies.
		}
	}

	return violations
}
