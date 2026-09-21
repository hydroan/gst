package apidoc_test

import (
	"testing"

	"github.com/hydroan/gst/apidoc"
)

func TestRegisterAndLookup(t *testing.T) {
	apidoc.Register("example.com/demo/model", "User", apidoc.StructDoc{
		Comment: "User is a demo model.",
		Fields: map[string]string{
			"Name": "Name is the user name.",
			"Age":  "Age is the user age.",
		},
	})

	doc, ok := apidoc.Lookup("example.com/demo/model", "User")
	if !ok {
		t.Fatal("Lookup() ok = false, want true")
	}
	if doc.Comment != "User is a demo model." {
		t.Fatalf("doc.Comment = %q, want %q", doc.Comment, "User is a demo model.")
	}
	if doc.Fields["Name"] != "Name is the user name." {
		t.Fatalf("doc.Fields[Name] = %q, want %q", doc.Fields["Name"], "Name is the user name.")
	}
}

func TestLookupMissing(t *testing.T) {
	if _, ok := apidoc.Lookup("example.com/demo/model", "NotRegistered"); ok {
		t.Fatal("Lookup() ok = true, want false for unregistered struct")
	}
}

func TestRegisterReplacesPreviousEntry(t *testing.T) {
	apidoc.Register("example.com/demo/model", "Replaced", apidoc.StructDoc{Comment: "old"})
	apidoc.Register("example.com/demo/model", "Replaced", apidoc.StructDoc{Comment: "new"})

	doc, ok := apidoc.Lookup("example.com/demo/model", "Replaced")
	if !ok {
		t.Fatal("Lookup() ok = false, want true")
	}
	if doc.Comment != "new" {
		t.Fatalf("doc.Comment = %q, want %q", doc.Comment, "new")
	}
}

func TestRegisterEnumAndLookupEnum(t *testing.T) {
	apidoc.RegisterEnum("example.com/demo/model", "Status", apidoc.EnumDoc{
		Comment: "Status is a demo enum.",
		Values: []apidoc.EnumValue{
			{Value: "active", Comment: "the record is active"},
			{Value: "disabled", Comment: "the record is disabled"},
		},
	})

	doc, ok := apidoc.LookupEnum("example.com/demo/model", "Status")
	if !ok {
		t.Fatal("LookupEnum() ok = false, want true")
	}
	if doc.Comment != "Status is a demo enum." {
		t.Fatalf("doc.Comment = %q, want %q", doc.Comment, "Status is a demo enum.")
	}
	if len(doc.Values) != 2 || doc.Values[0].Value != "active" || doc.Values[1].Comment != "the record is disabled" {
		t.Fatalf("doc.Values = %#v, want the two registered values in order", doc.Values)
	}

	if _, ok := apidoc.LookupEnum("example.com/demo/model", "NotRegistered"); ok {
		t.Fatal("LookupEnum() ok = true, want false for unregistered enum")
	}
}

func TestRegisterEnumAndLookupEnumCopyValues(t *testing.T) {
	values := []apidoc.EnumValue{{Value: "a", Comment: "original"}}
	apidoc.RegisterEnum("example.com/demo/model", "Isolated", apidoc.EnumDoc{Values: values})

	// Mutating the caller's slice after RegisterEnum must not affect the registry.
	values[0].Comment = "mutated by caller"

	doc, _ := apidoc.LookupEnum("example.com/demo/model", "Isolated")
	if doc.Values[0].Comment != "original" {
		t.Fatalf("doc.Values[0].Comment = %q, want %q", doc.Values[0].Comment, "original")
	}

	// Mutating the looked-up slice must not affect later lookups.
	doc.Values[0].Comment = "mutated by reader"
	again, _ := apidoc.LookupEnum("example.com/demo/model", "Isolated")
	if again.Values[0].Comment != "original" {
		t.Fatalf("again.Values[0].Comment = %q, want %q", again.Values[0].Comment, "original")
	}
}

func TestRegisterAndLookupCopyFields(t *testing.T) {
	fields := map[string]string{"Name": "original"}
	apidoc.Register("example.com/demo/model", "Isolated", apidoc.StructDoc{Fields: fields})

	// Mutating the caller's map after Register must not affect the registry.
	fields["Name"] = "mutated by caller"

	doc, _ := apidoc.Lookup("example.com/demo/model", "Isolated")
	if doc.Fields["Name"] != "original" {
		t.Fatalf("doc.Fields[Name] = %q, want %q", doc.Fields["Name"], "original")
	}

	// Mutating the looked-up map must not affect later lookups.
	doc.Fields["Name"] = "mutated by reader"
	again, _ := apidoc.Lookup("example.com/demo/model", "Isolated")
	if again.Fields["Name"] != "original" {
		t.Fatalf("again.Fields[Name] = %q, want %q", again.Fields["Name"], "original")
	}
}
