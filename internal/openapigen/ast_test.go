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

// anonAliasPayload is a type alias to an anonymous struct. Reflection resolves
// neither a package path nor a type name for it, so its field docs can only be
// recovered by field signature.
type anonAliasPayload = struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

func TestParseModelDocsRecoversAnonymousStructBySignature(t *testing.T) {
	apidoc.Register("openapigen/anon", "anonAliasPayloadDoc", apidoc.StructDoc{
		Fields: map[string]string{
			"Title":   "The title.",
			"Summary": "The summary.",
		},
	})

	docs := parseModelDocs(new(anonAliasPayload))
	if want := "The title."; docs["Title"] != want {
		t.Fatalf("docs[Title] = %q, want %q", docs["Title"], want)
	}
	if want := "The summary."; docs["Summary"] != want {
		t.Fatalf("docs[Summary] = %q, want %q", docs["Summary"], want)
	}
}

func TestParseModelDocsIgnoresAmbiguousAnonymousSignature(t *testing.T) {
	// Two structs share a field-name set but carry different field docs, so the
	// signature is ambiguous and must not resolve to either struct's docs.
	apidoc.Register("openapigen/anon", "ambiguousDocA", apidoc.StructDoc{
		Fields: map[string]string{"Alpha": "Alpha from A.", "Beta": "Beta from A."},
	})
	apidoc.Register("openapigen/anon", "ambiguousDocB", apidoc.StructDoc{
		Fields: map[string]string{"Alpha": "Alpha from B.", "Beta": "Beta from B."},
	})

	type ambiguousAnon = struct {
		Alpha string `json:"alpha"`
		Beta  string `json:"beta"`
	}

	docs := parseModelDocs(new(ambiguousAnon))
	if len(docs) != 0 {
		t.Fatalf("docs = %v, want empty for ambiguous signature", docs)
	}
}
