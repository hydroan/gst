package modelregistry

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/apidoc"
)

func TestAutoBaseDocsRegisteredFromEmbeddedSource(t *testing.T) {
	doc, ok := apidoc.Lookup(reflect.TypeFor[AutoBase]().PkgPath(), "AutoBase")
	if !ok {
		t.Fatal("apidoc.Lookup() ok = false, want AutoBase docs registered at init")
	}

	for _, field := range []string{"ID", "CreatedBy", "UpdatedBy", "CreatedAt", "UpdatedAt", "DeletedAt"} {
		if doc.Fields[field] == "" {
			t.Fatalf("doc.Fields[%s] is empty, want the field comment from autobase.go", field)
		}
	}
}
