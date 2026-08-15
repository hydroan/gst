package modelregistry

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/apidoc"
)

func TestQueryDocsRegisteredFromEmbeddedSource(t *testing.T) {
	tests := []struct {
		typeName string
		fields   []string
	}{
		{typeName: "Query", fields: []string{"Expand", "Depth", "SortBy"}},
		{typeName: "Pagination", fields: []string{"Page", "Size"}},
		{typeName: "Cursor", fields: []string{"CursorValue", "CursorField", "CursorNext"}},
	}

	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			doc, ok := apidoc.Lookup(reflect.TypeFor[Query]().PkgPath(), tt.typeName)
			if !ok {
				t.Fatalf("apidoc.Lookup(%s) ok = false, want docs registered at init", tt.typeName)
			}

			for _, field := range tt.fields {
				if doc.Fields[field] == "" {
					t.Fatalf("doc.Fields[%s] is empty, want the field comment from query.go", field)
				}
			}
		})
	}
}
