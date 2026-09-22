package ts

import (
	"cmp"
	"fmt"
	"go/types"
	"reflect"
	"slices"
	"strings"
	"unicode"
)

// This file holds the encoding/json rules the declarations follow: which keys
// a struct encodes to, which values may be null, and which types decide their
// encoding through methods of their own.

// jsonField is one key of the JSON object a struct encodes to.
type jsonField struct {
	key   string
	field *types.Var
	// omit is set by the omitempty or omitzero tag option.
	omit bool
	// quoted is set by the string tag option on a field it applies to: the
	// value travels as a JSON string.
	quoted bool
	// viaPointer marks a key promoted through an embedded pointer, which a nil
	// pointer drops together with every other key of the embedded struct.
	viaPointer bool
	tagged     bool
	index      []int
}

// jsonFields resolves the keys of the JSON object st encodes to, the way
// encoding/json resolves them: the fields of embedded structs are promoted
// breadth first; a key defined at several depths belongs to the shallowest
// field; a tie at one depth goes to the only tagged field and otherwise drops
// the key; keys keep the order of the fields they come from. So a model
// embedding model.Base first encodes to id, created_by, updated_by,
// created_at and updated_at, then to the keys of its own fields.
func (g *generator) jsonFields(st *types.Struct, s site) []jsonField {
	type level struct {
		st         *types.Struct
		key        string // the embedded struct type; empty for st itself
		index      []int
		viaPointer bool
	}
	var fields []jsonField
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
				if p, ok := elem.(*types.Pointer); ok {
					elem, isPointer = types.Unalias(p.Elem()), true
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
				fieldSite := g.fieldSite(s, f.Name(), f)
				name, opts, problem := parseJSONTag(tag)
				if problem != "" {
					g.report(fieldSite, "%s", problem)
				}
				index := append(slices.Clone(lv.index), i)
				if name != "" || !f.Embedded() || !elemIsStruct {
					field := jsonField{
						key:        cmp.Or(name, f.Name()),
						field:      f,
						omit:       opts.omitEmpty || opts.omitZero,
						quoted:     opts.quoted && quotable(f.Type()),
						viaPointer: lv.viaPointer,
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
					g.report(fieldSite, "encoding/json cannot allocate the unexported struct %s embedded through a pointer, so a request carrying its keys fails to decode; embed it by value or export the type", elem)
				}
				key := types.TypeString(elem, nil)
				nextCount[key]++
				if nextCount[key] == 1 {
					next = append(next, level{st: elemStruct, key: key, index: index, viaPointer: lv.viaPointer || isPointer})
				}
			}
		}
	}

	slices.SortStableFunc(fields, func(a, b jsonField) int {
		return cmp.Or(
			strings.Compare(a.key, b.key),
			cmp.Compare(len(a.index), len(b.index)),
			compareTagged(a, b),
			slices.Compare(a.index, b.index),
		)
	})
	dominant := make([]jsonField, 0, len(fields))
	for i := 0; i < len(fields); {
		j := i + 1
		for j < len(fields) && fields[j].key == fields[i].key {
			j++
		}
		if j-i == 1 || len(fields[i].index) != len(fields[i+1].index) || fields[i].tagged != fields[i+1].tagged {
			dominant = append(dominant, fields[i])
		}
		i = j
	}
	slices.SortFunc(dominant, func(a, b jsonField) int { return slices.Compare(a.index, b.index) })
	return dominant
}

// compareTagged orders a tagged field before an untagged one.
func compareTagged(a, b jsonField) int {
	switch {
	case a.tagged == b.tagged:
		return 0
	case a.tagged:
		return -1
	default:
		return 1
	}
}

// jsonTagOptions are the json tag options the declarations take into account.
type jsonTagOptions struct {
	omitEmpty bool
	omitZero  bool
	quoted    bool
}

// parseJSONTag splits a json struct tag into its key name and options: name
// and omitempty for name,omitempty, count and string for count,string, and
// the key - for -,. The problem it reports is a tag that encoding/json and the
// JSON v2 experiment read differently, so no single TypeScript shape
// describes it: a name encoding/json rejects, or an option other than
// omitempty, omitzero and string, such as inline in name,inline. A rejected
// name comes back empty, as encoding/json then falls back to the Go field
// name.
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

// builtinKind is the JSON shape of a type from outside the project whose
// encoding methods produce a known shape.
type builtinKind int

const (
	builtinString         builtinKind = iota + 1 // a JSON string
	builtinNumber                                // a JSON number
	builtinAny                                   // raw JSON of any shape, null included
	builtinObject                                // an object, or null for a nil map
	builtinNullableString                        // a string, or null when unset
	builtinWrapper                               // the encoding of the single type argument
)

// builtins lists the types with encoding methods whose output is known, keyed
// by package path and type name. The JSON v2 experiment turns json.RawMessage
// into an alias of jsontext.Value, so both names are listed.
var builtins = map[string]builtinKind{
	"time.Time":                    builtinString,
	"encoding/json.Number":         builtinNumber,
	"encoding/json.RawMessage":     builtinAny,
	"encoding/json/jsontext.Value": builtinAny,
	"gorm.io/datatypes.Date":       builtinString,
	"gorm.io/datatypes.Time":       builtinString,
	"gorm.io/datatypes.JSON":       builtinAny,
	"gorm.io/datatypes.JSONMap":    builtinObject,
	"gorm.io/datatypes.JSONType":   builtinWrapper,
	"gorm.io/gorm.DeletedAt":       builtinNullableString,
}

// builtinOf reports the builtin shape of n, if it has one.
func builtinOf(n *types.Named) (builtinKind, bool) {
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
// structure, which is what the declaration describes, and whatever else a
// request may send is up to the decoding method.
var marshalMethods = []string{"MarshalJSON", "MarshalJSONTo", "MarshalText", "AppendText"}

// keyMarshalMethods are the methods through which a map key type writes its
// key.
var keyMarshalMethods = []string{"MarshalText", "AppendText"}

// method returns the first of names in the method set of *t, which holds the
// methods declared on t and the ones promoted to it as well.
func (g *generator) method(t types.Type, names []string) string {
	mset := g.methodSets.MethodSet(types.NewPointer(t))
	for _, name := range names {
		if mset.Lookup(nil, name) != nil {
			return name
		}
	}
	return ""
}

// nilable reports whether a value of t can be nil: a pointer, slice, map or
// interface, such as *string, []string or any, but not string. A request may
// leave such a field out, and a nil value reaches the client as null or not at
// all.
func nilable(t types.Type) bool {
	switch types.Unalias(t).Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Interface:
		return true
	default:
		return false
	}
}

// nullable reports whether a value of t may encode as null, as a *string,
// []string or map value may. Raw JSON and interface values are left out:
// their TypeScript type, unknown, admits null already.
func nullable(t types.Type) bool {
	t = types.Unalias(t)
	if n, ok := t.(*types.Named); ok {
		if kind, isBuiltin := builtinOf(n); isBuiltin {
			switch kind {
			case builtinObject, builtinNullableString:
				return true
			case builtinWrapper:
				return nullable(n.TypeArgs().At(0))
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
