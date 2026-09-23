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

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
)

// JSONTagNaming holds the json tags of models and of explicit DSL Payload and
// Result types to snake_case.
var JSONTagNaming = Check{
	Name: "JSON tag naming",
	Rule: "model struct and explicit DSL Payload/Result type json tags must use snake_case naming",
	run:  checkJSONTagNaming,
}

// checkJSONTagNaming checks that json tags declared under the model directory
// use snake_case naming. It covers model structs and the explicit DSL Payload
// and Result types referenced by Design methods.
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

// structJSONTagViolations reports the fields of one struct whose json tags are
// not snake_case.
func structJSONTagViolations(relPath string, structType *ast.StructType) []string {
	var violations []string

	for _, field := range structType.Fields.List {
		if field.Tag == nil {
			continue
		}
		tagValue := strings.Trim(field.Tag.Value, "`")
		jsonTag := extractJSONTag(tagValue)
		if jsonTag == "" || isSnakeCase(jsonTag) {
			continue
		}

		fieldName := ""
		if len(field.Names) > 0 {
			fieldName = field.Names[0].Name
		}
		violations = append(violations, fmt.Sprintf(
			"%s: field '%s' json tag '%s' should be '%s'",
			relPath, fieldName, jsonTag, toSnakeCase(jsonTag),
		))
	}

	return violations
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

// toSnakeCase converts camelCase or kebab-case to snake_case: userId and
// user-id both give user_id.
func toSnakeCase(s string) string {
	if s == "" {
		return s
	}

	// Replace hyphens with underscores
	s = strings.ReplaceAll(s, "-", "_")

	// Convert camelCase to snake_case
	var result strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				result.WriteRune('_')
			}
			result.WriteRune(r - 'A' + 'a')
		} else {
			result.WriteRune(r)
		}
	}

	return result.String()
}
