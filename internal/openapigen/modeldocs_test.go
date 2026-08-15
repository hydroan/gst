package openapigen

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/apidoc"
)

// registryDocModel deliberately carries no doc comments: everything the
// generator documents it with has to come from the registry.
type registryDocModel struct {
	Name string `json:"name"`
}

func TestModelFieldDocsReadsRegistry(t *testing.T) {
	pkgPath := reflect.TypeFor[registryDocModel]().PkgPath()
	apidoc.Register(pkgPath, "registryDocModel", apidoc.StructDoc{
		Comment: "registered struct comment",
		Fields:  map[string]string{"Name": "registered field comment"},
	})

	docs := modelFieldDocs(&registryDocModel{})
	if docs["Name"] != "registered field comment" {
		t.Fatalf(`docs[Name] = %q, want "registered field comment"`, docs["Name"])
	}

	if comment := modelStructComment(&registryDocModel{}); comment != "registered struct comment" {
		t.Fatalf(`modelStructComment() = %q, want "registered struct comment"`, comment)
	}
}

// anonAliasPayload is a type alias to an anonymous struct. Reflection resolves
// neither a package path nor a type name for it, so its field docs can only be
// recovered by field signature.
type anonAliasPayload = struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

func TestModelFieldDocsRecoversAnonymousStructBySignature(t *testing.T) {
	apidoc.Register("openapigen/anon", "anonAliasPayloadDoc", apidoc.StructDoc{
		Fields: map[string]string{
			"Title":   "The title.",
			"Summary": "The summary.",
		},
	})

	docs := modelFieldDocs(new(anonAliasPayload))
	if want := "The title."; docs["Title"] != want {
		t.Fatalf("docs[Title] = %q, want %q", docs["Title"], want)
	}
	if want := "The summary."; docs["Summary"] != want {
		t.Fatalf("docs[Summary] = %q, want %q", docs["Summary"], want)
	}
}

func TestModelFieldDocsIgnoresAmbiguousAnonymousSignature(t *testing.T) {
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

	docs := modelFieldDocs(new(ambiguousAnon))
	if len(docs) != 0 {
		t.Fatalf("docs = %v, want empty for ambiguous signature", docs)
	}
}
