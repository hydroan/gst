package openapigen

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/apidoc"
)

// registryPriorityModel is the source comment, which must lose to the registry.
type registryPriorityModel struct {
	// Name source comment, which must lose to the registry.
	Name string `json:"name"`
}

// fallbackOnlyModel is parsed from this source file when not registered.
type fallbackOnlyModel struct {
	// Name is parsed from this source file.
	Name string `json:"name"`
}

func TestParseModelDocsPrefersRegistry(t *testing.T) {
	pkgPath := reflect.TypeFor[registryPriorityModel]().PkgPath()
	apidoc.Register(pkgPath, "registryPriorityModel", apidoc.StructDoc{
		Comment: "registered struct comment",
		Fields:  map[string]string{"Name": "registered field comment"},
	})

	docs := parseModelDocs(&registryPriorityModel{})
	if docs["Name"] != "registered field comment" {
		t.Fatalf(`docs[Name] = %q, want "registered field comment"`, docs["Name"])
	}

	if comment := parseStructComment(&registryPriorityModel{}); comment != "registered struct comment" {
		t.Fatalf(`parseStructComment() = %q, want "registered struct comment"`, comment)
	}
}

func TestParseModelDocsFallsBackToSourceFile(t *testing.T) {
	docs := parseModelDocs(&fallbackOnlyModel{})
	if want := "Name is parsed from this source file."; docs["Name"] != want {
		t.Fatalf("docs[Name] = %q, want %q", docs["Name"], want)
	}

	if want := "fallbackOnlyModel is parsed from this source file when not registered."; parseStructComment(&fallbackOnlyModel{}) != want {
		t.Fatalf("parseStructComment() = %q, want %q", parseStructComment(&fallbackOnlyModel{}), want)
	}
}
