package ts

import (
	"cmp"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strconv"
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

// usesBitwiseOperator reports whether expr applies a bitwise operator.
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

// commentText returns the text of the first non-empty comment group.
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

// enumType describes a named string or integer type of the project whose
// package declares constants of it. Its JSON values are those constants, and
// the zero value of a variable never assigned one.
type enumType struct {
	values []enumValue
	// bitwise marks a bit set: a constant built with a bitwise operator means
	// any combination of the constants may appear, so the type is a number.
	bitwise bool
	// zero is the literal of the zero value when no constant has that value.
	zero string
}

// enumValue is one constant of an enum type.
type enumValue struct {
	literal string
	doc     string
}

// enumOf returns the enum description of obj, or nil when obj is not an enum
// type.
func (g *generator) enumOf(obj *types.TypeName) *enumType {
	if e, ok := g.enums[obj]; ok {
		return e
	}
	e := g.buildEnum(obj)
	g.enums[obj] = e
	return e
}

// buildEnum describes obj as an enum type, or returns nil when it is none.
func (g *generator) buildEnum(obj *types.TypeName) *enumType {
	if obj.IsAlias() || obj.Pkg() == nil {
		return nil
	}
	source := g.sources[obj.Pkg().Path()]
	basic, isBasic := obj.Type().Underlying().(*types.Basic)
	if source == nil || !isBasic || basic.Info()&(types.IsString|types.IsInteger) == 0 {
		return nil
	}
	constants := source.constants[obj]
	if len(constants) == 0 {
		return nil
	}
	e := &enumType{}
	seen := make(map[string]bool)
	coversZero := false
	for _, c := range constants {
		e.bitwise = e.bitwise || c.bitwise
		literal, zero, ok := g.constantLiteral(c.obj)
		if !ok {
			continue
		}
		coversZero = coversZero || zero
		if seen[literal] {
			continue
		}
		seen[literal] = true
		e.values = append(e.values, enumValue{literal: literal, doc: c.doc})
	}
	if !coversZero {
		e.zero = "0"
		if basic.Info()&types.IsString != 0 {
			e.zero = `""`
		}
	}
	return e
}

// maxSafeInteger is the largest integer a JavaScript number holds exactly.
const maxSafeInteger = 1<<53 - 1

// constantLiteral renders the value of c as a TypeScript literal and reports
// whether it is the zero value.
func (g *generator) constantLiteral(c *types.Const) (literal string, zero, ok bool) {
	val := c.Val()
	switch val.Kind() {
	case constant.String:
		s := constant.StringVal(val)
		return quoteString(s), s == "", true
	case constant.Int:
		v, exact := constant.Int64Val(val)
		if !exact || v > maxSafeInteger || v < -maxSafeInteger {
			g.report(site{subject: c.Pkg().Path() + "." + c.Name(), pos: c.Pos()},
				"the value %s is not exactly representable as a JavaScript number", val.ExactString())
			return "", false, false
		}
		return strconv.FormatInt(v, 10), v == 0, true
	default:
		return "", false, false
	}
}

// enumBody renders the right-hand side of the declaration of an enum type: the
// union of its constants, or number for a bit set.
func enumBody(e *enumType) string {
	switch {
	case e.bitwise:
		return "number"
	case len(e.values) == 0:
		// Only a constant already reported leaves an enum without values.
		return "never"
	}
	literals := make([]string, len(e.values))
	for i, v := range e.values {
		literals[i] = v.literal
	}
	return strings.Join(literals, " | ")
}

// enumDoc appends the constants of e to the doc comment of its type, one line
// each with the constant's own comment, as the OpenAPI document lists enum
// values.
func enumDoc(doc string, e *enumType) string {
	lines := make([]string, 0, len(e.values)+3)
	if doc != "" {
		lines = append(lines, doc, "")
	}
	if e.bitwise {
		lines = append(lines, "Any bitwise combination of:")
	}
	for _, v := range e.values {
		line := "- " + v.literal
		if v.doc != "" {
			line += ": " + strings.Join(strings.Fields(v.doc), " ")
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// checkForeignConstants reports constants of an enum type declared outside the
// type's package. The declaration of the type lists the constants of its own
// package only, so such a constant could send a value the declaration refuses.
func (g *generator) checkForeignConstants() {
	for obj, e := range g.enums {
		if e == nil {
			continue
		}
		for pkgPath, source := range g.sources {
			if pkgPath == obj.Pkg().Path() {
				continue
			}
			for _, c := range source.constants[obj] {
				g.report(site{subject: pkgPath + "." + c.obj.Name(), pos: c.obj.Pos()},
					"the constant has type %s.%s but is declared in another package, so the declaration of %s does not list its value; declare it next to its type",
					obj.Pkg().Path(), obj.Name(), obj.Name())
			}
		}
	}
}
