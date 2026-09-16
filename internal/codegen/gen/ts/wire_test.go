package ts

import (
	"encoding/json"
	"go/types"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/codegen/gen/ts/fixture/record"
	"github.com/hydroan/gst/internal/codegen/gen/ts/fixture/sample"
	"github.com/stretchr/testify/require"
)

// TestPropertiesMatchEncodingJSON holds the declared properties of the fixture
// types against what encoding/json writes for them, for the zero value and
// for a value with every field set: each encoded key is declared, a null only
// reaches a property declared to hold one, and only the zero value may leave
// out a key, which must then be declared optional.
func TestPropertiesMatchEncodingJSON(t *testing.T) {
	g := newGenerator(fixtureConfig(), loadFixture(t))
	tests := []struct {
		ref TypeRef
		typ reflect.Type
	}{
		{TypeRef{PkgPath: fixtureModule + "/sample", Name: "Sample"}, reflect.TypeFor[sample.Sample]()},
		{TypeRef{PkgPath: fixtureModule + "/sample", Name: "Item"}, reflect.TypeFor[sample.Item]()},
		{TypeRef{PkgPath: fixtureModule + "/record", Name: "Record"}, reflect.TypeFor[record.Record]()},
	}
	for _, tt := range tests {
		t.Run(tt.ref.Name, func(t *testing.T) {
			properties := declaredProperties(t, g, tt.ref)
			full := reflect.New(tt.typ).Elem()
			fillValue(full, 0)

			for name, value := range map[string]reflect.Value{"zero": reflect.New(tt.typ).Elem(), "full": full} {
				object := encodeObject(t, value)
				for key, raw := range object {
					property, declared := properties[key]
					require.Truef(t, declared, "the %s value encodes key %q, which is not declared", name, key)
					if string(raw) == "null" {
						require.Truef(t, property.nullable, "the %s value encodes %q as null, but it is declared as %s", name, key, property.text)
					}
				}
				for key, property := range properties {
					if _, encoded := object[key]; !encoded {
						require.Truef(t, name == "zero" && property.optional, "the %s value does not encode %q, which is declared as %s", name, key, property.text)
					}
				}
			}
		})
	}
}

// declaredProperty is a rendered property and what the test reads off it.
type declaredProperty struct {
	text     string
	optional bool
	nullable bool
}

// declaredProperties renders the properties of the struct type ref names,
// keyed by JSON key.
func declaredProperties(t *testing.T, g *generator, ref TypeRef) map[string]declaredProperty {
	t.Helper()

	obj := g.pkgs[ref.PkgPath].Types.Scope().Lookup(ref.Name)
	require.NotNil(t, obj)
	st, ok := obj.Type().Underlying().(*types.Struct)
	require.True(t, ok)

	s := site{subject: ref.PkgPath + "." + ref.Name}
	ctx := &fileContext{pkgPath: ref.PkgPath, imports: make(map[string]bool)}
	properties := make(map[string]declaredProperty)
	for _, f := range g.jsonFields(st, s) {
		text := g.property(f, ctx, s)
		name, value, _ := strings.Cut(text, ": ")
		properties[f.key] = declaredProperty{
			text:     text,
			optional: strings.HasSuffix(name, "?"),
			nullable: strings.HasSuffix(value, " | null") || value == "unknown",
		}
	}
	require.Empty(t, g.diags)
	return properties
}

// encodeObject encodes value with encoding/json and decodes the keys of the
// resulting object.
func encodeObject(t *testing.T, value reflect.Value) map[string]json.RawMessage {
	t.Helper()

	encoded, err := json.Marshal(value.Interface())
	require.NoError(t, err)
	var object map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &object))
	return object
}

// timeType is the type fillValue sets to a fixed instant rather than walking
// its unexported fields.
var timeType = reflect.TypeFor[time.Time]()

// fillValue sets every exported field v reaches to a value no omitempty or
// omitzero option drops. Pointers are allocated a few levels deep, which ends
// recursive types.
func fillValue(v reflect.Value, depth int) {
	switch {
	case v.Kind() == reflect.Struct && v.Type().ConvertibleTo(timeType):
		v.Set(reflect.ValueOf(time.Unix(1, 0).UTC()).Convert(v.Type()))
		return
	case v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8:
		// Raw JSON types must hold valid JSON; a plain byte slice takes any
		// bytes.
		v.SetBytes([]byte("{}"))
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		if depth < 3 {
			v.Set(reflect.New(v.Type().Elem()))
			fillValue(v.Elem(), depth+1)
		}
	case reflect.Struct:
		for _, field := range v.Fields() {
			if field.CanSet() {
				fillValue(field, depth)
			}
		}
	case reflect.Slice:
		elems := reflect.MakeSlice(v.Type(), 1, 1)
		fillValue(elems.Index(0), depth)
		v.Set(elems)
	case reflect.Array:
		for i := range v.Len() {
			fillValue(v.Index(i), depth)
		}
	case reflect.Map:
		entries := reflect.MakeMapWithSize(v.Type(), 1)
		key := reflect.New(v.Type().Key()).Elem()
		fillValue(key, depth)
		elem := reflect.New(v.Type().Elem()).Elem()
		fillValue(elem, depth)
		entries.SetMapIndex(key, elem)
		v.Set(entries)
	case reflect.Interface:
		v.Set(reflect.ValueOf("value"))
	case reflect.String:
		v.SetString("value")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		v.SetUint(1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1.5)
	default:
	}
}
