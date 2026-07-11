// Package structdoc parses Go source code and extracts the doc comments of
// struct declarations and their fields.
//
// It is the single comment-extraction implementation shared by the OpenAPI
// generator (runtime fallback when source files are available), the model
// registry (embedded Base source) and gg code generation (build-time
// extraction of business model comments).
package structdoc

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"

	"github.com/hydroan/gst/apidoc"
)

// ParseFile reads filename and returns the doc comments of every struct
// declared in it, keyed by struct type name.
func ParseFile(filename string) (map[string]apidoc.StructDoc, error) {
	src, err := os.ReadFile(filename) // #nosec G304 -- callers pass trusted source file paths
	if err != nil {
		return nil, err
	}
	return ParseSource(filename, src)
}

// ParseSource parses Go source code and returns the doc comments of every
// struct declared in it, keyed by struct type name. Structs without any
// struct or field comment are omitted. The filename is only used for error
// positions.
func ParseSource(filename string, src []byte) (map[string]apidoc.StructDoc, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	result := make(map[string]apidoc.StructDoc)
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}

			doc := apidoc.StructDoc{
				Comment: structComment(genDecl, typeSpec),
				Fields:  fieldComments(structType),
			}
			if doc.Comment == "" && len(doc.Fields) == 0 {
				continue
			}
			result[typeSpec.Name.Name] = doc
		}
	}

	return result, nil
}

// structComment returns the doc comment of a struct declaration, preferring
// the TypeSpec doc over the surrounding GenDecl doc.
func structComment(genDecl *ast.GenDecl, typeSpec *ast.TypeSpec) string {
	if typeSpec.Doc != nil && len(typeSpec.Doc.List) > 0 {
		return ExtractCommentText(typeSpec.Doc)
	}
	if genDecl.Doc != nil && len(genDecl.Doc.List) > 0 {
		return ExtractCommentText(genDecl.Doc)
	}
	return ""
}

// fieldComments returns the doc comments of named struct fields, preferring
// the field doc comment over the trailing line comment. Fields without any
// comment and embedded fields are omitted.
func fieldComments(structType *ast.StructType) map[string]string {
	fields := make(map[string]string)
	for _, field := range structType.Fields.List {
		var comment string
		if field.Doc != nil && len(field.Doc.List) > 0 {
			comment = ExtractCommentText(field.Doc)
		} else if field.Comment != nil && len(field.Comment.List) > 0 {
			comment = ExtractCommentText(field.Comment)
		}
		if comment == "" {
			continue
		}
		for _, name := range field.Names {
			fields[name.Name] = comment
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

// ExtractCommentText extracts text content from a comment group.
func ExtractCommentText(commentGroup *ast.CommentGroup) string {
	if commentGroup == nil || len(commentGroup.List) == 0 {
		return ""
	}

	var lines []string
	for _, comment := range commentGroup.List {
		text := comment.Text

		if after, ok := strings.CutPrefix(text, "//"); ok {
			lines = append(lines, normalizeLineComment(after))
		} else if strings.HasPrefix(text, "/*") && strings.HasSuffix(text, "*/") {
			text = strings.TrimPrefix(text, "/*")
			text = strings.TrimSuffix(text, "*/")

			blockLines := strings.SplitSeq(text, "\n")
			for line := range blockLines {
				lines = append(lines, normalizeLineComment(line))
			}
		}
	}

	return strings.Join(trimBlankCommentLines(lines), "\n")
}

func normalizeLineComment(text string) string {
	text = strings.TrimRight(text, " \t\r")
	return strings.TrimPrefix(text, " ")
}

func trimBlankCommentLines(lines []string) []string {
	start := 0
	for start < len(lines) && lines[start] == "" {
		start++
	}

	end := len(lines)
	for end > start && lines[end-1] == "" {
		end--
	}

	return lines[start:end]
}
