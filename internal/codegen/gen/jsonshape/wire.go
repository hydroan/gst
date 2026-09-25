package jsonshape

import (
	"cmp"
	"fmt"
	"go/types"
	"reflect"
	"slices"
	"strings"
	"unicode"
)

// This file holds the encoding/json rules a shape follows: which keys a
// struct encodes to, which values may be null, which types decide their
// encoding through methods of their own, and how byte slices and map keys
// encode.

// Field is one key of the JSON object a struct encodes to.
type Field struct {
	// Key is the JSON key.
	Key string
	// Var is the struct field the key comes from.
	Var *types.Var
	// Omit is set by the omitempty or omitzero tag option.
	Omit bool
	// Quoted is set by the string tag option on a field it applies to: the
	// value travels as a JSON string.
	Quoted bool
	// ViaPointer marks a key promoted through an embedded pointer, which a nil
	// pointer drops together with every other key of the embedded struct.
	ViaPointer bool

	// tagged marks a key the json tag names, which wins a tie with the keys of
	// untagged fields at the same depth.
	tagged bool
	// index is the path of field indexes from the encoded struct down to the
	// field, as reflect.StructField.Index holds it; its length is the depth of
	// the field.
	index []int
}

// Fields resolves the keys of the JSON object st encodes to, the way
// encoding/json resolves them: the fields of embedded structs are promoted
// breadth first; a key defined at several depths belongs to the shallowest
// field; a tie at one depth goes to the only tagged field and otherwise drops
// the key; keys keep the order of the fields they come from. So a model
// embedding model.Base first encodes to id, created_by, updated_by,
// created_at and updated_at, then to the keys of its own fields. A json tag
// that encoding/json and the JSON v2 experiment read differently is reported
// at the field's site (see FieldSite), and so is an unexported struct
// embedded through a pointer, which a request carrying its keys fails to
// decode into.
func (p *Project) Fields(st *types.Struct, s Site) []Field {
	type level struct {
		st         *types.Struct
		key        string // the embedded struct type; empty for st itself
		index      []int
		viaPointer bool
	}
	var fields []Field
	visited := make(map[string]bool)
	nextCount := make(map[string]int)
	for next := []level{{st: st}}; len(next) > 0; {
		current := next
		next = nil
		// count tallies how often each embedded struct type occurs at the
		// current depth.
		count := nextCount
		nextCount = make(map[string]int)
		for _, lv := range current {
			if lv.key != "" {
				if visited[lv.key] {
					continue
				}
				visited[lv.key] = true
			}
			for i := range lv.st.NumFields() {
				f := lv.st.Field(i)
				elem, isPointer := types.Unalias(f.Type()), false
				if ptr, ok := elem.(*types.Pointer); ok {
					elem, isPointer = types.Unalias(ptr.Elem()), true
				}
				elemStruct, elemIsStruct := elem.Underlying().(*types.Struct)
				if f.Embedded() {
					if !f.Exported() && !elemIsStruct {
						continue
					}
				} else if !f.Exported() {
					continue
				}
				tag := reflect.StructTag(lv.st.Tag(i)).Get("json")
				if tag == "-" {
					continue
				}
				fieldSite := p.FieldSite(s, f.Name(), f)
				name, opts, problem := parseJSONTag(tag)
				if problem != "" {
					p.Report(fieldSite, "%s", problem)
				}
				index := append(slices.Clone(lv.index), i)
				if name != "" || !f.Embedded() || !elemIsStruct {
					field := Field{
						Key:        cmp.Or(name, f.Name()),
						Var:        f,
						Omit:       opts.omitEmpty || opts.omitZero,
						Quoted:     opts.quoted && quotable(f.Type()),
						ViaPointer: lv.viaPointer,
						tagged:     name != "",
						index:      index,
					}
					fields = append(fields, field)
					if count[lv.key] > 1 {
						// The embedded struct occurs more than once at this
						// depth; the duplicate makes its keys cancel out.
						fields = append(fields, field)
					}
					continue
				}
				if isPointer && !f.Exported() {
					p.Report(fieldSite, "encoding/json cannot allocate the unexported struct %s embedded through a pointer, so a request carrying its keys fails to decode; embed it by value or export the type", elem)
				}
				key := types.TypeString(elem, nil)
				nextCount[key]++
				if nextCount[key] == 1 {
					next = append(next, level{st: elemStruct, key: key, index: index, viaPointer: lv.viaPointer || isPointer})
				}
			}
		}
	}

	slices.SortStableFunc(fields, func(a, b Field) int {
		return cmp.Or(
			strings.Compare(a.Key, b.Key),
			cmp.Compare(len(a.index), len(b.index)),
			compareTagged(a, b),
			slices.Compare(a.index, b.index),
		)
	})
	dominant := make([]Field, 0, len(fields))
	for i := 0; i < len(fields); {
		j := i + 1
		for j < len(fields) && fields[j].Key == fields[i].Key {
			j++
		}
		if j-i == 1 || len(fields[i].index) != len(fields[i+1].index) || fields[i].tagged != fields[i+1].tagged {
			dominant = append(dominant, fields[i])
		}
		i = j
	}
	slices.SortFunc(dominant, func(a, b Field) int { return slices.Compare(a.index, b.index) })
	return dominant
}

// compareTagged orders a tagged field before an untagged one.
func compareTagged(a, b Field) int {
	switch {
	case a.tagged == b.tagged:
		return 0
	case a.tagged:
		return -1
	default:
		return 1
	}
}

// jsonTagOptions are the json tag options a shape takes into account.
type jsonTagOptions struct {
	omitEmpty bool
	omitZero  bool
	quoted    bool
}

// parseJSONTag splits a json struct tag into its key name and options: name
// and omitempty for name,omitempty, count and string for count,string, and
// the key - for -,. The problem it reports is a tag that encoding/json and the
// JSON v2 experiment read differently, so no single shape describes it: a
// name encoding/json rejects, or an option other than omitempty, omitzero and
// string, such as inline in name,inline. A rejected name comes back empty, as
// encoding/json then falls back to the Go field name.
func parseJSONTag(tag string) (string, jsonTagOptions, string) {
	name, rest, _ := strings.Cut(tag, ",")
	var (
		opts    jsonTagOptions
		problem string
	)
	if name != "" && !validTagName(name) {
		problem = fmt.Sprintf("json tag name %q is read differently with and without the JSON v2 experiment; keep to letters, digits, spaces and !#$%%&()*+-./:;<=>?@[]^_{|}~", name)
		name = ""
	}
	for rest != "" {
		var opt string
		opt, rest, _ = strings.Cut(rest, ",")
		switch opt {
		case "omitempty":
			opts.omitEmpty = true
		case "omitzero":
			opts.omitZero = true
		case "string":
			opts.quoted = true
		case "":
		default:
			if problem == "" {
				problem = fmt.Sprintf("json tag option %q is read differently with and without the JSON v2 experiment; keep to omitempty, omitzero and string", opt)
			}
		}
	}
	return name, opts, problem
}

// validTagName reports whether encoding/json accepts a non-empty name as a key
// name: letters, digits, spaces and the punctuation !#$%&()*+-./:;<=>?@[]^_{|}~,
// so trace_id and created at pass while a name holding a quote or a
// backslash does not.
func validTagName(name string) bool {
	for _, c := range name {
		switch {
		case strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", c):
		case !unicode.IsLetter(c) && !unicode.IsDigit(c):
			return false
		}
	}
	return true
}

// quotable reports whether the string tag option applies to a field of type t:
// encoding/json quotes booleans, numbers and strings, held directly or through
// one unnamed pointer, as with int64 and *int, but not []string.
func quotable(t types.Type) bool {
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		t = types.Unalias(p.Elem())
	}
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&(types.IsBoolean|types.IsInteger|types.IsFloat|types.IsString) != 0
}

// Builtin is the JSON shape of a type from outside the project whose encoding
// methods produce a known shape.
type Builtin int

const (
	BuiltinString         Builtin = iota + 1 // a JSON string
	BuiltinNumber                            // a JSON number
	BuiltinAny                               // raw JSON of any shape, null included
	BuiltinObject                            // an object, or null for a nil map
	BuiltinNullableString                    // a string, or null when unset
	BuiltinWrapper                           // the encoding of the single type argument
)

// builtins lists the types with encoding methods whose output is known, keyed
// by package path and type name. The JSON v2 experiment turns json.RawMessage
// into an alias of jsontext.Value, so both names are listed.
var builtins = map[string]Builtin{
	"time.Time":                    BuiltinString,
	"encoding/json.Number":         BuiltinNumber,
	"encoding/json.RawMessage":     BuiltinAny,
	"encoding/json/jsontext.Value": BuiltinAny,
	"gorm.io/datatypes.Date":       BuiltinString,
	"gorm.io/datatypes.Time":       BuiltinString,
	"gorm.io/datatypes.JSON":       BuiltinAny,
	"gorm.io/datatypes.JSONMap":    BuiltinObject,
	"gorm.io/datatypes.JSONType":   BuiltinWrapper,
	"gorm.io/gorm.DeletedAt":       BuiltinNullableString,
}

// BuiltinOf reports the builtin shape of n, if it has one: BuiltinString for
// time.Time, and BuiltinWrapper for datatypes.JSONType[*Options] as for any
// other instantiation of datatypes.JSONType.
func BuiltinOf(n *types.Named) (Builtin, bool) {
	obj := n.Origin().Obj()
	if obj.Pkg() == nil {
		return 0, false
	}
	kind, ok := builtins[obj.Pkg().Path()+"."+obj.Name()]
	return kind, ok
}

// marshalMethods are the methods through which a type writes its own JSON: the
// encoding/json ones, and the one the JSON v2 experiment calls as well.
// Decoding methods do not count: a type with only those still encodes by its
// structure, which is what a shape describes, and whatever else a request may
// send is up to the decoding method.
var marshalMethods = []string{"MarshalJSON", "MarshalJSONTo", "MarshalText", "AppendText"}

// keyMarshalMethods are the methods through which a map key type writes its
// key.
var keyMarshalMethods = []string{"MarshalText", "AppendText"}

// method returns the first of names in the method set of *t, which holds the
// methods declared on t and the ones promoted to it as well.
func (p *Project) method(t types.Type, names []string) string {
	mset := p.methodSets.MethodSet(types.NewPointer(t))
	for _, name := range names {
		if mset.Lookup(nil, name) != nil {
			return name
		}
	}
	return ""
}

// MarshalMethod returns the method through which t writes its own JSON, or ""
// when t encodes by its structure (see marshalMethods).
func (p *Project) MarshalMethod(t types.Type) string { return p.method(t, marshalMethods) }

// IsByteSlice reports whether encoding/json encodes s as a base64 string: a
// slice of bytes whose element type has no marshal methods.
func (p *Project) IsByteSlice(s *types.Slice) bool {
	elem := types.Unalias(s.Elem())
	b, ok := elem.Underlying().(*types.Basic)
	return ok && b.Kind() == types.Uint8 && p.method(elem, marshalMethods) == ""
}

// CheckMapKey reports a map key type without a stable JSON encoding. Keys of
// string and integer types encode as strings; a key type with text marshal
// methods is keyed differently with and without the JSON v2 experiment.
func (p *Project) CheckMapKey(key types.Type, s Site) {
	key = types.Unalias(key)
	if method := p.method(key, keyMarshalMethods); method != "" {
		p.Report(s, "map key type %s declares %s, which encoding/json and the JSON v2 experiment apply to keys differently; use a string or integer key type without it", key, method)
		return
	}
	if b, ok := key.Underlying().(*types.Basic); ok && b.Info()&(types.IsString|types.IsInteger) != 0 {
		return
	}
	p.Report(s, "map key type %s has no JSON encoding; use a string or integer key type", key)
}

// Nilable reports whether a value of t can be nil: a pointer, slice, map or
// interface, such as *string, []string or any, but not string. A request may
// leave such a field out, and a nil value reaches the client as null or not at
// all.
func Nilable(t types.Type) bool {
	switch types.Unalias(t).Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Interface:
		return true
	default:
		return false
	}
}

// Nullable reports whether a value of t may encode as null, as a *string,
// []string or map value may. Raw JSON and interface values are left out:
// their shape admits null already.
func Nullable(t types.Type) bool {
	t = types.Unalias(t)
	if n, ok := t.(*types.Named); ok {
		if kind, isBuiltin := BuiltinOf(n); isBuiltin {
			switch kind {
			case BuiltinObject, BuiltinNullableString:
				return true
			case BuiltinWrapper:
				return Nullable(n.TypeArgs().At(0))
			default:
				return false
			}
		}
	}
	switch t.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map:
		return true
	default:
		return false
	}
}
