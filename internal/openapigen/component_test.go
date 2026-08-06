package openapigen

import (
	"reflect"
	"testing"
)

func TestSchemaComponentName(t *testing.T) {
	tests := []struct {
		name    string
		pkgPath string
		typName string
		want    string
	}{
		{"model subpackage", "myproject/model/sample", "Record", "sample.Record"},
		{"nested model subpackage", "github.com/hydroan/gst/internal/model/iam/user", "User", "iam.user.User"},
		{"model root", "myproject/model", "Item", "Item"},
		{"non-model package", "github.com/hydroan/gst/module/mfa", "TOTPBind", "module.mfa.TOTPBind"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := schemaComponentNameFromPath(tt.pkgPath, tt.typName)
			if got != tt.want {
				t.Fatalf("schemaComponentName(%s.%s) = %q, want %q", tt.pkgPath, tt.typName, got, tt.want)
			}
		})
	}
}

func TestUniqueComponentNameResolvesCrossPackageCollision(t *testing.T) {
	first := uniqueComponentName(reflect.TypeFor[openapiTimeQueryModel]())
	if first != "internal.openapigen.openapiTimeQueryModel" {
		t.Fatalf("first = %q, want the last-two-segment qualified name", first)
	}

	// A different package claiming the same base name must get a fully
	// qualified fallback instead of silently sharing the component.
	componentNameMu.Lock()
	componentNameOwners["internal.openapigen.openapiTimeQueryModel"] = "example.com/other/pkg"
	componentNameMu.Unlock()

	second := uniqueComponentName(reflect.TypeFor[openapiTimeQueryModel]())
	if second != "github.com.hydroan.gst.internal.openapigen.openapiTimeQueryModel" {
		t.Fatalf("second = %q, want the fully qualified fallback", second)
	}
}
