package ggcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/stoewer/go-strcase"
)

// JSONTagNaming holds every exported field of models and of explicit DSL
// Payload and Result types to a snake_case json tag.
var JSONTagNaming = Check{
	Name: "JSON tag naming",
	Rule: "model struct and explicit DSL Payload/Result type fields must carry snake_case json tags; an exported field without a json key would be served under its Go name",
	run:  checkJSONTagNaming,
}

// checkJSONTagNaming checks that the structs declared under the model
// directory carry snake_case json tags on every exported field. It covers
// model structs and the explicit DSL Payload and Result types referenced by
// Design methods.
func checkJSONTagNaming(ignore gghelper.ProjectIgnore) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return violations
	}

	// Files are grouped per directory because an explicit DSL action type may
	// be declared in a different file of the package that references it.
	var packageDirs []string
	packageFiles := make(map[string][]string)
	err := ignore.Walk(ggconst.DirModel, func(path string, info os.FileInfo) error {
		// Skip directories and non-Go files
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip framework-generated files.
		if isGeneratedFileName(path) {
			return nil
		}

		violations = append(violations, checkFileModelJSONTagNaming(path)...)

		if strings.HasSuffix(path, "_test.go") {
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
		violations = append(violations, checkPackageActionTypeJSONTagNaming(packageFiles[dir])...)
	}

	return violations
}

// checkFileModelJSONTagNaming checks json tags of the model structs in one file.
func checkFileModelJSONTagNaming(filePath string) []string {
	var violations []string

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	// Find all model structs in this file
	modelBaseNames := dsl.FindAllModelBase(node)
	modelEmptyNames := dsl.FindAllModelEmpty(node)
	allModelNames := slices.Concat(modelBaseNames, modelEmptyNames)

	// If no model structs found, skip this file
	if len(allModelNames) == 0 {
		return violations
	}

	relPath := gghelper.RelativePath(filePath)

	// Check only model structs
	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			// Check if this struct is a model
			if !slices.Contains(allModelNames, typeSpec.Name.Name) {
				continue
			}

			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok || structType.Fields == nil {
				continue
			}

			violations = append(violations, structJSONTagViolations(relPath, structType)...)
		}
	}

	return violations
}

// checkPackageActionTypeJSONTagNaming checks json tags of the explicit DSL
// Payload and Result types declared in one model package. Only types
// referenced by a Design method and declared in the same package are checked:
// model structs are already covered by checkFileModelJSONTagNaming, and
// unreferenced structs, such as DTOs mirroring an external wire contract, must
// keep their own naming.
func checkPackageActionTypeJSONTagNaming(paths []string) []string {
	var violations []string

	fset := token.NewFileSet()
	files := make(map[string]*ast.File, len(paths))
	for _, path := range paths {
		node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			continue
		}
		files[path] = node
	}

	// Model structs are covered by the per-file model check already.
	covered := make(map[string]bool)
	for _, node := range files {
		for _, name := range slices.Concat(dsl.FindAllModelBase(node), dsl.FindAllModelEmpty(node)) {
			covered[name] = true
		}
	}

	// Collect the same-package type names referenced as Payload[T] or Result[T].
	referenced := make(map[string]bool)
	for _, node := range files {
		for _, decl := range node.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Name == nil || funcDecl.Name.Name != "Design" || funcDecl.Recv == nil || funcDecl.Body == nil {
				continue
			}
			ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				_, typeExpr, ok := dslActionTypeCall(call.Fun)
				if !ok {
					return true
				}
				if name, ok := localActionTypeName(typeExpr); ok && !covered[name] {
					referenced[name] = true
				}
				return true
			})
		}
	}
	if len(referenced) == 0 {
		return violations
	}

	// Check declarations of the referenced types in file walk order.
	for _, path := range paths {
		node, ok := files[path]
		if !ok {
			continue
		}
		relPath := gghelper.RelativePath(path)
		for _, decl := range node.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}
			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || !referenced[typeSpec.Name.Name] {
					continue
				}

				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok || structType.Fields == nil {
					continue
				}

				violations = append(violations, structJSONTagViolations(relPath, structType)...)
			}
		}
	}

	return violations
}

// structJSONTagViolations reports the fields of one struct that would be
// served under a key that is not snake_case: an exported field without a
// json key, served under its Go name, and a field whose json key is not
// snake_case. An embedded field without a json key promotes fields carrying
// keys of their own, an unexported field never reaches JSON, and json:"-"
// keeps a field off the wire, so none of those is reported; an embedded
// field given a json key is served under that key, like any other field.
func structJSONTagViolations(relPath string, structType *ast.StructType) []string {
	var violations []string

	for _, field := range structType.Fields.List {
		jsonTag := ""
		if field.Tag != nil {
			jsonTag = extractJSONTag(strings.Trim(field.Tag.Value, "`"))
		}
		if len(field.Names) == 0 {
			if jsonTag != "" && !isSnakeCase(jsonTag) {
				violations = append(violations, fmt.Sprintf(
					"%s: embedded field '%s' json tag '%s' should be '%s'",
					relPath, embeddedTypeName(field.Type), jsonTag, strcase.SnakeCase(jsonTag),
				))
			}
			continue
		}
		for _, name := range field.Names {
			switch {
			case jsonTag == "":
				if ast.IsExported(name.Name) {
					violations = append(violations, fmt.Sprintf(
						"%s: field '%s' has no json key, so its key is '%s'; tag it json:%q, json:\"-\" to keep it off the wire, or unexport it",
						relPath, name.Name, name.Name, strcase.SnakeCase(name.Name),
					))
				}
			case !isSnakeCase(jsonTag):
				violations = append(violations, fmt.Sprintf(
					"%s: field '%s' json tag '%s' should be '%s'",
					relPath, name.Name, jsonTag, strcase.SnakeCase(jsonTag),
				))
			}
		}
	}

	return violations
}

// embeddedTypeName names the type an embedded field embeds, the way the
// field is referred to: Inner for Inner, *Inner and model.Inner alike.
func embeddedTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return embeddedTypeName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return embeddedTypeName(t.X)
	case *ast.IndexListExpr:
		return embeddedTypeName(t.X)
	}
	return ""
}

// jsonTagPattern matches the json key of a struct tag, capturing its value.
var jsonTagPattern = regexp.MustCompile(`json:"([^"]+)"`)

// extractJSONTag extracts the json name from a struct tag, leaving its
// options out: `gorm:"column:user_id" json:"userId,omitempty"` gives userId,
// and a tag without a json key gives "".
func extractJSONTag(tag string) string {
	matches := jsonTagPattern.FindStringSubmatch(tag)
	if len(matches) > 1 {
		// Remove options like omitempty
		parts := strings.Split(matches[1], ",")
		return parts[0]
	}
	return ""
}

// isSnakeCase reports whether a json name is in snake_case: user_id is, while
// userId and user-id are not. The json:"-" marker and single characters
// pass.
func isSnakeCase(s string) bool {
	if s == "" {
		return true
	}

	// Skip special cases like "-" or single characters
	if s == "-" || len(s) == 1 {
		return true
	}

	// Check if it contains hyphens (kebab-case) or uppercase letters
	if strings.Contains(s, "-") {
		return false
	}

	// Check for uppercase letters (not snake_case)
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			return false
		}
	}

	return true
}
