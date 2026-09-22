package ts

import (
	"fmt"
	"go/types"
	"strings"
)

// render renders the declaration of a project type under its doc comment: an
// interface for a struct type (see writeInterface), a union of the constants
// for an enum type (see enumBody), and a type alias for any other. The struct
// Options, the string type Status with the constants active and archived, and
// the named slice type Records of *record.Record of package sample render as
//
//	/** Options is kept as a JSON document. */
//	export interface Options {
//	  theme: string;
//	}
//
//	/**
//	 * Status is the lifecycle state of a sample.
//	 *
//	 * - "active": StatusActive marks a sample in use.
//	 * - "archived": StatusArchived marks a sample kept read-only.
//	 */
//	export type Status = "active" | "archived";
//
//	/** Records lists records. */
//	export type Records = ($record.Record | null)[] | null;
func (g *generator) render(obj *types.TypeName) *declaration {
	ctx := &fileContext{pkgPath: obj.Pkg().Path(), imports: make(map[string]bool)}
	s := site{subject: obj.Pkg().Path() + "." + obj.Name(), pos: obj.Pos()}
	d := &declaration{obj: obj, imports: ctx.imports}
	if reservedTypeNames[obj.Name()] {
		g.report(s, "%s is reserved in TypeScript and cannot name a type; rename the Go type", obj.Name())
	}
	doc := g.sources[ctx.pkgPath].typeDocs[obj]

	var b strings.Builder
	switch t := obj.Type().(type) {
	case *types.Alias:
		if t.TypeParams().Len() > 0 {
			g.report(s, "generic type aliases are not supported; declare an alias for each instantiation the API uses")
			return d
		}
		body := g.expr(t.Rhs(), ctx, s)
		// An alias of a slice, map or pointer holds nil like a named type of
		// one does; an alias of a declared project type leaves that to the
		// declaration it refers to.
		if nullable(t.Rhs()) && !g.refersToDeclaration(t.Rhs()) {
			body += " | null"
		}
		writeDoc(&b, "", doc)
		fmt.Fprintf(&b, "export type %s = %s;\n", obj.Name(), body)
	case *types.Named:
		if t.TypeParams().Len() > 0 {
			g.report(s, "generic types of the project are not supported; declare a non-generic type for each instantiation the API uses")
			return d
		}
		if method := g.method(t, marshalMethods); method != "" {
			g.report(s, "the type declares %s, so its JSON shape is decided by code the generator cannot read; drop the method or use a type without one", method)
			return d
		}
		switch u := t.Underlying().(type) {
		case *types.Struct:
			writeDoc(&b, "", doc)
			g.writeInterface(&b, obj.Name(), g.jsonFields(u, s), ctx, s)
		case *types.Basic:
			if e := g.enumOf(obj); e != nil {
				writeDoc(&b, "", enumDoc(doc, e))
				fmt.Fprintf(&b, "export type %s = %s;\n", obj.Name(), enumBody(e))
				break
			}
			writeDoc(&b, "", doc)
			fmt.Fprintf(&b, "export type %s = %s;\n", obj.Name(), g.structural(u, ctx, s))
		default:
			// A nil slice, map or pointer is a value of the type itself, which
			// encodes as null wherever the type is used.
			body := g.structural(u, ctx, s)
			if nullable(u) {
				body += " | null"
			}
			writeDoc(&b, "", doc)
			fmt.Fprintf(&b, "export type %s = %s;\n", obj.Name(), body)
		}
	}
	d.text = b.String()
	return d
}

// writeInterface writes an interface declaration with a property per key, as
// in
//
//	export interface Endpoint {
//	  /** URL is the address reports go to. */
//	  url: string;
//	  token?: string;
//	}
//
// A struct without keys gets the single member [key: string]: never, which
// admits the empty object alone.
func (g *generator) writeInterface(b *strings.Builder, name string, fields []jsonField, ctx *fileContext, s site) {
	fmt.Fprintf(b, "export interface %s {\n", name)
	if len(fields) == 0 {
		b.WriteString("  [key: string]: never;\n")
	}
	for _, f := range fields {
		writeDoc(b, "  ", g.fieldDoc(f.field))
		fmt.Fprintf(b, "  %s;\n", g.property(f, ctx, g.fieldSite(s, f.key, f.field)))
	}
	b.WriteString("}\n")
}

// fieldDoc returns the doc comment of a project struct field.
func (g *generator) fieldDoc(v *types.Var) string {
	if v.Pkg() == nil {
		return ""
	}
	if source := g.sources[v.Pkg().Path()]; source != nil {
		return source.fieldDocs[v]
	}
	return ""
}

// fieldSite locates a field of the type at s. The field's own position is
// used when the project declares it; a field of a type from outside the
// project is reported where the project uses that type.
func (g *generator) fieldSite(s site, name string, f *types.Var) site {
	pos := s.pos
	if f.Pkg() != nil && g.sources[f.Pkg().Path()] != nil {
		pos = f.Pos()
	}
	return site{subject: s.subject + "." + name, pos: pos}
}

// property renders the property of one key. It is optional when the key may
// be absent -- an omitempty or omitzero option, a nil-able type, promotion
// through an embedded pointer -- and nullable when a value of the field's type
// may encode as null. For example, the fields
//
//	Name    string  `json:"name"`
//	Summary string  `json:"summary,omitempty"`
//	Remark  *string `json:"remark"`
//	Count   int64   `json:"count,string"`
//	Current Status  `json:"status"`
//	Retired Status  `json:"retired,omitempty"`
//
// with Status an enum whose constants leave its zero value out, render as
//
//	name: string
//	summary?: string
//	remark?: string | null
//	count: string
//	status: Status | ""
//	retired?: Status
func (g *generator) property(f jsonField, ctx *fileContext, s site) string {
	t := f.field.Type()
	var value string
	if f.quoted {
		value = "string"
	} else {
		value = g.expr(t, ctx, s)
		// An omitted zero value never reaches the client, unless the field is a
		// pointer: a non-nil pointer to the zero value is not omitted.
		_, isPointer := types.Unalias(t).Underlying().(*types.Pointer)
		if zero := g.enumZero(t); zero != "" && (!f.omit || isPointer) {
			value += " | " + zero
		}
	}
	if nullable(t) {
		value += " | null"
	}
	optional := ""
	if f.omit || f.viaPointer || nilable(t) {
		optional = "?"
	}
	return propertyName(f.key) + optional + ": " + value
}

// expr renders the TypeScript type of a value of t that is not null: Status
// for Status and $record.Record for *record.Record in the file of package
// sample, and { String: string; Valid: boolean } for sql.NullString, a struct
// type from outside the project, which is spelled out where it is used.
// Whether null or the zero value of an enum may stand in for the value is
// decided where the value is used.
func (g *generator) expr(t types.Type, ctx *fileContext, s site) string {
	switch tt := t.(type) {
	case *types.Alias:
		if g.declares(tt.Obj()) {
			if tt.TypeArgs().Len() > 0 {
				g.report(s, "the generic alias %s is not supported; declare an alias for each instantiation the API uses", tt)
				return "unknown"
			}
			return g.ref(tt.Obj(), ctx)
		}
		return g.expr(types.Unalias(tt), ctx, s)
	case *types.Named:
		if kind, ok := builtinOf(tt); ok {
			return g.builtin(kind, tt, ctx, s)
		}
		if g.declares(tt.Obj()) {
			if tt.TypeArgs().Len() > 0 {
				g.report(s, "the generic type %s of the project is not supported; declare a non-generic type for each instantiation the API uses", tt)
				return "unknown"
			}
			return g.ref(tt.Obj(), ctx)
		}
		if method := g.method(tt, marshalMethods); method != "" {
			g.report(s, "type %s declares %s, so its JSON shape is decided by code the generator cannot read; use a type without the method", tt, method)
			return "unknown"
		}
		if _, isStruct := tt.Underlying().(*types.Struct); isStruct {
			key := types.TypeString(tt, nil)
			if g.inlining[key] {
				g.report(s, "type %s refers to itself and is declared outside the project, so it cannot be spelled out inline; use a project type instead", tt)
				return "unknown"
			}
			g.inlining[key] = true
			defer delete(g.inlining, key)
		}
		return g.structural(tt.Underlying(), ctx, s)
	default:
		return g.structural(t, ctx, s)
	}
}

// refersToDeclaration reports whether t renders as a reference to the
// declaration of a project type rather than being spelled out.
func (g *generator) refersToDeclaration(t types.Type) bool {
	switch tt := t.(type) {
	case *types.Alias:
		return g.declares(tt.Obj())
	case *types.Named:
		_, isBuiltin := builtinOf(tt)
		return !isBuiltin && g.declares(tt.Obj())
	default:
		return false
	}
}

// ref queues the declaration of a project type and renders a reference to it
// from the file of ctx: Record from the file of package record, and
// $record.Record from the file of package sample.
func (g *generator) ref(obj *types.TypeName, ctx *fileContext) string {
	g.enqueue(obj)
	if obj.Pkg().Path() == ctx.pkgPath {
		return obj.Name()
	}
	ctx.imports[obj.Pkg().Path()] = true
	return g.importAlias(obj.Pkg().Path()) + "." + obj.Name()
}

// builtin renders a type of the builtin table: string for time.Time,
// { [key: string]: unknown } for datatypes.JSONMap, unknown for
// json.RawMessage, and what the type argument renders as for
// datatypes.JSONType, as Options for datatypes.JSONType[*Options].
func (g *generator) builtin(kind builtinKind, n *types.Named, ctx *fileContext, s site) string {
	switch kind {
	case builtinString, builtinNullableString:
		return "string"
	case builtinNumber:
		return "number"
	case builtinObject:
		return "{ [key: string]: unknown }"
	case builtinWrapper:
		arg := n.TypeArgs().At(0)
		value := g.expr(arg, ctx, s)
		if zero := g.enumZero(arg); zero != "" {
			value += " | " + zero
		}
		return value
	default:
		return "unknown"
	}
}

// structural renders a type by its structure: string for []byte, which
// encodes as base64, number[] for [2]int, { [key: string]: number } for
// map[int]float64, ($record.Record | null)[] for []*record.Record, and
// { from: string; to?: string } for an unnamed struct of the keys from and
// to,omitempty.
func (g *generator) structural(t types.Type, ctx *fileContext, s site) string {
	switch u := t.(type) {
	case *types.Basic:
		info := u.Info()
		switch {
		case info&types.IsBoolean != 0:
			return "boolean"
		case info&types.IsString != 0:
			return "string"
		case info&(types.IsInteger|types.IsFloat) != 0:
			return "number"
		}
		g.report(s, "%s values have no JSON encoding", u)
		return "unknown"
	case *types.Pointer:
		return g.expr(u.Elem(), ctx, s)
	case *types.Slice:
		if g.isByteSlice(u) {
			return "string"
		}
		return arrayOf(g.value(u.Elem(), ctx, s))
	case *types.Array:
		return arrayOf(g.value(u.Elem(), ctx, s))
	case *types.Map:
		g.checkMapKey(u.Key(), s)
		return "{ [key: string]: " + g.value(u.Elem(), ctx, s) + " }"
	case *types.Struct:
		fields := g.jsonFields(u, s)
		if len(fields) == 0 {
			return "{ [key: string]: never }"
		}
		properties := make([]string, len(fields))
		for i, f := range fields {
			properties[i] = g.property(f, ctx, g.fieldSite(s, f.key, f.field))
		}
		return "{ " + strings.Join(properties, "; ") + " }"
	case *types.Interface:
		if u.Empty() {
			return "unknown"
		}
		g.report(s, "an interface with methods has no JSON shape of its own, the dynamic type decides it; use a concrete type")
		return "unknown"
	case *types.Named, *types.Alias:
		return g.expr(u, ctx, s)
	default:
		g.report(s, "%s values have no JSON encoding", t)
		return "unknown"
	}
}

// value renders a value position: the type, the zero value of an enum no
// constant covers, and null when a value of t may encode as null, as in
// Status | "" for an element of []Status.
func (g *generator) value(t types.Type, ctx *fileContext, s site) string {
	value := g.expr(t, ctx, s)
	if zero := g.enumZero(t); zero != "" {
		value += " | " + zero
	}
	if nullable(t) {
		value += " | null"
	}
	return value
}
