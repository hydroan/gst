package ts

import (
	"cmp"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/hydroan/gst/internal/structdoc"
	"golang.org/x/tools/go/packages"
)

// sourceIndex holds what generation reads from the syntax of a project
// package: doc comments, and the constants declared with each named type. Doc
// comments are picked the way the OpenAPI document picks them, so a type is
// described the same in both.
type sourceIndex struct {
	typeDocs  map[*types.TypeName]string
	fieldDocs map[*types.Var]string
	constants map[*types.TypeName][]typedConstant
}

// typedConstant is a constant declared with a named type.
type typedConstant struct {
	obj *types.Const
	doc string
	// bitwise marks a value expression, written out or repeated from an
	// earlier line of the const block, that applies a bitwise operator.
	bitwise bool
}

// newSourceIndex indexes the syntax of pkg. The constants of each type are
// kept in source order.
func newSourceIndex(fset *token.FileSet, pkg *packages.Package) *sourceIndex {
	idx := &sourceIndex{
		typeDocs:  make(map[*types.TypeName]string),
		fieldDocs: make(map[*types.Var]string),
		constants: make(map[*types.TypeName][]typedConstant),
	}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			switch genDecl.Tok {
			case token.TYPE:
				idx.addTypes(pkg.TypesInfo, genDecl)
			case token.CONST:
				idx.addConstants(pkg.TypesInfo, genDecl)
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if field, ok := n.(*ast.Field); ok {
				idx.addField(pkg.TypesInfo, field)
			}
			return true
		})
	}
	for _, constants := range idx.constants {
		slices.SortFunc(constants, func(a, b typedConstant) int {
			return comparePositions(fset, a.obj.Pos(), b.obj.Pos())
		})
	}
	return idx
}

// addTypes records the doc comments of a type declaration. As in the OpenAPI
// document, a type without a comment of its own takes the comment of its
// declaration group.
func (idx *sourceIndex) addTypes(info *types.Info, decl *ast.GenDecl) {
	for _, spec := range decl.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		obj, ok := info.Defs[typeSpec.Name].(*types.TypeName)
		if !ok {
			continue
		}
		if doc := commentText(typeSpec.Doc, decl.Doc); doc != "" {
			idx.typeDocs[obj] = doc
		}
	}
}

// addField records the doc comment of a struct field, or its trailing comment
// when it has none.
func (idx *sourceIndex) addField(info *types.Info, field *ast.Field) {
	doc := commentText(field.Doc, field.Comment)
	if doc == "" {
		return
	}
	for _, name := range field.Names {
		if v, ok := info.Defs[name].(*types.Var); ok && v.IsField() {
			idx.fieldDocs[v] = doc
		}
	}
}

// addConstants records the constants of a const declaration that have a named
// type.
func (idx *sourceIndex) addConstants(info *types.Info, decl *ast.GenDecl) {
	var repeated []ast.Expr
	for _, spec := range decl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		// A line without values repeats the values of the line before it.
		values := valueSpec.Values
		if len(values) == 0 {
			values = repeated
		} else {
			repeated = values
		}
		bitwise := slices.ContainsFunc(values, usesBitwiseOperator)
		doc := commentText(valueSpec.Doc, valueSpec.Comment)
		for _, name := range valueSpec.Names {
			c, ok := info.Defs[name].(*types.Const)
			if !ok || name.Name == "_" {
				continue
			}
			named, ok := types.Unalias(c.Type()).(*types.Named)
			if !ok {
				continue
			}
			idx.constants[named.Obj()] = append(idx.constants[named.Obj()], typedConstant{obj: c, doc: doc, bitwise: bitwise})
		}
	}
}

// usesBitwiseOperator reports whether expr applies a bitwise operator: 1 << iota,
// read | write and all &^ write do, iota + 1 and first * 10 do not.
func usesBitwiseOperator(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if binary, ok := n.(*ast.BinaryExpr); ok {
			switch binary.Op {
			case token.SHL, token.OR, token.AND, token.XOR, token.AND_NOT:
				found = true
			}
		}
		return !found
	})
	return found
}

// commentText returns the text of the first non-empty comment group, as in
// the doc comment of a field before its trailing comment.
func commentText(groups ...*ast.CommentGroup) string {
	for _, group := range groups {
		if group != nil && len(group.List) > 0 {
			return structdoc.ExtractCommentText(group)
		}
	}
	return ""
}

// comparePositions orders two positions by file name and offset. Pos values
// alone do not order positions of different files: the loader adds files to
// the file set in whatever order it parses them.
func comparePositions(fset *token.FileSet, a, b token.Pos) int {
	pa, pb := fset.Position(a), fset.Position(b)
	return cmp.Or(strings.Compare(pa.Filename, pb.Filename), cmp.Compare(pa.Offset, pb.Offset))
}
