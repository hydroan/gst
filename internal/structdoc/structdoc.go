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
	"strconv"
	"strings"

	"github.com/hydroan/gst/apidoc"
)

// Docs holds everything extracted from one Go source file.
type Docs struct {
	// Structs maps a struct type name to its doc comments. Structs without
	// any struct or field comment are omitted.
	Structs map[string]apidoc.StructDoc
	// Enums maps an enum-like named type (eg. `type Status string`) to its
	// doc comment and declared constant values. An entry may carry only the
	// comment (type declared here, constants elsewhere) or only values
	// (constants declared here, type elsewhere); callers merge entries of
	// the same package before use.
	Enums map[string]apidoc.EnumDoc
}

// enumBaseKinds are the underlying basic types accepted for enum-like named
// type declarations.
var enumBaseKinds = map[string]bool{
	"string": true,
	"int":    true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
}

// ParseFile reads filename and returns the doc comments of every struct
// declared in it, keyed by struct type name.
func ParseFile(filename string) (map[string]apidoc.StructDoc, error) {
	docs, err := ParseFileDocs(filename)
	if err != nil {
		return nil, err
	}
	return docs.Structs, nil
}

// ParseFileDocs reads filename and returns the struct and enum doc comments
// declared in it.
func ParseFileDocs(filename string) (Docs, error) {
	src, err := os.ReadFile(filename) // #nosec G304 -- callers pass trusted source file paths
	if err != nil {
		return Docs{}, err
	}
	return ParseSourceDocs(filename, src)
}

// ParseSource parses Go source code and returns the doc comments of every
// struct declared in it, keyed by struct type name. Structs without any
// struct or field comment are omitted. The filename is only used for error
// positions.
func ParseSource(filename string, src []byte) (map[string]apidoc.StructDoc, error) {
	docs, err := ParseSourceDocs(filename, src)
	if err != nil {
		return nil, err
	}
	return docs.Structs, nil
}

// ParseSourceDocs parses Go source code and returns the struct and enum doc
// comments declared in it. The filename is only used for error positions.
func ParseSourceDocs(filename string, src []byte) (Docs, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return Docs{}, err
	}

	docs := Docs{
		Structs: make(map[string]apidoc.StructDoc),
		Enums:   make(map[string]apidoc.EnumDoc),
	}
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		switch genDecl.Tok {
		case token.TYPE:
			parseTypeDecl(genDecl, &docs)
		case token.CONST:
			parseConstDecl(genDecl, docs.Enums)
		}
	}

	return docs, nil
}

// parseTypeDecl collects struct doc comments and enum-like named type doc
// comments from a type declaration.
func parseTypeDecl(genDecl *ast.GenDecl, docs *Docs) {
	for _, spec := range genDecl.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}

		switch typ := typeSpec.Type.(type) {
		case *ast.StructType:
			doc := apidoc.StructDoc{
				Comment: structComment(genDecl, typeSpec),
				Fields:  fieldComments(typ),
			}
			if doc.Comment == "" && len(doc.Fields) == 0 {
				continue
			}
			docs.Structs[typeSpec.Name.Name] = doc
		case *ast.Ident:
			// A named basic type is an enum candidate; its constant values
			// may live in the same or another file of the package.
			if typeSpec.Assign.IsValid() || !enumBaseKinds[typ.Name] {
				continue
			}
			enum := docs.Enums[typeSpec.Name.Name]
			enum.Comment = structComment(genDecl, typeSpec)
			docs.Enums[typeSpec.Name.Name] = enum
		}
	}
}

// parseConstDecl collects the constants of enum-like named types from a
// const declaration. Within one const block the named type carries over to
// bare iota continuation lines, matching Go type inference for constants.
func parseConstDecl(genDecl *ast.GenDecl, enums map[string]apidoc.EnumDoc) {
	currentType := ""
	for specIndex, spec := range genDecl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		switch {
		case valueSpec.Type != nil:
			ident, ok := valueSpec.Type.(*ast.Ident)
			if !ok || enumBaseKinds[ident.Name] {
				// A qualified or builtin type is not a local enum type.
				currentType = ""
				continue
			}
			currentType = ident.Name
		case len(valueSpec.Values) > 0:
			// An untyped constant with an explicit value does not belong to
			// the preceding enum type.
			currentType = ""
		}
		if currentType == "" {
			continue
		}

		comment := ""
		if valueSpec.Doc != nil && len(valueSpec.Doc.List) > 0 {
			comment = ExtractCommentText(valueSpec.Doc)
		} else if valueSpec.Comment != nil && len(valueSpec.Comment.List) > 0 {
			comment = ExtractCommentText(valueSpec.Comment)
		}

		for nameIndex, name := range valueSpec.Names {
			if name.Name == "_" {
				continue
			}
			value, ok := constValue(valueSpec, nameIndex, specIndex)
			if !ok {
				continue
			}
			enum := enums[currentType]
			enum.Values = append(enum.Values, apidoc.EnumValue{Value: value, Comment: comment})
			enums[currentType] = enum
		}
	}
}

// constValue resolves the literal value of one constant name. specIndex is
// the ValueSpec position inside the const block, which equals the iota value
// for that line.
func constValue(valueSpec *ast.ValueSpec, nameIndex, specIndex int) (any, bool) {
	// Bare continuation line in an iota block.
	if len(valueSpec.Values) == 0 {
		return specIndex, true
	}
	if nameIndex >= len(valueSpec.Values) {
		return nil, false
	}

	switch value := valueSpec.Values[nameIndex].(type) {
	case *ast.BasicLit:
		switch value.Kind {
		case token.STRING:
			text, err := strconv.Unquote(value.Value)
			if err != nil {
				return nil, false
			}
			return text, true
		case token.INT:
			number, err := strconv.Atoi(value.Value)
			if err != nil {
				return nil, false
			}
			return number, true
		}
	case *ast.Ident:
		if value.Name == "iota" {
			return specIndex, true
		}
	}

	// Complex constant expressions (iota arithmetic, bit shifts, ...) are
	// intentionally not evaluated.
	return nil, false
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
