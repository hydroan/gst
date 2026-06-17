package openapigen

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
)

// structCommentCache caches parsed struct comments to avoid repeated parsing
var (
	structCommentCache = make(map[string]string)
	structCommentMutex sync.RWMutex
)

// parseModelDocs
// Returns a map to get the comments/documentation for each field of this model
// key is the struct field name, value is the struct field documentation
/*
This is the definition of ast.Field. We need to parse two parts of struct fields: Doc and Comment
If Doc exists, Comment is not used; otherwise, Comment is used.
type Field struct {
	Doc     *CommentGroup // associated documentation; or nil
	Names   []*Ident      // field/method/(type) parameter names; or nil
	Type    Expr          // field/method/parameter type; or nil
	Tag     *BasicLit     // field tag; or nil
	Comment *CommentGroup // line comments; or nil
}
*/
func parseModelDocs(t any) map[string]string {
	result := make(map[string]string)

	typ := reflect.TypeOf(t)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	// Get the package path and name of the type
	pkgPath := typ.PkgPath()
	typeName := typ.Name()

	if pkgPath == "" || typeName == "" {
		// Silently handle invalid type cases
		return result
	}

	// Try to find the source file
	sourceFile := findSourceFile(pkgPath, typeName)
	if sourceFile == "" {
		// Silently handle cases where source file is not found to avoid printing too many error messages
		return result
	}

	// Parse the source file
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, sourceFile, nil, parser.ParseComments)
	if err != nil {
		return result
	}

	// Find the target struct
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != typeName {
				continue
			}

			// Check if it's a struct
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}

			// Parse field comments
			for _, field := range structType.Fields.List {
				for _, name := range field.Names {
					var comment string

					// Prioritize Doc, use Comment if Doc doesn't exist
					if field.Doc != nil && len(field.Doc.List) > 0 {
						// Get documentation comments, may be multi-line
						comment = extractCommentText(field.Doc)
					} else if field.Comment != nil && len(field.Comment.List) > 0 {
						// Get line comments
						comment = extractCommentText(field.Comment)
					}

					if comment != "" {
						result[name.Name] = comment
					}
				}
			}
			return result
		}
	}

	return result
}

// findSourceFile finds the source file based on package path and type name
func findSourceFile(pkgPath, typeName string) string {
	// First try to find using go/build package
	pkg, err := build.Import(pkgPath, ".", build.FindOnly)
	if err == nil && pkg.Dir != "" {
		// Search for .go files containing the target type in the package directory
		files, err := filepath.Glob(filepath.Join(pkg.Dir, "*.go"))
		if err == nil {
			// First check non-test files
			for _, file := range files {
				if strings.HasSuffix(file, "_test.go") {
					continue
				}

				// Check if the file contains the target type
				if containsType(file, typeName) {
					return file
				}
			}

			// If not found in non-test files, then check test files
			for _, file := range files {
				if !strings.HasSuffix(file, "_test.go") {
					continue
				}

				// Check if the file contains the target type
				if containsType(file, typeName) {
					return file
				}
			}
		}
	}

	// If go/build fails, try using call stack information
	for i := range 15 {
		_, file, _, ok := runtime.Caller(i)
		if !ok {
			break
		}

		// Check if the file contains the target package path
		if strings.Contains(file, strings.ReplaceAll(pkgPath, "/", string(filepath.Separator))) {
			return file
		}
	}

	return ""
}

// containsType checks if the file contains the specified type definition
func containsType(filename, typeName string) bool {
	content, err := os.ReadFile(filename)
	if err != nil {
		return false
	}

	// Simple string matching to check if it contains type definition
	// This is not a perfect solution, but it works for most cases
	typeDecl := "type " + typeName + " struct"
	return strings.Contains(string(content), typeDecl)
}

// parseStructComment parses the comment of the struct itself (not its fields)
// Returns the struct's documentation comment
// Results are cached to avoid repeated parsing
func parseStructComment(t any) string {
	typ := reflect.TypeOf(t)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	// Get the package path and name of the type
	pkgPath := typ.PkgPath()
	typeName := typ.Name()

	if pkgPath == "" || typeName == "" {
		// Silently handle invalid type cases
		return ""
	}

	// Create cache key
	cacheKey := pkgPath + "." + typeName

	// Check cache first (read lock)
	structCommentMutex.RLock()
	if cached, exists := structCommentCache[cacheKey]; exists {
		structCommentMutex.RUnlock()
		return cached
	}
	structCommentMutex.RUnlock()

	// Try to find the source file
	sourceFile := findSourceFile(pkgPath, typeName)
	if sourceFile == "" {
		// Cache empty result and return
		structCommentMutex.Lock()
		structCommentCache[cacheKey] = ""
		structCommentMutex.Unlock()
		return ""
	}

	// Parse the source file
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, sourceFile, nil, parser.ParseComments)
	if err != nil {
		// Cache empty result and return
		structCommentMutex.Lock()
		structCommentCache[cacheKey] = ""
		structCommentMutex.Unlock()
		return ""
	}

	// Find the target struct
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != typeName {
				continue
			}

			// Check if it's a struct
			_, ok = typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}

			// Get struct comment, prioritize typeSpec.Doc over genDecl.Doc
			var comment string
			if typeSpec.Doc != nil && len(typeSpec.Doc.List) > 0 {
				comment = extractCommentText(typeSpec.Doc)
			} else if genDecl.Doc != nil && len(genDecl.Doc.List) > 0 {
				comment = extractCommentText(genDecl.Doc)
			}

			// Cache the result (write lock)
			structCommentMutex.Lock()
			structCommentCache[cacheKey] = comment
			structCommentMutex.Unlock()

			return comment
		}
	}

	// Cache empty result as well
	structCommentMutex.Lock()
	structCommentCache[cacheKey] = ""
	structCommentMutex.Unlock()

	return ""
}

// extractCommentText extracts text content from comment group
func extractCommentText(commentGroup *ast.CommentGroup) string {
	if commentGroup == nil || len(commentGroup.List) == 0 {
		return ""
	}

	var lines []string
	for _, comment := range commentGroup.List {
		text := comment.Text

		// Handle different types of comments
		if after, ok := strings.CutPrefix(text, "//"); ok {
			// Line comment
			text = after
			text = strings.TrimSpace(text)
			if text != "" {
				lines = append(lines, text)
			}
		} else if strings.HasPrefix(text, "/*") && strings.HasSuffix(text, "*/") {
			// Block comment
			text = strings.TrimPrefix(text, "/*")
			text = strings.TrimSuffix(text, "*/")

			// Handle multi-line block comments, split by lines and clean each line
			blockLines := strings.SplitSeq(text, "\n")
			for line := range blockLines {
				line = strings.TrimSpace(line)
				if line != "" {
					lines = append(lines, line)
				}
			}
		}
	}

	// Merge multi-line comments, separated by spaces
	return strings.Join(lines, " ")
}
