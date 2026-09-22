package ts

import (
	"go/constant"
	"go/types"
	"strconv"
	"strings"
)

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
// whether it is the zero value: "active" for the string constant active, ""
// and zero for the empty string, 1 for the integer 1. An integer a JavaScript
// number cannot hold exactly is reported, and yields no literal.
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
// union of its constants, as in "active" | "archived", or number for a bit
// set.
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

// enumDoc appends the constants of e to the doc comment of its type, a line
// each, with the constant's comment joined onto that line, as the OpenAPI
// document lists enum values. For the doc "Status is the state of a sample."
// and the constants "active", commented "StatusActive marks a sample" and "in
// use." on two lines, and "archived", without a comment, it returns
//
//	Status is the state of a sample.
//
//	- "active": StatusActive marks a sample in use.
//	- "archived"
//
// A bit set lists its constants under "Any bitwise combination of:", as in
//
//	Any bitwise combination of:
//	- 1
//	- 2
//
// for the constants 1 and 2 of a type without a doc comment.
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

// enumZero returns the zero value literal of the project enum t holds,
// directly or through a pointer, when no constant of the enum covers it: the
// TypeScript literal "" for a string enum without an empty constant, 0 for an
// integer enum starting at 1, and an empty result for an enum that has a
// constant of its zero value or combines its constants bitwise.
func (g *generator) enumZero(t types.Type) string {
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		t = types.Unalias(p.Elem())
	}
	n, ok := t.(*types.Named)
	if !ok || !g.declares(n.Obj()) {
		return ""
	}
	if e := g.enumOf(n.Obj()); e != nil && !e.bitwise {
		return e.zero
	}
	return ""
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
