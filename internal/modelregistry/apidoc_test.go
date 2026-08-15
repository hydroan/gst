package modelregistry

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/apidoc"
)

// TestGeneratedDocsRegisterEveryDeclaredStruct checks that apidoc.gen.go
// registers the doc comments the OpenAPI generator turns into schema and query
// parameter descriptions. The generator reads the registry and nothing else, so
// a struct missing here is a struct that ships undocumented.
func TestGeneratedDocsRegisterEveryDeclaredStruct(t *testing.T) {
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

// TestGeneratedDocsKeepImplementationNotesOutOfFieldDocs pins the field docs an
// implementation note would silently take over. Doc comment extraction prefers
// a field's doc comment over its trailing one, so a note written above a field
// stops being a note and becomes that field's API-facing description; these
// fields carry notes about their column type, which is exactly the trap.
func TestGeneratedDocsKeepImplementationNotesOutOfFieldDocs(t *testing.T) {
	tests := []struct {
		typeName string
		field    string
		want     string
	}{
		{typeName: "Base", field: "ID", want: "UUIDv7 identifier for the record"},
		{typeName: "Base", field: "CreatedBy", want: "UUIDv7 user ID who created the record"},
		{typeName: "AutoBase", field: "CreatedBy", want: "UUIDv7 user ID who created the record"},
		{typeName: "AutoBase", field: "UpdatedBy", want: "UUIDv7 user ID who last updated the record"},
	}

	for _, tt := range tests {
		t.Run(tt.typeName+"."+tt.field, func(t *testing.T) {
			doc, ok := apidoc.Lookup(reflect.TypeFor[Base]().PkgPath(), tt.typeName)
			if !ok {
				t.Fatalf("apidoc.Lookup(%s) ok = false, want docs registered at init", tt.typeName)
			}

			if got := doc.Fields[tt.field]; got != tt.want {
				t.Fatalf("doc.Fields[%s] = %q, want the trailing field comment %q", tt.field, got, tt.want)
			}
		})
	}
}
