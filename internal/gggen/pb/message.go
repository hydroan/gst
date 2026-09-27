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
	wellKnownDuration  = ".google.protobuf.Duration"
	wellKnownValue     = ".google.protobuf.Value"
	wellKnownStruct    = ".google.protobuf.Struct"
	wellKnownFieldMask = ".google.protobuf.FieldMask"

	timestampProto = "google/protobuf/timestamp.proto"
	durationProto  = "google/protobuf/duration.proto"
	structProto    = "google/protobuf/struct.proto"
	fieldMaskProto = "google/protobuf/field_mask.proto"
)

// The Go types mapped to a well-known time type, keyed by package path and
// name: an instant to Timestamp, gorm's date to a Timestamp at the start of
// the day, and gorm's time of day, a duration since midnight, to Duration.
var timeTypes = map[string]struct{ typeName, proto string }{
	"time.Time":              {wellKnownTimestamp, timestampProto},
	"gorm.io/datatypes.Date": {wellKnownTimestamp, timestampProto},
	"gorm.io/datatypes.Time": {wellKnownDuration, durationProto},
	"gorm.io/gorm.DeletedAt": {wellKnownTimestamp, timestampProto},
}

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
	// enclosing message; mapKey and mapValue are the types of its key and
	// its value.
	mapEntry *descriptorpb.DescriptorProto
	mapKey   descriptorpb.FieldDescriptorProto_Type
	mapValue *fieldType
	// enum lists the values of a project enum, appended to the comment of
	// the field.
	enum *jsonshape.Enum
	// nested is the conversion of the message of an anonymous struct, the
	// field's own or the one its elements or values are.
	nested *conversion
}

// conversion describes the message of a Go struct type field by field, what
// the conversion functions of the generated handlers file are made of (see
// convert.go): the Go name of the message and, for every field of the
// message, the Go struct field it carries.
type conversion struct {
	goName string // the Go type name of the message, Record or RecordWindow
	fields []fieldConversion
}

// fieldConversion is one field of a conversion: the Go struct field it
// carries, its protobuf type and its Go name in the message.
type fieldConversion struct {
	field  jsonshape.Field
	ft     fieldType
	goName string
	// path selects the field from a value of the struct, the embedded
	// structs it is promoted through first (see fieldPath).
	path []string
}

// fieldPath returns the names selecting the field v from a value of the
// struct s, the embedded structs it is promoted through first: Audit, Name
// for the Name of an embedded Audit, Name alone for a field of s itself.
// The generated conversions select a promoted field by its path when its
// name alone would select a field of the same name declared nearer the
// surface, which shadows it in Go while both keep their JSON keys.
func fieldPath(s *types.Struct, v *types.Var) []string {
	for f := range s.Fields() {
		if f == v {
			return []string{f.Name()}
		}
		if !f.Embedded() {
			continue
		}
		t := types.Unalias(f.Type())
		if p, ok := t.(*types.Pointer); ok {
			t = types.Unalias(p.Elem())
		}
		if inner, ok := t.Underlying().(*types.Struct); ok {
			if path := fieldPath(inner, v); path != nil {
				return append([]string{f.Name()}, path...)
			}
		}
	}
	return nil
}

// buildMessage fills the message of obj with the fields of its struct type.
// The keys are the ones the type encodes to (see jsonshape.Fields); each
// carries the number of its pb tag, or the fixed number of a framework base
// key, and the type of its Go type (see fieldTypeOf).
//
// For the model type
//
//	// Item belongs to a record.
//	type Item struct {
//		Content string `json:"content" pb:"11"`
//		Links   []Link `json:"links,omitempty" pb:"12" gorm:"-"`
//
//		model.Base
//	}
//
// the file gets, printed,
//
//	// Item belongs to a record.
//	message Item {
//	  string id = 1;
//
//	  string created_by = 2;
//
//	  string updated_by = 3;
//
//	  google.protobuf.Timestamp created_at = 4;
//
//	  google.protobuf.Timestamp updated_at = 5;
//
//	  string content = 11;
//
//	  repeated Link links = 12;
//	}
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
	m.conv = g.messageOfStruct(m.name, g.project.TypeDoc(obj), st, m.file, s)
	m.file.typed = append(m.file.typed, m)
}

// numberedField is a field of a message being built, with its comment and
// its conversion, the Go name of the latter filled once the fields are in
// order.
type numberedField struct {
	field      *descriptorpb.FieldDescriptorProto
	comment    string
	conversion fieldConversion
}

// messageOfStruct adds the message named name to file under comment, the
// descriptor of the struct st, whose fields are declared in file, and
// returns the conversion of the message. Every message is a top-level one
// of its file, the message of an unnamed struct field included (see
// fieldTypeOf), so the message takes its place in the file before its fields
// are built and the messages of those fields follow it. The fields are
// listed by number, so the framework's base keys come first. The tagged
// fields take their numbers first; a field without a tag is then given the
// next number after every number in use (see nextNumbers), reported with it
// for gg check, and listed in
// DiagnosticsError.MissingTags for gg gen to write into its tag. A key
// promoted through an embedded pointer is reported: the handlers read and
// write every field of a message as a field of the struct, which a nil
// pointer would have them leave out or allocate. So is a key promoted from
// a struct outside the project, whose fields cannot carry pb tags.
func (g *generator) messageOfStruct(name, comment string, st *types.Struct, file *protoFile, s jsonshape.Site) *conversion {
	desc := &descriptorpb.DescriptorProto{Name: new(name)}
	goName, protoName := goCamelCase(name), name
	g.goNames[desc] = goName
	g.protoNames[desc] = protoName
	prefix := []int32{fileMessagesTag, int32Index(len(file.messages))}
	file.addMessage(desc, comment)
	fields := g.project.Fields(st, s)
	base := false
	for _, f := range fields {
		base = base || isBaseField(f)
	}
	numbers := make(map[int32]string)
	planned := make([]int32, len(fields)) // the number of each field, 0 for one left out
	var untagged []int
	for i, f := range fields {
		fs := g.project.FieldSite(s, f.Key, f.Var)
		switch {
		case !identifier.MatchString(f.Key):
			g.project.Report(fs, "the JSON key %q cannot name a protobuf field; name it with a json tag of letters, digits and underscores", f.Key)
		case f.ViaPointer:
			g.project.Report(fs, "the field is promoted through an embedded pointer, which a message has no way to leave unset; embed the struct by value")
		case isBaseField(f):
			planned[i] = BaseFieldNumbers[f.Key]
			numbers[planned[i]] = f.Key
		case f.Var.Pkg() == nil || g.project.Package(f.Var.Pkg().Path()) == nil:
			g.project.Report(fs, "the field is promoted from a struct outside the project, whose fields cannot carry pb tags; embed a project type")
		default:
			tag, tagged := reflect.StructTag(f.Tag).Lookup(Tag)
			if !tagged {
				untagged = append(untagged, i)
				continue
			}
			number := g.taggedNumber(tag, base, fs)
			if number == 0 {
				continue
			}
			if previous, taken := numbers[number]; taken {
				g.project.Report(fs, "field number %d is already taken by %s; give each field its own number", number, previous)
				continue
			}
			numbers[number] = f.Key
			planned[i] = number
		}
	}
	if len(untagged) > 0 {
		next := g.nextNumbers(file, protoName, numbers, base)
		for _, i := range untagged {
			f := fields[i]
			fs := g.project.FieldSite(s, f.Key, f.Var)
			number, ok := next(f.Key)
			if !ok {
				g.project.Report(fs, "the field has no pb tag, and every field number left is one the committed %s/%s reserves; lift a reservation, or delete the file to start over", ggconst.DirPB, file.name)
				continue
			}
			numbers[number] = f.Key
			planned[i] = number
			g.project.Report(fs, "the field has no pb tag; number it pb:%q", strconv.Itoa(int(number)))
			position := g.project.FileSet().Position(f.Var.Pos())
			g.missingTags = append(g.missingTags, MissingTag{Path: g.project.RelativeFile(position.Filename), Line: position.Line, Struct: protoName, Field: f.Var.Name(), Number: number})
		}
	}

	var numbered []numberedField
	for i, f := range fields {
		if planned[i] == 0 {
			continue
		}
		fs := g.project.FieldSite(s, f.Key, f.Var)
		ft, ok := g.fieldTypeOf(f.Var.Type(), file, desc, f.Key, fs)
		if !ok {
			continue
		}
		field := &descriptorpb.FieldDescriptorProto{
			Name:   new(f.Key),
			Number: new(planned[i]),
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
		// The field's name selects it unless Go resolves the name to another
		// field, one declared nearer the surface with the same name, or to
		// none, two embedded structs promoting it alike: then its path does.
		path := []string{f.Var.Name()}
		if obj, _, _ := types.LookupFieldOrMethod(st, true, f.Var.Pkg(), f.Var.Name()); obj != f.Var {
			path = fieldPath(st, f.Var)
		}
		numbered = append(numbered, numberedField{field: field, comment: fieldComment(g.project.FieldDoc(f.Var), ft.enum), conversion: fieldConversion{field: f, ft: ft, path: path}})
	}
	slices.SortStableFunc(numbered, func(a, b numberedField) int { return cmp.Compare(a.field.GetNumber(), b.field.GetNumber()) })
	for i, nf := range numbered {
		file.comment(append(slices.Clone(prefix), messageFieldsTag, int32Index(i)), nf.comment)
		desc.Field = append(desc.Field, nf.field)
	}
	conv := &conversion{goName: goName, fields: make([]fieldConversion, 0, len(numbered))}
	goFields := goFieldNames(desc)
	for _, nf := range numbered {
		nf.conversion.goName = goFields[nf.field.GetName()]
		conv.fields = append(conv.fields, nf.conversion)
	}
	return conv
}

// nextNumbers returns the numbering of the fields without a pb tag of the
// message protoName of file, given the numbers its tagged fields hold and
// whether it embeds the framework's base: a field the committed definition
// under pb/ already holds keeps the number it held there, and any other
// field takes the next number after the highest one in use, in the message
// as it is and in the committed one, skipping the numbers the committed
// message reserves and the range 19000 to 19999 protobuf reserves. It
// answers false once no number is left. Record, holding title = 11 and
// tags = 12 in its committed definition and declaring title tagged 11, tags
// untagged and a new summary untagged, numbers tags 12 and summary 13; with
// tags dropped and 12 reserved by the committed file, summary is 13 too.
func (g *generator) nextNumbers(file *protoFile, protoName string, numbers map[int32]string, base bool) func(key string) (int32, bool) {
	committed := g.committedMessage(file, protoName)
	committedNumbers := make(map[int32]bool, len(committed.GetField()))
	committedByName := make(map[string]int32, len(committed.GetField()))
	for _, f := range committed.GetField() {
		committedNumbers[f.GetNumber()] = true
		committedByName[f.GetName()] = f.GetNumber()
	}
	first := int32(1)
	if base {
		first = FirstBusinessFieldNumber
	}
	candidate := first - 1
	for n := range numbers {
		candidate = max(candidate, n)
	}
	for n := range committedNumbers {
		candidate = max(candidate, n)
	}
	taken := func(n int32) bool {
		_, held := numbers[n]
		return held || committedNumbers[n] || reserves(committed.GetReservedRange(), n) || (n >= reservedRangeStart && n <= reservedRangeEnd)
	}
	return func(key string) (int32, bool) {
		if n, ok := committedByName[key]; ok {
			if _, held := numbers[n]; !held {
				return n, true
			}
		}
		for candidate < fieldMaxNumber {
			candidate++
			if !taken(candidate) {
				return candidate, true
			}
		}
		return 0, false
	}
}

// isBaseField reports whether f is a key promoted from the framework's model
// base, which carries a fixed field number.
func isBaseField(f jsonshape.Field) bool {
	_, fixed := BaseFieldNumbers[f.Key]
	return fixed && f.Var.Pkg() != nil && f.Var.Pkg().Path() == modelRegistryPath
}

// taggedNumber returns the field number the pb tag of a field names, which
// must be positive, at most 536870911, outside the range 19000 to 19999
// protobuf reserves, and from FirstBusinessFieldNumber on in a message
// embedding the base. A number that fails these is reported and yields 0.
func (g *generator) taggedNumber(tag string, base bool, s jsonshape.Site) int32 {
	first := int32(1)
	if base {
		first = FirstBusinessFieldNumber
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
// double; time.Time, datatypes.Date and gorm.DeletedAt to
// google.protobuf.Timestamp and datatypes.Time to google.protobuf.Duration
// (see timeTypes); a project enum to its underlying string or integer type;
// a JSON document (any, json.RawMessage, datatypes.JSON) to
// google.protobuf.Value and a JSON object (map[string]any,
// datatypes.JSONMap) to google.protobuf.Struct; []byte to bytes, a slice or
// array to repeated, a map to map; a pointer to the type it
// points to, optional when that is a scalar; a project struct to its message
// (queued to be built) and an unnamed struct to a message of its own beside
// parent, named after parent and the field. Anything else is reported: a
// nested slice or map, a map with a value of those, a map key of the wrong
// type, an interface with methods, a type with encoding methods of its own,
// a struct from outside the project, an unnamed struct whose message name a
// type of the file already takes.
//
// The fields of the Record model of the golden fixture print as follows,
// the Go field on the left of each arrow and the protobuf field on its right:
//
//	Status  RecordStatus      -> string status = 12;
//	Summary *string           -> optional string summary = 13;
//	Tags    []string          -> repeated string tags = 14;
//	Labels  map[string]string -> map<string, string> labels = 15;
//	Count   int               -> int64 count = 16;
//	Ratio   float64           -> double ratio = 17;
//	Enabled bool              -> bool enabled = 18;
//	Payload []byte            -> bytes payload = 19;
//	Raw     json.RawMessage   -> google.protobuf.Value raw = 20;
//	Extra   map[string]any    -> google.protobuf.Struct extra = 21;
//	Due     time.Time         -> google.protobuf.Timestamp due = 22;
//	Meta    RecordMeta        -> RecordMeta meta = 23;
//	Window  struct{...}       -> RecordWindow window = 24;
//
// the last with the message RecordWindow declared after Record, holding the
// fields of the struct.
func (g *generator) fieldTypeOf(t types.Type, file *protoFile, parent *descriptorpb.DescriptorProto, key string, s jsonshape.Site) (fieldType, bool) {
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		ft, ok := g.fieldTypeOf(types.Unalias(p.Elem()), file, parent, key, s)
		if ok && ft.kind != descriptorpb.FieldDescriptorProto_TYPE_MESSAGE && !ft.repeated {
			ft.optional = true
		}
		return ft, ok
	}
	switch u := t.(type) {
	case *types.Named:
		return g.namedFieldType(u, file, parent, key, s)
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
		return g.repeatedOf(u.Elem(), file, parent, key, s)
	case *types.Array:
		return g.repeatedOf(u.Elem(), file, parent, key, s)
	case *types.Map:
		return g.mapOf(u, file, parent, key, s)
	case *types.Interface:
		if u.Empty() {
			file.importOf(structProto)
			return fieldType{kind: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, typeName: wellKnownValue}, true
		}
		g.project.Report(s, "an interface with methods has no protobuf type, the dynamic type decides it; use a concrete type")
		return fieldType{}, false
	case *types.Struct:
		// An unnamed struct becomes a message of its own beside the
		// enclosing one, named after both: RecordWindow, RecordWindow in Go
		// too, for the window field of Record. Nested in the enclosing
		// message under the field's name, it would shadow a top-level
		// message of that name for every field of the enclosing one, the
		// relative names the printed file writes being resolved from the
		// inside out.
		name := g.protoNames[parent] + strcase.UpperCamelCase(key)
		if holder, ok := file.claim(name, "the "+key+" field of "+g.protoNames[parent]); !ok {
			g.project.Report(s, "the unnamed struct of the field becomes the message %s, which clashes with %s; name the field or the type differently", name, holder)
			return fieldType{}, false
		}
		conv := g.messageOfStruct(name, name+" is the message of the "+key+" field of "+g.protoNames[parent]+".", u, file, s)
		return fieldType{kind: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, typeName: "." + file.pkg + "." + name, nested: conv}, true
	default:
		g.project.Report(s, "%s values have no protobuf type", t)
		return fieldType{}, false
	}
}

// namedFieldType maps a named type (see fieldTypeOf).
func (g *generator) namedFieldType(n *types.Named, file *protoFile, parent *descriptorpb.DescriptorProto, key string, s jsonshape.Site) (fieldType, bool) {
	obj := n.Obj()
	if obj.Pkg() != nil {
		if wellKnown, ok := timeTypes[obj.Pkg().Path()+"."+obj.Name()]; ok {
			file.importOf(wellKnown.proto)
			return fieldType{kind: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, typeName: wellKnown.typeName}, true
		}
	}
	if kind, ok := jsonshape.BuiltinOf(n); ok {
		return g.builtinFieldType(kind, n, file, parent, key, s)
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
			return g.fieldTypeOf(n.Underlying(), file, parent, key, s)
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
		return g.fieldTypeOf(n.Underlying(), file, parent, key, s)
	}
}

// builtinFieldType maps a type of jsonshape's builtin table other than the
// time types (see timeTypes): a JSON number to string (json.Number keeps
// digits a double would not), raw JSON to google.protobuf.Value, a JSON
// object to google.protobuf.Struct, and a wrapper to the type it wraps.
func (g *generator) builtinFieldType(kind jsonshape.Builtin, n *types.Named, file *protoFile, parent *descriptorpb.DescriptorProto, key string, s jsonshape.Site) (fieldType, bool) {
	switch kind {
	case jsonshape.BuiltinNumber:
		return fieldType{kind: descriptorpb.FieldDescriptorProto_TYPE_STRING}, true
	case jsonshape.BuiltinAny:
		file.importOf(structProto)
		return fieldType{kind: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, typeName: wellKnownValue}, true
	case jsonshape.BuiltinObject:
		file.importOf(structProto)
		return fieldType{kind: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, typeName: wellKnownStruct}, true
	case jsonshape.BuiltinWrapper:
		return g.fieldTypeOf(n.TypeArgs().At(0), file, parent, key, s)
	default:
		g.project.Report(s, "%s values have no protobuf type", n)
		return fieldType{}, false
	}
}

// repeatedOf maps a slice or array of elem: repeated of the element's type,
// which must be a scalar or a message, since protobuf has no repeated of
// repeated or of map.
func (g *generator) repeatedOf(elem types.Type, file *protoFile, parent *descriptorpb.DescriptorProto, key string, s jsonshape.Site) (fieldType, bool) {
	ft, ok := g.fieldTypeOf(elem, file, parent, key, s)
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
func (g *generator) mapOf(m *types.Map, file *protoFile, parent *descriptorpb.DescriptorProto, key string, s jsonshape.Site) (fieldType, bool) {
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
	if _, pointer := types.Unalias(m.Elem()).(*types.Pointer); pointer {
		g.project.Report(s, "a map of pointers has no protobuf type, a map value is never unset; use a map of values")
		return fieldType{}, false
	}
	value, ok := g.fieldTypeOf(m.Elem(), file, parent, key, s)
	if !ok {
		return fieldType{}, false
	}
	if value.repeated || value.mapEntry != nil {
		g.project.Report(s, "a map of slices or maps has no protobuf type; wrap the value in a struct type")
		return fieldType{}, false
	}
	entry := &descriptorpb.DescriptorProto{
		Name:    new(mapEntryName(key)),
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
	return fieldType{kind: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, typeName: entry.GetName(), repeated: true, mapEntry: entry, mapKey: keyKind, mapValue: &value, nested: value.nested}, true
}

// mapEntryName returns the name protoc gives the entry message of the map
// field named key, which protodesc holds the descriptor to: the first
// letter and every letter after an underscore upper-cased, the underscores
// dropped, the other characters kept as they are, and Entry appended.
// headers gives HeadersEntry, http_headers HttpHeadersEntry, HTTPHeaders
// HTTPHeadersEntry and userIDList UserIDListEntry.
func mapEntryName(key string) string {
	var b strings.Builder
	capitalize := true
	for _, r := range key {
		switch {
		case r == '_':
			capitalize = true
		case capitalize:
			if r >= 'a' && r <= 'z' {
				r -= 'a' - 'A'
			}
			b.WriteRune(r)
			capitalize = false
		default:
			b.WriteRune(r)
		}
	}
	b.WriteString("Entry")
	return b.String()
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
// is the zero value: the Status field of the golden Record, without a doc
// comment, gets
//
//	Values: "active", "archived".
//	Unset, the field holds the zero value "".
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
