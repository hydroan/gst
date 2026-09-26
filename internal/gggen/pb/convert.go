package pb

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gggen/jsonshape"
	"google.golang.org/protobuf/types/descriptorpb"
)

// This file holds the conversions of the generated handlers file between
// the Go types of the project and their messages: two functions per project
// type, XToProto and XFromProto, assigning the message field by field, with
// what a field cannot hold as the Go value is converted by the helpers of
// the framework's grpc package (see the toProto and fromProto rules).

// fileWriter writes the Go file generated beside one definition (see
// handlerFile): it spells the Go types of the project and of the messages
// as the file refers to them, recording the imports they need, and names
// the temporaries of the function being written.
type fileWriter struct {
	g     *generator
	file  *protoFile
	out   *goFile
	temps map[string]int // the temporaries declared so far in the function, by base name
}

func (g *generator) newFileWriter(f *protoFile) *fileWriter {
	return &fileWriter{g: g, file: f, out: newGoFile(f.goPackageName()), temps: make(map[string]int)}
}

// temp returns the name of a temporary of the function being written: base
// on first use, then base2, base3 and so on, so that the temporaries of a
// nested conversion never shadow one of the enclosing.
func (w *fileWriter) temp(base string) string {
	w.temps[base]++
	if n := w.temps[base]; n > 1 {
		return base + strconv.Itoa(n)
	}
	return base
}

// resetTemps starts a scope no temporary of the enclosing code is visible
// in, a function or one field of a conversion function: the temporaries
// start over.
func (w *fileWriter) resetTemps() { clear(w.temps) }

// guarded returns the statement running body on src when src is not nil,
// with src bound to a temporary unless it is an identifier already:
//
//	if v := p.GetMeta(); v != nil { ... }
//	if v != nil { ... }
func (w *fileWriter) guarded(src ast.Expr, body func(v ast.Expr) []ast.Stmt) ast.Stmt {
	if id, ok := src.(*ast.Ident); ok {
		return ifNotNil(id, body(id)...)
	}
	v := ident(w.temp("v"))
	return ifStmt(define([]string{v.Name}, src), notNil(v), body(v)...)
}

// pkgRef returns the identifier the file refers to the Go package pkg by.
func (w *fileWriter) pkgRef(pkg *types.Package) *ast.Ident {
	if _, fixed := fixedImportNames[pkg.Path()]; fixed {
		return w.out.imports.fixedRef(pkg.Path())
	}
	return w.out.imports.ref(pkg.Path(), pkg.Name())
}

// goType spells the Go type t as the file refers to it, the way the model
// declares it: a type of a package is qualified by the package, model.Record;
// an alias is spelled by its own name, gstmodel.Version for the framework's
// model.Version, whose target is a type of an internal package the project
// cannot name; and an unnamed struct is spelled out with its tags.
func (w *fileWriter) goType(t types.Type) ast.Expr {
	switch u := t.(type) {
	case *types.Alias:
		obj := u.Obj()
		if obj.Pkg() == nil {
			return ident(obj.Name())
		}
		return w.instantiated(sel(w.pkgRef(obj.Pkg()), obj.Name()), u.TypeArgs())
	case *types.Basic:
		return ident(u.Name())
	case *types.Named:
		obj := u.Obj()
		var expr ast.Expr = ident(obj.Name())
		if obj.Pkg() != nil {
			expr = sel(w.pkgRef(obj.Pkg()), obj.Name())
		}
		return w.instantiated(expr, u.TypeArgs())
	case *types.Pointer:
		return star(w.goType(u.Elem()))
	case *types.Slice:
		return &ast.ArrayType{Elt: w.goType(u.Elem())}
	case *types.Array:
		return &ast.ArrayType{Len: &ast.BasicLit{Kind: token.INT, Value: strconv.FormatInt(u.Len(), 10)}, Elt: w.goType(u.Elem())}
	case *types.Map:
		return &ast.MapType{Key: w.goType(u.Key()), Value: w.goType(u.Elem())}
	case *types.Interface:
		if u.Empty() {
			return ident("any")
		}
	case *types.Struct:
		fields := &ast.FieldList{}
		for i := range u.NumFields() {
			f := u.Field(i)
			field := &ast.Field{Type: w.goType(f.Type())}
			if !f.Embedded() {
				field.Names = []*ast.Ident{ident(f.Name())}
			}
			if tag := u.Tag(i); tag != "" {
				field.Tag = &ast.BasicLit{Kind: token.STRING, Value: "`" + tag + "`"}
			}
			fields.List = append(fields.List, field)
		}
		return &ast.StructType{Fields: fields}
	}
	panic(fmt.Sprintf("pb: no Go type expression for %s", t))
}

// instantiated spells the generic type expr instantiated with args, expr
// itself for none.
func (w *fileWriter) instantiated(expr ast.Expr, args *types.TypeList) ast.Expr {
	if args.Len() == 0 {
		return expr
	}
	indices := make([]ast.Expr, args.Len())
	for i := range args.Len() {
		indices[i] = w.goType(args.At(i))
	}
	if len(indices) == 1 {
		return index(expr, indices[0])
	}
	return &ast.IndexListExpr{X: expr, Indices: indices}
}

// messageType spells the Go type of the message of msg as the file refers to
// it: Item in the file's own package, pb_record.Item from another.
func (w *fileWriter) messageType(msg *message) ast.Expr {
	if msg.file.dir() == w.file.dir() {
		return ident(msg.conv.goName)
	}
	return sel(w.out.imports.ref(msg.file.goImportPath(), msg.file.goPackageName()), msg.conv.goName)
}

// conversionFunc refers to the conversion function of msg with the given
// suffix, ItemToProto or pb_record.ItemToProto.
func (w *fileWriter) conversionFunc(msg *message, suffix string) ast.Expr {
	if msg.file.dir() == w.file.dir() {
		return ident(msg.conv.goName + suffix)
	}
	return sel(w.out.imports.ref(msg.file.goImportPath(), msg.file.goPackageName()), msg.conv.goName+suffix)
}

// namedMessage returns the message of t when t is a project type given one,
// nil otherwise.
func (w *fileWriter) namedMessage(t types.Type) *message {
	if n, ok := t.(*types.Named); ok {
		return w.g.messages[n.Obj()]
	}
	return nil
}

// wellKnownGoTypes names the Go types of the well-known message types by
// their package path and type name.
var wellKnownGoTypes = map[string]struct{ importPath, name string }{
	wellKnownTimestamp: {importPathTimestampPB, "Timestamp"},
	wellKnownDuration:  {importPathDurationPB, "Duration"},
	wellKnownValue:     {importPathStructPB, "Value"},
	wellKnownStruct:    {importPathStructPB, "Struct"},
}

// protoType spells the Go type of a message field of protobuf type ft, or
// of one element of it when elem is set: string for a string, *string for
// an optional one, []string for a repeated one, map<string, int64> as
// map[string]int64, a message as a pointer to its Go type, *Record_Window
// for the nested Window and *timestamppb.Timestamp for a Timestamp.
func (w *fileWriter) protoType(ft fieldType, elem bool) ast.Expr {
	if ft.mapEntry != nil {
		return &ast.MapType{Key: ident(scalarGoType(ft.mapKey)), Value: w.protoType(*ft.mapValue, false)}
	}
	var base ast.Expr
	switch ft.kind {
	case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE:
		switch {
		case ft.nested != nil:
			base = star(ident(ft.nested.goName))
		case wellKnownGoTypes[ft.typeName].name != "":
			known := wellKnownGoTypes[ft.typeName]
			base = star(sel(w.out.imports.fixedRef(known.importPath), known.name))
		default:
			msg := w.g.byFullName[ft.typeName]
			if msg == nil {
				panic("pb: no message for " + ft.typeName)
			}
			base = star(w.messageType(msg))
		}
	case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		base = &ast.ArrayType{Elt: ident("byte")}
	default:
		base = ident(scalarGoType(ft.kind))
	}
	if ft.optional {
		return star(base)
	}
	if ft.repeated && !elem {
		return &ast.ArrayType{Elt: base}
	}
	return base
}

// protoGoType returns the Go type of a message field of protobuf type ft, or
// of one element of it when elem is set, for the assignability checks
// deciding whether a Go value is assigned as it is or converted; nil for a
// message, which is never assignable to a Go struct.
func protoGoType(ft fieldType, elem bool) types.Type {
	if ft.mapEntry != nil {
		value := protoGoType(*ft.mapValue, false)
		if value == nil {
			return nil
		}
		return types.NewMap(scalarType(ft.mapKey), value)
	}
	var base types.Type
	switch ft.kind {
	case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE:
		return nil
	case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		base = types.NewSlice(types.Typ[types.Byte])
	default:
		base = scalarType(ft.kind)
	}
	if ft.optional {
		return types.NewPointer(base)
	}
	if ft.repeated && !elem {
		return types.NewSlice(base)
	}
	return base
}

// scalarType and scalarGoType are the Go type of a protobuf scalar, as
// go/types and as the generated files spell it.
func scalarType(kind descriptorpb.FieldDescriptorProto_Type) types.Type {
	switch kind {
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		return types.Typ[types.Bool]
	case descriptorpb.FieldDescriptorProto_TYPE_STRING:
		return types.Typ[types.String]
	case descriptorpb.FieldDescriptorProto_TYPE_INT64:
		return types.Typ[types.Int64]
	case descriptorpb.FieldDescriptorProto_TYPE_INT32:
		return types.Typ[types.Int32]
	case descriptorpb.FieldDescriptorProto_TYPE_UINT64:
		return types.Typ[types.Uint64]
	case descriptorpb.FieldDescriptorProto_TYPE_UINT32:
		return types.Typ[types.Uint32]
	case descriptorpb.FieldDescriptorProto_TYPE_FLOAT:
		return types.Typ[types.Float32]
	case descriptorpb.FieldDescriptorProto_TYPE_DOUBLE:
		return types.Typ[types.Float64]
	}
	panic("pb: no Go type for the protobuf type " + kind.String())
}

func scalarGoType(kind descriptorpb.FieldDescriptorProto_Type) string {
	return scalarType(kind).(*types.Basic).Name() //nolint:errcheck // scalarType answers a basic type.
}

// assignable reports whether a Go value of type t is assignable to a message
// field of protobuf type ft, or to one element of it when elem is set, and
// the field's value back to the Go type: []string to a repeated string and
// a named slice of strings too, but not a named string type, which converts.
func assignable(t types.Type, ft fieldType, elem bool) bool {
	pt := protoGoType(ft, elem)
	return pt != nil && types.AssignableTo(t, pt) && types.AssignableTo(pt, t)
}

// converted spells the conversion of x to the type typ, (*string)(x) for a
// type that is not an identifier.
func converted(typ, x ast.Expr) ast.Expr {
	switch typ.(type) {
	case *ast.Ident, *ast.SelectorExpr, *ast.IndexExpr, *ast.IndexListExpr:
		return call(typ, x)
	}
	return call(&ast.ParenExpr{X: typ}, x)
}

// grpc refers to the function name of the framework's grpc package.
func (w *fileWriter) grpc(name string) ast.Expr {
	return sel(w.out.imports.fixedRef(ggconst.ImportPathGRPC), name)
}

// typeKey names a Go type by package path and name, time.Time for the
// standard time, "" for a type of no package.
func typeKey(t types.Type) string {
	n, ok := t.(*types.Named)
	if !ok || n.Obj().Pkg() == nil {
		return ""
	}
	return n.Obj().Pkg().Path() + "." + n.Obj().Name()
}

// toProto returns the statements encoding src, a Go value of type t, into
// dst, a message field of protobuf type ft. A value the field holds as it
// is, a string, a []string, is assigned; one of another type, an int for
// an int64, a string enum for a string, is converted; a time becomes a
// Timestamp through grpc.Timestamp, a duration a Duration, raw JSON a Value
// through grpc.JSONValue, any value a Value through grpc.Value, a JSON object
// a Struct through grpc.Struct; a project struct converts through its
// XToProto, an unnamed one field by field into its nested message; a slice
// or a map of any of these converts element by element, and a pointer to
// one converts when it is not nil.
//
// The fields of the Record model of the golden fixture encode as
//
//	p.Id = m.ID
//	p.CreatedBy = m.CreatedBy
//	p.UpdatedBy = m.UpdatedBy
//	p.CreatedAt = grpc.Timestamp(m.CreatedAt)
//	p.UpdatedAt = grpc.Timestamp(m.UpdatedAt)
//	p.Title = m.Title
//	p.Status = string(m.Status)
//	p.Summary = m.Summary
//	p.Tags = m.Tags
//	p.Labels = m.Labels
//	p.Count = int64(m.Count)
//	p.Ratio = m.Ratio
//	p.Enabled = m.Enabled
//	p.Payload = m.Payload
//	p.Raw = grpc.JSONValue(m.Raw)
//	p.Extra = grpc.Struct(m.Extra)
//	p.Due = grpc.Timestamp(m.Due)
//	p.Meta = RecordMetaToProto(&m.Meta)
//	p.Window = new(Record_Window)
//	p.Window.From = m.Window.From
//	p.Window.To = m.Window.To
//
// a slice of the Link struct, the links field of Item, as
//
//	if m.Links != nil {
//		p.Links = make([]*Link, len(m.Links))
//		for i, v := range m.Links {
//			p.Links[i] = LinkToProto(&v)
//		}
//	}
//
// and, of the Shape model, the date, time of day, JSON document, JSON
// object, JSON wrapper, struct held by value, JSON number, integer enum and
// optional integer as
//
//	p.Date = grpc.Timestamp(time.Time(m.Date))
//	p.Clock = durationpb.New(time.Duration(m.Clock))
//	p.Doc = grpc.JSONValue(m.Doc)
//	p.Attrs = grpc.Struct(m.Attrs)
//	data := m.Options.Data()
//	p.Options = ShapeOptionsToProto(&data)
//	p.Audit = ShapeAuditToProto(&m.Audit)
//	p.Amount = string(m.Amount)
//	p.Level = int64(m.Level)
//	if m.Score != nil {
//		x := int64(*m.Score)
//		p.Score = &x
//	}
//
// then the map of integers, the map of structs and the slice of unnamed
// structs as
//
//	if m.Scores != nil {
//		p.Scores = make(map[string]int64, len(m.Scores))
//		for k, v := range m.Scores {
//			p.Scores[k] = int64(v)
//		}
//	}
//	if m.ByCode != nil {
//		p.ByCode = make(map[int32]*ShapePoint, len(m.ByCode))
//		for k, v := range m.ByCode {
//			p.ByCode[k] = ShapePointToProto(&v)
//		}
//	}
//	if m.Spans != nil {
//		p.Spans = make([]*Shape_Spans, len(m.Spans))
//		for i, v := range m.Spans {
//			p.Spans[i] = new(Shape_Spans)
//			p.Spans[i].From = int64(v.From)
//			p.Spans[i].To = int64(v.To)
//	}
//	}
func (w *fileWriter) toProto(dst, src ast.Expr, t types.Type, ft fieldType) []ast.Stmt {
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		elem := types.Unalias(p.Elem())
		if msg := w.namedMessage(elem); msg != nil {
			return []ast.Stmt{assign(dst, call(w.conversionFunc(msg, "ToProto"), src))}
		}
		if _, anonymous := elem.(*types.Struct); anonymous {
			body := append([]ast.Stmt{assign(dst, newCall(ident(ft.nested.goName)))}, w.structToProto(dst, src, ft.nested)...)
			return []ast.Stmt{ifNotNil(src, body...)}
		}
		if ft.kind != descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
			// A pointer to a scalar: the optional field it maps to holds
			// the pointer itself when the types agree; a repeated one
			// holds the value pointed to.
			if ft.optional {
				if assignable(t, ft, false) {
					return []ast.Stmt{assign(dst, src)}
				}
				x := w.temp("x")
				return []ast.Stmt{ifNotNil(src, define([]string{x}, converted(w.protoType(elementOf(ft), true), star(src))), assign(dst, addr(ident(x))))}
			}
			return []ast.Stmt{ifNotNil(src, assign(dst, w.encoded(elem, ft, star(src))))}
		}
		// A pointer to a time, a JSON value, a slice or a map: what it points
		// to encodes when there is anything.
		return []ast.Stmt{ifNotNil(src, w.toProto(dst, star(src), elem, ft)...)}
	}

	switch typeKey(t) {
	case "time.Time":
		return []ast.Stmt{assign(dst, call(w.grpc("Timestamp"), src))}
	case "gorm.io/datatypes.Date":
		return []ast.Stmt{assign(dst, call(w.grpc("Timestamp"), call(sel(w.out.imports.fixedRef(importPathTime), "Time"), src)))}
	case "gorm.io/datatypes.Time":
		return []ast.Stmt{assign(dst, call(sel(w.out.imports.fixedRef(importPathDurationPB), "New"), call(sel(w.out.imports.fixedRef(importPathTime), "Duration"), src)))}
	case "gorm.io/gorm.DeletedAt":
		return []ast.Stmt{ifStmt(nil, sel(src, "Valid"), assign(dst, call(sel(w.out.imports.fixedRef(importPathTimestampPB), "New"), sel(src, "Time"))))}
	case "encoding/json.Number":
		return []ast.Stmt{assign(dst, call(ident("string"), src))}
	}
	if n, ok := t.(*types.Named); ok {
		if kind, builtin := jsonshape.BuiltinOf(n); builtin {
			switch kind {
			case jsonshape.BuiltinAny:
				return []ast.Stmt{assign(dst, call(w.grpc("JSONValue"), src))}
			case jsonshape.BuiltinObject:
				return []ast.Stmt{assign(dst, call(w.grpc("Struct"), src))}
			case jsonshape.BuiltinWrapper:
				data := w.temp("data")
				return append([]ast.Stmt{define([]string{data}, call(sel(src, "Data")))}, w.toProto(dst, ident(data), n.TypeArgs().At(0), ft)...)
			}
		}
		if msg := w.namedMessage(n); msg != nil {
			return []ast.Stmt{assign(dst, call(w.conversionFunc(msg, "ToProto"), addr(src)))}
		}
	}

	switch u := t.Underlying().(type) {
	case *types.Basic:
		return []ast.Stmt{assign(dst, w.encoded(t, ft, src))}
	case *types.Slice:
		if ft.kind == descriptorpb.FieldDescriptorProto_TYPE_BYTES || assignable(t, ft, false) {
			return []ast.Stmt{assign(dst, src)}
		}
		i, v := w.temp("i"), w.temp("v")
		return []ast.Stmt{ifNotNil(src,
			assign(dst, makeCall(w.protoType(ft, false), lenCall(src))),
			rangeStmt(i, v, src, w.toProto(index(dst, ident(i)), ident(v), u.Elem(), elementOf(ft))...),
		)}
	case *types.Array:
		i, v := w.temp("i"), w.temp("v")
		return []ast.Stmt{
			assign(dst, makeCall(w.protoType(ft, false), lenCall(src))),
			rangeStmt(i, v, src, w.toProto(index(dst, ident(i)), ident(v), u.Elem(), elementOf(ft))...),
		}
	case *types.Map:
		if ft.typeName == wellKnownStruct {
			return []ast.Stmt{assign(dst, call(w.grpc("Struct"), src))}
		}
		if assignable(t, ft, false) {
			return []ast.Stmt{assign(dst, src)}
		}
		k, v := w.temp("k"), w.temp("v")
		key := ast.Expr(ident(k))
		if !types.Identical(types.Unalias(u.Key()), scalarType(ft.mapKey)) {
			key = call(ident(scalarGoType(ft.mapKey)), ident(k))
		}
		return []ast.Stmt{ifNotNil(src,
			assign(dst, makeCall(w.protoType(ft, false), lenCall(src))),
			rangeStmt(k, v, src, w.toProto(index(dst, key), ident(v), u.Elem(), *ft.mapValue)...),
		)}
	case *types.Interface:
		return []ast.Stmt{assign(dst, call(w.grpc("Value"), src))}
	case *types.Struct:
		return append([]ast.Stmt{assign(dst, newCall(ident(ft.nested.goName)))}, w.structToProto(dst, src, ft.nested)...)
	}
	panic(fmt.Sprintf("pb: no encoding for a %s", t))
}

// encoded returns src, a scalar Go value of type t, as a message field or
// element of protobuf type ft holds it: as it is when assignable, converted
// to the field's Go type otherwise.
func (w *fileWriter) encoded(t types.Type, ft fieldType, src ast.Expr) ast.Expr {
	if assignable(t, ft, true) {
		return src
	}
	return converted(w.protoType(ft, true), src)
}

// elementOf is the protobuf type of one element of a repeated field.
func elementOf(ft fieldType) fieldType {
	ft.repeated = false
	ft.optional = false
	return ft
}

// structToProto returns the statements encoding the fields of src, a struct
// of the conversion conv, into the fields of dst, its message.
func (w *fileWriter) structToProto(dst, src ast.Expr, conv *conversion) []ast.Stmt {
	var stmts []ast.Stmt
	for _, fc := range conv.fields {
		stmts = append(stmts, w.fieldToProto(dst, src, fc)...)
	}
	return stmts
}

// fieldToProto encodes the field fc of the struct src into the field of its
// message dst.
func (w *fileWriter) fieldToProto(dst, src ast.Expr, fc fieldConversion) []ast.Stmt {
	return w.toProto(sel(dst, fc.goName), sel(src, fc.field.Var.Name()), fc.field.Var.Type(), fc.ft)
}

// fromProto returns the statements decoding src, the value of a message
// field of protobuf type ft, into dst, a Go value of type t: the mirror of
// toProto. A message is read through its getters, so an unset one decodes
// into the zero value; the pointer of an optional scalar is taken as it is.
//
// The fields of the Record model of the golden fixture decode as
//
//	m.ID = p.GetId()
//	m.CreatedBy = p.GetCreatedBy()
//	m.UpdatedBy = p.GetUpdatedBy()
//	m.CreatedAt = grpc.Time(p.GetCreatedAt())
//	m.UpdatedAt = grpc.Time(p.GetUpdatedAt())
//	m.Title = p.GetTitle()
//	m.Status = model.RecordStatus(p.GetStatus())
//	m.Summary = p.Summary
//	m.Tags = p.GetTags()
//	m.Labels = p.GetLabels()
//	m.Count = int(p.GetCount())
//	m.Ratio = p.GetRatio()
//	m.Enabled = p.GetEnabled()
//	m.Payload = p.GetPayload()
//	m.Raw = grpc.JSON(p.GetRaw())
//	m.Extra = grpc.Map(p.GetExtra())
//	m.Due = grpc.Time(p.GetDue())
//	if v := p.GetMeta(); v != nil {
//		m.Meta = *RecordMetaFromProto(v)
//	}
//	if v := p.GetWindow(); v != nil {
//		m.Window.From = v.GetFrom()
//		m.Window.To = v.GetTo()
//	}
//
// the links field of Item as
//
//	if p.GetLinks() != nil {
//		m.Links = make([]record.Link, len(p.GetLinks()))
//		for i, v := range p.GetLinks() {
//			if v != nil {
//				m.Links[i] = *LinkFromProto(v)
//			}
//		}
//	}
//
// and, of the Shape model, the date, time of day, JSON document, JSON
// object, JSON wrapper, struct held by value, JSON number, integer enum and
// optional integer as
//
//	m.Date = datatypes.Date(grpc.Time(p.GetDate()))
//	m.Clock = datatypes.Time(p.GetClock().AsDuration())
//	m.Doc = grpc.JSON(p.GetDoc())
//	m.Attrs = grpc.Map(p.GetAttrs())
//	var data model.ShapeOptions
//	if v := p.GetOptions(); v != nil {
//		data = *ShapeOptionsFromProto(v)
//	}
//	m.Options = datatypes.NewJSONType(data)
//	if v := p.GetAudit(); v != nil {
//		m.Audit = *ShapeAuditFromProto(v)
//	}
//	m.Amount = json.Number(p.GetAmount())
//	m.Level = model.ShapeLevel(p.GetLevel())
//	if p.Score != nil {
//		x := int(*p.Score)
//		m.Score = &x
//	}
//
// then the pointer to an unnamed struct and the optional time as
//
//	if v := p.GetNote(); v != nil {
//		m.Note = new(struct {
//			Text string `json:"text" pb:"1"`
//		})
//		m.Note.Text = v.GetText()
//	}
//	if p.GetWhen() != nil {
//		x := grpc.Time(p.GetWhen())
//		m.When = &x
//	}
func (w *fileWriter) fromProto(dst, src ast.Expr, t types.Type, ft fieldType) []ast.Stmt {
	// declared is t as the model spells it, an alias kept, which is how the
	// file spells it too; t itself decides the conversion.
	declared := t
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		elem := types.Unalias(p.Elem())
		if msg := w.namedMessage(elem); msg != nil {
			return []ast.Stmt{assign(dst, call(w.conversionFunc(msg, "FromProto"), src))}
		}
		if _, anonymous := elem.(*types.Struct); anonymous {
			return []ast.Stmt{w.guarded(src, func(v ast.Expr) []ast.Stmt {
				return append([]ast.Stmt{assign(dst, newCall(w.goType(p.Elem())))}, w.structFromProto(dst, v, ft.nested)...)
			})}
		}
		if ft.kind != descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
			if ft.optional && assignable(t, ft, false) {
				return []ast.Stmt{assign(dst, src)}
			}
			x := w.temp("x")
			if ft.optional {
				return []ast.Stmt{ifNotNil(src, define([]string{x}, w.decoded(p.Elem(), elementOf(ft), star(src))), assign(dst, addr(ident(x))))}
			}
			return []ast.Stmt{define([]string{x}, w.decoded(p.Elem(), ft, src)), assign(dst, addr(ident(x)))}
		}
		x := w.temp("x")
		body := append(w.declared(x, p.Elem(), w.fromProto(ident(x), src, p.Elem(), ft)), assign(dst, addr(ident(x))))
		if ft.repeated {
			// A repeated field has no unset: the slice or map it decodes
			// into is always there to point to.
			return body
		}
		return []ast.Stmt{ifNotNil(src, body...)}
	}

	switch typeKey(t) {
	case "time.Time":
		return []ast.Stmt{assign(dst, call(w.grpc("Time"), src))}
	case "gorm.io/datatypes.Date":
		return []ast.Stmt{assign(dst, call(sel(w.out.imports.fixedRef(importPathDatatypes), "Date"), call(w.grpc("Time"), src)))}
	case "gorm.io/datatypes.Time":
		return []ast.Stmt{assign(dst, call(sel(w.out.imports.fixedRef(importPathDatatypes), "Time"), call(sel(src, "AsDuration"))))}
	case "gorm.io/gorm.DeletedAt":
		return []ast.Stmt{w.guarded(src, func(v ast.Expr) []ast.Stmt {
			deleted := compositeLit(sel(w.out.imports.fixedRef(importPathGorm), "DeletedAt"), keyValue("Time", call(sel(v, "AsTime"))), keyValue("Valid", ident("true")))
			return []ast.Stmt{assign(dst, deleted)}
		})}
	case "encoding/json.Number":
		return []ast.Stmt{assign(dst, call(sel(w.out.imports.fixedRef(importPathJSON), "Number"), src))}
	}
	if n, ok := t.(*types.Named); ok {
		if kind, builtin := jsonshape.BuiltinOf(n); builtin {
			switch kind {
			case jsonshape.BuiltinAny:
				return []ast.Stmt{assign(dst, call(w.grpc("JSON"), src))}
			case jsonshape.BuiltinObject:
				return []ast.Stmt{assign(dst, call(w.grpc("Map"), src))}
			case jsonshape.BuiltinWrapper:
				data := w.temp("data")
				arg := n.TypeArgs().At(0)
				stmts := w.declared(data, arg, w.fromProto(ident(data), src, arg, ft))
				return append(stmts, assign(dst, call(sel(w.out.imports.fixedRef(importPathDatatypes), "NewJSONType"), ident(data))))
			}
		}
		if msg := w.namedMessage(n); msg != nil {
			return []ast.Stmt{w.guarded(src, func(v ast.Expr) []ast.Stmt {
				return []ast.Stmt{assign(dst, star(call(w.conversionFunc(msg, "FromProto"), v)))}
			})}
		}
	}

	switch u := t.Underlying().(type) {
	case *types.Basic:
		return []ast.Stmt{assign(dst, w.decoded(declared, ft, src))}
	case *types.Slice:
		if ft.kind == descriptorpb.FieldDescriptorProto_TYPE_BYTES || assignable(t, ft, false) {
			return []ast.Stmt{assign(dst, src)}
		}
		i, v := w.temp("i"), w.temp("v")
		return []ast.Stmt{ifNotNil(src,
			assign(dst, makeCall(w.goType(declared), lenCall(src))),
			rangeStmt(i, v, src, w.fromProto(index(dst, ident(i)), ident(v), u.Elem(), elementOf(ft))...),
		)}
	case *types.Array:
		i := w.temp("i")
		return []ast.Stmt{rangeStmt(i, "", call(ident("min"), lenCall(src), lenCall(dst)),
			w.fromProto(index(dst, ident(i)), index(src, ident(i)), u.Elem(), elementOf(ft))...)}
	case *types.Map:
		if ft.typeName == wellKnownStruct {
			return []ast.Stmt{assign(dst, call(w.grpc("Map"), src))}
		}
		if assignable(t, ft, false) {
			return []ast.Stmt{assign(dst, src)}
		}
		k, v := w.temp("k"), w.temp("v")
		key := ast.Expr(ident(k))
		if !types.Identical(types.Unalias(u.Key()), scalarType(ft.mapKey)) {
			key = converted(w.goType(u.Key()), ident(k))
		}
		return []ast.Stmt{ifNotNil(src,
			assign(dst, makeCall(w.goType(declared), lenCall(src))),
			rangeStmt(k, v, src, w.fromProto(index(dst, key), ident(v), u.Elem(), *ft.mapValue)...),
		)}
	case *types.Interface:
		return []ast.Stmt{assign(dst, call(sel(src, "AsInterface")))}
	case *types.Struct:
		return []ast.Stmt{w.guarded(src, func(v ast.Expr) []ast.Stmt { return w.structFromProto(dst, v, ft.nested) })}
	}
	panic(fmt.Sprintf("pb: no decoding for a %s", t))
}

// declared returns the statements declaring the variable name of type t and
// running stmts, which assign it: x := expr when stmts is that one
// assignment, var x T followed by stmts otherwise.
func (w *fileWriter) declared(name string, t types.Type, stmts []ast.Stmt) []ast.Stmt {
	if len(stmts) == 1 {
		if a, ok := stmts[0].(*ast.AssignStmt); ok && a.Tok == token.ASSIGN && len(a.Lhs) == 1 {
			if id, ok := a.Lhs[0].(*ast.Ident); ok && id.Name == name {
				return []ast.Stmt{define([]string{name}, a.Rhs[0])}
			}
		}
	}
	decl := &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ident(name)}, Type: w.goType(t)}}}}
	return append([]ast.Stmt{decl}, stmts...)
}

// decoded returns src, the scalar value of a message field or element of
// protobuf type ft, as the Go type t holds it: as it is when assignable,
// converted to t, spelled as the model declares it, otherwise.
func (w *fileWriter) decoded(t types.Type, ft fieldType, src ast.Expr) ast.Expr {
	if assignable(types.Unalias(t), ft, true) {
		return src
	}
	return converted(w.goType(t), src)
}

// structFromProto returns the statements decoding the fields of src, a
// message of the conversion conv known to be set, into the fields of dst,
// its struct.
func (w *fileWriter) structFromProto(dst, src ast.Expr, conv *conversion) []ast.Stmt {
	var stmts []ast.Stmt
	for _, fc := range conv.fields {
		stmts = append(stmts, w.fieldFromProto(dst, src, fc)...)
	}
	return stmts
}

// fieldFromProto decodes the field fc of the message src, known to be set,
// into the field of its struct dst: through the getter of the field, or the
// field itself for the pointer of an optional scalar.
func (w *fileWriter) fieldFromProto(dst, src ast.Expr, fc fieldConversion) []ast.Stmt {
	var value ast.Expr = call(sel(src, "Get"+fc.goName))
	if fc.ft.optional {
		value = sel(src, fc.goName)
	}
	return w.fromProto(sel(dst, fc.field.Var.Name()), value, fc.field.Var.Type(), fc.ft)
}

// conversionFuncs builds the two conversion functions of the message of
// msg, a project type: XToProto encoding a value into its message and
// XFromProto decoding a message into a value, nil into nil both ways.
//
// The Link type of the golden fixture, record/item.proto, gets
//
//	// LinkToProto encodes Link values into their message, nil into nil.
//	func LinkToProto(m *record.Link) *Link {
//		if m == nil {
//			return nil
//		}
//		p := new(Link)
//		p.Url = m.URL
//		p.Title = m.Title
//		return p
//	}
//
//	// LinkFromProto decodes Link messages into values, nil into nil.
//	func LinkFromProto(p *Link) *record.Link {
//		if p == nil {
//			return nil
//		}
//		m := new(record.Link)
//		m.URL = p.GetUrl()
//		m.Title = p.GetTitle()
//		return m
//	}
func (w *fileWriter) conversionFuncs(msg *message) {
	goName := msg.conv.goName
	body := []ast.Stmt{
		ifStmt(nil, &ast.BinaryExpr{X: ident("m"), Op: token.EQL, Y: ident("nil")}, returns(ident("nil"))),
		define([]string{"p"}, newCall(ident(goName))),
	}
	for _, fc := range msg.conv.fields {
		// The temporaries of one field are scoped to its statements.
		w.resetTemps()
		body = append(body, w.fieldToProto(ident("p"), ident("m"), fc)...)
	}
	body = append(body, returns(ident("p")))
	w.out.add(goName+"ToProto encodes "+msg.obj.Name()+" values into their message, nil into nil.", &ast.FuncDecl{
		Name: ident(goName + "ToProto"),
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ident("m")}, Type: star(w.goType(msg.obj.Type()))}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: star(ident(goName))}}},
		},
		Body: block(body...),
	}, nil)

	body = []ast.Stmt{
		ifStmt(nil, &ast.BinaryExpr{X: ident("p"), Op: token.EQL, Y: ident("nil")}, returns(ident("nil"))),
		define([]string{"m"}, newCall(w.goType(msg.obj.Type()))),
	}
	for _, fc := range msg.conv.fields {
		w.resetTemps()
		body = append(body, w.fieldFromProto(ident("m"), ident("p"), fc)...)
	}
	body = append(body, returns(ident("m")))
	w.out.add(goName+"FromProto decodes "+msg.obj.Name()+" messages into values, nil into nil.", &ast.FuncDecl{
		Name: ident(goName + "FromProto"),
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ident("p")}, Type: star(ident(goName))}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: star(w.goType(msg.obj.Type()))}}},
		},
		Body: block(body...),
	}, nil)
}

// lowerFirst returns s with its first letter in lower case: createRecord for
// CreateRecord.
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
