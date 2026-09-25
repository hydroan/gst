package jsonshape

import (
	"go/constant"
	"go/types"
)

// Enum describes a named string or integer type of the project whose package
// declares constants of it. Its JSON values are those constants, and the zero
// value of a variable never assigned one.
type Enum struct {
	// Values are the constants, one per distinct value, in source order.
	Values []EnumValue
	// Bitwise marks a bit set: a constant built with a bitwise operator means
	// any combination of the constants may appear, so the type is a number.
	Bitwise bool
	// CoversZero reports whether one of the constants has the zero value of
	// the type; otherwise a variable never assigned a constant holds a value
	// the constants do not list.
	CoversZero bool
}

// EnumValue is one constant of an enum type.
type EnumValue struct {
	Const *types.Const
	Doc   string
}

// Enum returns the enum description of obj, or nil when obj is no enum type:
// an alias, a type from outside the project, a type that is not a string or
// integer, or one whose package declares no constant of it.
func (p *Project) Enum(obj *types.TypeName) *Enum {
	if e, ok := p.enums[obj]; ok {
		return e
	}
	e := p.buildEnum(obj)
	p.enums[obj] = e
	return e
}

// buildEnum describes obj as an enum type, or returns nil when it is none.
func (p *Project) buildEnum(obj *types.TypeName) *Enum {
	if obj.IsAlias() || obj.Pkg() == nil {
		return nil
	}
	source := p.sources[obj.Pkg().Path()]
	basic, isBasic := obj.Type().Underlying().(*types.Basic)
	if source == nil || !isBasic || basic.Info()&(types.IsString|types.IsInteger) == 0 {
		return nil
	}
	constants := source.constants[obj]
	if len(constants) == 0 {
		return nil
	}
	e := &Enum{}
	seen := make(map[string]bool)
	for _, c := range constants {
		e.Bitwise = e.Bitwise || c.bitwise
		val := c.obj.Val()
		e.CoversZero = e.CoversZero || isZeroValue(val)
		key := val.ExactString()
		if seen[key] {
			continue
		}
		seen[key] = true
		e.Values = append(e.Values, EnumValue{Const: c.obj, Doc: c.doc})
	}
	return e
}

// isZeroValue reports whether a string or integer constant value is the zero
// value of its type: the empty string, or 0.
func isZeroValue(val constant.Value) bool {
	switch val.Kind() {
	case constant.String:
		return constant.StringVal(val) == ""
	case constant.Int:
		return constant.Sign(val) == 0
	default:
		return false
	}
}

// CheckForeignConstants reports constants of an enum type declared outside
// the type's package, for every enum Enum described so far. The description
// of the type lists the constants of its own package only, so such a constant
// could send a value the description refuses.
func (p *Project) CheckForeignConstants() {
	for obj, e := range p.enums {
		if e == nil {
			continue
		}
		for pkgPath, source := range p.sources {
			if pkgPath == obj.Pkg().Path() {
				continue
			}
			for _, c := range source.constants[obj] {
				p.Report(Site{Subject: pkgPath + "." + c.obj.Name(), Pos: c.obj.Pos()},
					"the constant has type %s.%s but is declared in another package, so the declaration of %s does not list its value; declare it next to its type",
					obj.Pkg().Path(), obj.Name(), obj.Name())
			}
		}
	}
}
