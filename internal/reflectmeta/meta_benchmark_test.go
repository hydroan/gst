package reflectmeta_test

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/internal/reflectmeta"
)

func BenchmarkReflectmeta_GetStructMeta(b *testing.B) {
	typ := reflect.TypeFor[User]()

	for b.Loop() {
		meta := reflectmeta.GetStructMeta(typ)
		for i := range meta.NumField() {
			_ = meta.JSONTag(i)
			_ = meta.SchemaTag(i)
			_ = meta.GormTag(i)
			_ = meta.QueryTag(i)
			_ = meta.URLTag(i)
		}
	}
}

func BenchmarkNativeReflect(b *testing.B) {
	typ := reflect.TypeFor[User]()

	for b.Loop() {
		for field := range typ.Fields() {
			_ = field.Tag.Get("json")
			_ = field.Tag.Get("schema")
			_ = field.Tag.Get("gorm")
			_ = field.Tag.Get("query")
			_ = field.Tag.Get("url")
		}
	}
}
