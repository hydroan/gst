package modelregistry

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/apidoc"
)

func TestEmbeddedDocsRegisteredForEveryDeclaredStruct(t *testing.T) {
	baseFields := []string{"ID", "CreatedBy", "UpdatedBy", "CreatedAt", "UpdatedAt", "DeletedAt"}

	tests := []struct {
		typeName string
		source   string
		fields   []string
	}{
		{typeName: "Base", source: "base.go", fields: baseFields},
		{typeName: "AutoBase", source: "autobase.go", fields: baseFields},
		{typeName: "Query", source: "query.go", fields: []string{"Expand", "Depth", "SortBy"}},
		{typeName: "Pagination", source: "query.go", fields: []string{"Page", "Size"}},
		{typeName: "Cursor", source: "query.go", fields: []string{"CursorValue", "CursorField", "CursorNext"}},
	}

	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			doc, ok := apidoc.Lookup(reflect.TypeFor[Base]().PkgPath(), tt.typeName)
			if !ok {
				t.Fatalf("apidoc.Lookup(%s) ok = false, want docs registered at init", tt.typeName)
			}

			if doc.Comment == "" {
				t.Errorf("doc.Comment is empty, want the struct comment from %s", tt.source)
			}
			for _, field := range tt.fields {
				if doc.Fields[field] == "" {
					t.Errorf("doc.Fields[%s] is empty, want the field comment from %s", field, tt.source)
				}
			}
		})
	}
}

func TestRegisterEmbeddedDocsPanicsOnUnparsableSource(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("registerEmbeddedDocs() returned normally, want a panic when the embedded source stops parsing")
		}
	}()

	registerEmbeddedDocs("broken.go", []byte("package modelregistry\nfunc ("))
}
