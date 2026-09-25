package pb

import (
	"cmp"
	"fmt"
	"go/constant"
	"go/types"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gggen/jsonshape"
	"github.com/stoewer/go-strcase"
	"google.golang.org/protobuf/types/descriptorpb"
)

// This file builds the message of a Go struct type: a field per JSON key,
// numbered by the pb tag, typed by the protobuf mapping of the Go type.

// The well-known types the mapping refers to, and the files declaring them.
const (
	wellKnownTimestamp = ".google.protobuf.Timestamp"
	wellKnownValue     = ".google.protobuf.Value"
	wellKnownStruct    = ".google.protobuf.Struct"
	wellKnownFieldMask = ".google.protobuf.FieldMask"

	timestampProto = "google/protobuf/timestamp.proto"
	structProto    = "google/protobuf/struct.proto"
	fieldMaskProto = "google/protobuf/field_mask.proto"
)

// modelRegistryPath is the package declaring the framework's model base, whose
// promoted keys carry the fixed field numbers of BaseFieldNumbers.
const modelRegistryPath = ggconst.ImportPathGst + "/internal/modelregistry"

// fieldType is the protobuf type of one field.
type fieldType struct {
	kind     descriptorpb.FieldDescriptorProto_Type
	typeName string // the fully-qualified message name for a message kind
	repeated bool
	optional bool // proto3 presence for a scalar held through a pointer
	// mapEntry is the synthetic entry message of a map field, nested in the
	// enclosing message.
	mapEntry *descriptorpb.DescriptorProto
	// enum lists the values of a project enum, appended to the comment of
	// the field.
	enum *jsonshape.Enum
}

// buildMessage fills the message of obj with the fields of its struct type.
// The keys are the ones the type encodes to (see jsonshape.Fields); each
// carries the number of its pb tag, or the fixed number of a framework base
// key, and the type of its Go type (see fieldTypeOf).
func (g *generator) buildMessage(obj *types.TypeName) {
	m := g.messages[obj]
	s := jsonshape.Site{Subject: obj.Pkg().Path() + "." + obj.Name(), Pos: obj.Pos()}
	if !g.project.Declares(obj) {
		g.project.Report(s, "the type is declared outside the project, so its fields cannot carry pb tags; use a project type")
		return
	}
	if obj.IsAlias() {
		g.project.Report(s, "the alias cannot become a message; use the type it stands for")
		return
	}
	st, ok := obj.Type().Underlying().(*types.Struct)
	if !ok {
		g.project.Report(s, "only a struct type becomes a message; %s is a %s", obj.Name(), obj.Type().Underlying())
		return
	}
	if method := g.project.MarshalMethod(obj.Type()); method != "" {
		g.project.Report(s, "the type declares %s, so its JSON shape is decided by code the generator cannot read; drop the method or use a type without one", method)
		return
	}
	desc := g.messageOfStruct(m.name, st, m.file, s, []int32{fileMessagesTag, int32Index(len(m.file.messages))})
	m.file.addMessage(desc, g.project.TypeDoc(obj))
}

// numberedField is a field of a message being built, with its comment.
type numberedField struct {
	field   *descriptorpb.FieldDescriptorProto
	comment string
}

// messageOfStruct builds the descriptor of the message named name for the
// struct st, whose fields are declared in file, at the source path prefix
// the comments of its fields are recorded under. The fields are listed by
// number, so the framework's base keys come first.
func (g *generator) messageOfStruct(name string, st *types.Struct, file *protoFile, s jsonshape.Site, prefix []int32) *descriptorpb.DescriptorProto {
	desc := &descriptorpb.DescriptorProto{Name: new(name)}
	fields := g.project.Fields(st, s)
	base := false
	for _, f := range fields {
		base = base || isBaseField(f)
	}
	numbers := make(map[int32]string)
	var numbered []numberedField
	for _, f := range fields {
		fs := g.project.FieldSite(s, f.Key, f.Var)
		if !identifier.MatchString(f.Key) {
			g.project.Report(fs, "the JSON key %q cannot name a protobuf field; name it with a json tag of letters, digits and underscores", f.Key)
			continue
		}
		number := g.fieldNumber(f, base, fs)
		if number == 0 {
			continue
		}
		if previous, taken := numbers[number]; taken {
			g.project.Report(fs, "field number %d is already taken by %s; give each field its own number", number, previous)
			continue
		}
		numbers[number] = f.Key
		ft, ok := g.fieldTypeOf(f.Var.Type(), file, desc, prefix, f.Key, fs)
		if !ok {
			continue
		}
		field := &descriptorpb.FieldDescriptorProto{
			Name:   new(f.Key),
			Number: new(number),
			Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			Type:   ft.kind.Enum(),
		}
		if ft.typeName != "" {
			field.TypeName = new(ft.typeName)
		}
		if ft.repeated {
			field.Label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()
		}
		if ft.optional {
			// A proto3 optional field lives in a synthetic oneof of its own.
			field.Proto3Optional = new(true)
			field.OneofIndex = new(int32Index(len(desc.OneofDecl)))
			desc.OneofDecl = append(desc.OneofDecl, &descriptorpb.OneofDescriptorProto{Name: new(syntheticOneofPrefix + f.Key)})
		}
		numbered = append(numbered, numberedField{field: field, comment: fieldComment(g.project.FieldDoc(f.Var), ft.enum)})
	}
	slices.SortStableFunc(numbered, func(a, b numberedField) int { return cmp.Compare(a.field.GetNumber(), b.field.GetNumber()) })
	for i, nf := range numbered {
		file.comment(append(slices.Clone(prefix), messageFieldsTag, int32Index(i)), nf.comment)
		desc.Field = append(desc.Field, nf.field)
	}
	return desc
}

// isBaseField reports whether f is a key promoted from the framework's model
// base, which carries a fixed field number.
func isBaseField(f jsonshape.Field) bool {
	_, fixed := BaseFieldNumbers[f.Key]
	return fixed && f.Var.Pkg() != nil && f.Var.Pkg().Path() == modelRegistryPath
}

// fieldNumber returns the field number of f: the fixed number of a framework
// base key, or the number its pb tag names, which must be positive, at most
// 536870911, outside the range 19000 to 19999 protobuf reserves, and from
// FirstBusinessFieldNumber on in a message embedding the base. A number that
// fails these is reported and yields 0.
func (g *generator) fieldNumber(f jsonshape.Field, base bool, s jsonshape.Site) int32 {
	if isBaseField(f) {
		return BaseFieldNumbers[f.Key]
	}
	first := int32(1)
	if base {
		first = FirstBusinessFieldNumber
	}
	tag, tagged := reflect.StructTag(f.Tag).Lookup(Tag)
	if !tagged {
		g.project.Report(s, "the field has no pb tag; number it pb:%q with N from %d", "N", first)
		return 0
	}
	number, err := strconv.ParseInt(tag, 10, 32)
	switch {
	case err != nil:
		g.project.Report(s, "the pb tag %q is not a field number; write the number alone, as in pb:%q", tag, strconv.Itoa(int(first)))
	case number < int64(first):
		if base {
			g.project.Report(s, "the pb tag names field number %d, but 1 to %d belong to the framework's base fields; number business fields from %d", number, FirstBusinessFieldNumber-1, first)
		} else {
			g.project.Report(s, "the pb tag names field number %d; field numbers start at 1", number)
		}
	case number > int64(fieldMaxNumber):
		g.project.Report(s, "the pb tag names field number %d, above the largest field number %d", number, fieldMaxNumber)
	case number >= int64(reservedRangeStart) && number <= int64(reservedRangeEnd):
		g.project.Report(s, "the pb tag names field number %d, inside the range %d to %d protobuf reserves", number, reservedRangeStart, reservedRangeEnd)
	default:
		return int32(number)
	}
	return 0
}

// fieldTypeOf maps the Go type t of a field to its protobuf type: bool to
// bool, string to string, int and int64 to int64, the smaller integers to
// int32, unsigned ones to uint64 and uint32, float32 to float and float64 to
// double; time.Time to google.protobuf.Timestamp; a project enum to its
// underlying string or integer type; a JSON document (any, json.RawMessage,
// datatypes.JSON) to google.protobuf.Value and a JSON object
// (map[string]any, datatypes.JSONMap) to google.protobuf.Struct; []byte to
// bytes, a slice or array to repeated, a map to map; a pointer to the type it
// points to, optional when that is a scalar; a project struct to its message
// (queued to be built) and an unnamed struct to a message nested in parent
// under the field's name. Anything else is reported: a nested slice or map, a
// map with a value of those, a map key of the wrong type, an interface with
// methods, a type with encoding methods of its own, a struct from outside the
// project.
func (g *generator) fieldTypeOf(t types.Type, file *protoFile, parent *descriptorpb.DescriptorProto, prefix []int32, key string, s jsonshape.Site) (fieldType, bool) {
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		ft, ok := g.fieldTypeOf(types.Unalias(p.Elem()), file, parent, prefix, key, s)
		if ok && ft.kind != descriptorpb.FieldDescriptorProto_TYPE_MESSAGE && !ft.repeated {
			ft.optional = true
		}
		return ft, ok
	}
	switch u := t.(type) {
	case *types.Named:
		return g.namedFieldType(u, file, parent, prefix, key, s)
	case *types.Basic:
		kind, ok := scalarKind(u)
		if !ok {
			g.project.Report(s, "%s values have no protobuf type", u)
			return fieldType{}, false
		}
		return fieldType{kind: kind}, true
	case *types.Slice:
		if g.project.IsByteSlice(u) {
			return fieldType{kind: descriptorpb.FieldDescriptorProto_TYPE_BYTES}, true
		}
		return g.repeatedOf(u.Elem(), file, parent, prefix, key, s)
	case *types.Array:
		return g.repeatedOf(u.Elem(), file, parent, prefix, key, s)
	case *types.Map:
		return g.mapOf(u, file, parent, prefix, key, s)
	case *types.Interface:
		if u.Empty() {
			file.importOf(structProto)
			return fieldType{kind: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, typeName: wellKnownValue}, true
		}
		g.project.Report(s, "an interface with methods has no protobuf type, the dynamic type decides it; use a concrete type")
		return fieldType{}, false
	case *types.Struct:
		// An unnamed struct becomes a message of its own, nested in the
		// enclosing one under the field's name.
		name := strcase.UpperCamelCase(key)
		nested := g.messageOfStruct(name, u, file, s, append(slices.Clone(prefix), messageNestedTag, int32Index(len(parent.NestedType))))
		parent.NestedType = append(parent.NestedType, nested)
		return fieldType{kind: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, typeName: name}, true
	default:
		g.project.Report(s, "%s values have no protobuf type", t)
		return fieldType{}, false
	}
}

// namedFieldType maps a named type (see fieldTypeOf).
func (g *generator) namedFieldType(n *types.Named, file *protoFile, parent *descriptorpb.DescriptorProto, prefix []int32, key string, s jsonshape.Site) (fieldType, bool) {
	obj := n.Obj()
	if obj.Pkg() != nil && obj.Pkg().Path() == "time" && obj.Name() == "Time" {
		file.importOf(timestampProto)
		return fieldType{kind: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, typeName: wellKnownTimestamp}, true
	}
	if kind, ok := jsonshape.BuiltinOf(n); ok {
		return g.builtinFieldType(kind, n, file, parent, prefix, key, s)
	}
	if g.project.Declares(obj) {
		if n.TypeArgs().Len() > 0 {
			g.project.Report(s, "the generic type %s of the project is not supported; declare a non-generic type for each instantiation the API uses", n)
			return fieldType{}, false
		}
		if method := g.project.MarshalMethod(n); method != "" {
			g.project.Report(s, "type %s declares %s, so its JSON shape is decided by code the generator cannot read; use a type without the method", n, method)
			return fieldType{}, false
		}
		if e := g.project.Enum(obj); e != nil {
			kind, _ := scalarKind(n.Underlying().(*types.Basic)) //nolint:errcheck // An enum has a string or integer underlying type.
			return fieldType{kind: kind, enum: e}, true
		}
		switch n.Underlying().(type) {
		case *types.Struct:
			m := g.messageOf(obj)
			file.importOf(m.file.name)
			return fieldType{kind: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, typeName: m.fullName()}, true
		default:
			// A named slice, map or basic type encodes as its underlying
			// type, which is what the field holds.
			return g.fieldTypeOf(n.Underlying(), file, parent, prefix, key, s)
		}
	}
	if method := g.project.MarshalMethod(n); method != "" {
		g.project.Report(s, "type %s declares %s, so its JSON shape is decided by code the generator cannot read; use a type without the method", n, method)
		return fieldType{}, false
	}
	switch n.Underlying().(type) {
	case *types.Struct:
		g.project.Report(s, "type %s is declared outside the project, so its fields cannot carry pb tags; use a project type", n)
		return fieldType{}, false
	default:
		return g.fieldTypeOf(n.Underlying(), file, parent, prefix, key, s)
	}
}

// builtinFieldType maps a type of jsonshape's builtin table: a JSON string to
// string, a JSON number to string as well (json.Number keeps digits a double
// would not), raw JSON to google.protobuf.Value, a JSON object to
// google.protobuf.Struct, a nullable time (gorm.DeletedAt) to
// google.protobuf.Timestamp, and a wrapper to the type it wraps.
func (g *generator) builtinFieldType(kind jsonshape.Builtin, n *types.Named, file *protoFile, parent *descriptorpb.DescriptorProto, prefix []int32, key string, s jsonshape.Site) (fieldType, bool) {
	switch kind {
	case jsonshape.BuiltinString, jsonshape.BuiltinNumber:
		return fieldType{kind: descriptorpb.FieldDescriptorProto_TYPE_STRING}, true
	case jsonshape.BuiltinAny:
		file.importOf(structProto)
		return fieldType{kind: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, typeName: wellKnownValue}, true
	case jsonshape.BuiltinObject:
		file.importOf(structProto)
		return fieldType{kind: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, typeName: wellKnownStruct}, true
	case jsonshape.BuiltinNullableString:
		file.importOf(timestampProto)
		return fieldType{kind: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, typeName: wellKnownTimestamp}, true
	case jsonshape.BuiltinWrapper:
		return g.fieldTypeOf(n.TypeArgs().At(0), file, parent, prefix, key, s)
	default:
		g.project.Report(s, "%s values have no protobuf type", n)
		return fieldType{}, false
	}
}

// repeatedOf maps a slice or array of elem: repeated of the element's type,
// which must be a scalar or a message, since protobuf has no repeated of
// repeated or of map.
func (g *generator) repeatedOf(elem types.Type, file *protoFile, parent *descriptorpb.DescriptorProto, prefix []int32, key string, s jsonshape.Site) (fieldType, bool) {
	ft, ok := g.fieldTypeOf(elem, file, parent, prefix, key, s)
	if !ok {
		return fieldType{}, false
	}
	if ft.repeated || ft.mapEntry != nil {
		g.project.Report(s, "a slice of slices or maps has no protobuf type; wrap the element in a struct type")
		return fieldType{}, false
	}
	ft.repeated = true
	ft.optional = false
	return ft, true
}

// mapOf maps a Go map: a JSON object of any values becomes
// google.protobuf.Struct, any other map a protobuf map whose key is a string
// or integer type and whose value is a scalar or a message.
func (g *generator) mapOf(m *types.Map, file *protoFile, parent *descriptorpb.DescriptorProto, prefix []int32, key string, s jsonshape.Site) (fieldType, bool) {
	keyBasic, keyOK := types.Unalias(m.Key()).Underlying().(*types.Basic)
	if keyOK && keyBasic.Info()&types.IsString != 0 {
		if elem, ok := types.Unalias(m.Elem()).Underlying().(*types.Interface); ok && elem.Empty() {
			file.importOf(structProto)
			return fieldType{kind: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, typeName: wellKnownStruct}, true
		}
	}
	if !keyOK || keyBasic.Info()&(types.IsString|types.IsInteger) == 0 {
		g.project.Report(s, "map key type %s has no protobuf type; use a string or integer key type", m.Key())
		return fieldType{}, false
	}
	keyKind, _ := scalarKind(keyBasic)
	value, ok := g.fieldTypeOf(m.Elem(), file, parent, prefix, key, s)
	if !ok {
		return fieldType{}, false
	}
	if value.repeated || value.mapEntry != nil {
		g.project.Report(s, "a map of slices or maps has no protobuf type; wrap the value in a struct type")
		return fieldType{}, false
	}
	entry := &descriptorpb.DescriptorProto{
		Name:    new(strcase.UpperCamelCase(key) + "Entry"),
		Options: &descriptorpb.MessageOptions{MapEntry: new(true)},
		Field: []*descriptorpb.FieldDescriptorProto{
			{Name: new("key"), Number: new(int32(1)), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: keyKind.Enum()},
			{Name: new("value"), Number: new(int32(2)), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: value.kind.Enum()},
		},
	}
	if value.typeName != "" {
		entry.Field[1].TypeName = new(value.typeName)
	}
	parent.NestedType = append(parent.NestedType, entry)
	return fieldType{kind: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, typeName: entry.GetName(), repeated: true, mapEntry: entry}, true
}

// scalarKind maps a basic Go type to its protobuf scalar: bool to bool,
// string to string, int and int64 to int64, int8, int16 and int32 to int32,
// uint and uint64 to uint64, uint8, uint16 and uint32 to uint32, float32 to
// float and float64 to double. Anything else, such as uintptr or complex
// numbers, has none.
func scalarKind(b *types.Basic) (descriptorpb.FieldDescriptorProto_Type, bool) {
	switch b.Kind() {
	case types.Bool:
		return descriptorpb.FieldDescriptorProto_TYPE_BOOL, true
	case types.String:
		return descriptorpb.FieldDescriptorProto_TYPE_STRING, true
	case types.Int, types.Int64:
		return descriptorpb.FieldDescriptorProto_TYPE_INT64, true
	case types.Int8, types.Int16, types.Int32:
		return descriptorpb.FieldDescriptorProto_TYPE_INT32, true
	case types.Uint, types.Uint64:
		return descriptorpb.FieldDescriptorProto_TYPE_UINT64, true
	case types.Uint8, types.Uint16, types.Uint32:
		return descriptorpb.FieldDescriptorProto_TYPE_UINT32, true
	case types.Float32:
		return descriptorpb.FieldDescriptorProto_TYPE_FLOAT, true
	case types.Float64:
		return descriptorpb.FieldDescriptorProto_TYPE_DOUBLE, true
	default:
		return 0, false
	}
}

// fieldComment is the comment of a field: its doc comment, followed for a
// field of a project enum by the values the enum lists, as in
//
//	Status is the lifecycle state.
//
//	Values: "active", "archived".
//
// A bit set lists its constants under "Any bitwise combination of:", and an
// enum none of whose constants is the zero value notes that an unset value
// is the zero value.
func fieldComment(doc string, e *jsonshape.Enum) string {
	if e == nil {
		return doc
	}
	literals := make([]string, 0, len(e.Values))
	for _, v := range e.Values {
		val := v.Const.Val()
		if val.Kind() == constant.String {
			literals = append(literals, strconv.Quote(constant.StringVal(val)))
		} else {
			literals = append(literals, val.ExactString())
		}
	}
	var lines []string
	if doc != "" {
		lines = append(lines, doc, "")
	}
	switch {
	case e.Bitwise:
		lines = append(lines, "Any bitwise combination of: "+strings.Join(literals, ", ")+".")
	default:
		lines = append(lines, "Values: "+strings.Join(literals, ", ")+".")
	}
	if !e.CoversZero && !e.Bitwise {
		lines = append(lines, fmt.Sprintf("Unset, the field holds the zero value %s.", zeroLiteral(e)))
	}
	return strings.Join(lines, "\n")
}

// zeroLiteral is the literal of the zero value of the type of e: "" for a
// string enum and 0 for an integer one.
func zeroLiteral(e *jsonshape.Enum) string {
	if len(e.Values) > 0 && e.Values[0].Const.Val().Kind() == constant.String {
		return `""`
	}
	return "0"
}
