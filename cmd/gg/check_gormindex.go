package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// gormIndexTagKeys are the gorm tag keys that configure an index. unique is
// banned alongside index and uniqueIndex because it creates a unique index
// too and would reopen the side door the ban closes.
var gormIndexTagKeys = map[string]bool{
	"index":       true,
	"uniqueIndex": true,
	"unique":      true,
}

// CheckGormTagIndexBan reports struct fields that configure indexes through
// gorm struct tags. Indexes are declared exclusively through the model's
// Indexes() []model.Index method, which validates the fields, generates the
// names, and feeds bootstrap and gg migrate from one definition; a tag index
// would bypass all of that. Only the primary key stays in tags, declared by
// the framework base structs themselves. Model subtrees owned by copyable
// framework modules are skipped, since copied module code is owned by the
// framework repository.
func CheckGormTagIndexBan(ignore gitignore.Matcher) []string {
	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return nil
	}

	owned, err := copyableModuleOwners()
	if err != nil {
		return []string{fmt.Sprintf("listing copyable framework modules: %v", err)}
	}

	var violations []string
	walkErr := walkProjectDir(modelDir, ignore, func(path string, info os.FileInfo) error {
		if info.IsDir() {
			if moduleOwnedPath(owned, modelDir, path) {
				return filepath.SkipDir
			}
			return nil
		}
		base := filepath.Base(path)
		if !strings.HasSuffix(base, ".go") || strings.HasSuffix(base, "_test.go") || isGeneratedFileName(path) {
			return nil
		}
		violations = append(violations, checkFileGormTagIndexBan(path)...)
		return nil
	})
	if walkErr != nil {
		violations = append(violations, fmt.Sprintf("walking model directory: %v", walkErr))
	}
	return violations
}

// checkFileGormTagIndexBan reports the index-configuring gorm tags in one
// file. A file that fails to parse is reported as a violation so broken code
// cannot slip past the check.
func checkFileGormTagIndexBan(path string) []string {
	var violations []string

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return append(violations, fmt.Sprintf("%s has parse error: %v", relativePath(path), err))
	}
	relPath := relativePath(path)

	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name == nil {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok || structType.Fields == nil {
				continue
			}
			for _, field := range structType.Fields.List {
				key, ok := gormIndexTagKey(field)
				if !ok {
					continue
				}
				pos := fset.Position(field.Pos())
				violations = append(violations, fmt.Sprintf(
					"%s:%d: field '%s.%s' configures an index through the gorm tag (%s); declare it through the model's Indexes() []model.Index method instead",
					relPath, pos.Line, typeSpec.Name.Name, fieldDisplayName(field), key,
				))
			}
		}
	}
	return violations
}

// gormIndexTagKey returns the first banned index key the field's gorm tag
// carries. Keys compare exactly per semicolon-separated segment, so values
// merely containing the word (a comment, a default) never match.
func gormIndexTagKey(field *ast.Field) (string, bool) {
	if field.Tag == nil {
		return "", false
	}
	raw, err := strconv.Unquote(field.Tag.Value)
	if err != nil {
		return "", false
	}
	gormTag, ok := reflect.StructTag(raw).Lookup("gorm")
	if !ok {
		return "", false
	}
	for part := range strings.SplitSeq(gormTag, ";") {
		key, _, _ := strings.Cut(part, ":")
		if gormIndexTagKeys[strings.TrimSpace(key)] {
			return strings.TrimSpace(key), true
		}
	}
	return "", false
}

// fieldDisplayName names a struct field for the violation message: the
// declared name, or the embedded type's base name.
func fieldDisplayName(field *ast.Field) string {
	if len(field.Names) > 0 {
		return field.Names[0].Name
	}
	if name, ok := actionTypeBaseName(field.Type); ok {
		return name
	}
	return "(embedded)"
}
