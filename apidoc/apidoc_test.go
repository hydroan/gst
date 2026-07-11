package apidoc

import "testing"

func TestRegisterAndLookup(t *testing.T) {
	Register("example.com/demo/model", "User", StructDoc{
		Comment: "User is a demo model.",
		Fields: map[string]string{
			"Name": "Name is the user name.",
			"Age":  "Age is the user age.",
		},
	})

	doc, ok := Lookup("example.com/demo/model", "User")
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
	if _, ok := Lookup("example.com/demo/model", "NotRegistered"); ok {
		t.Fatal("Lookup() ok = true, want false for unregistered struct")
	}
}

func TestRegisterReplacesPreviousEntry(t *testing.T) {
	Register("example.com/demo/model", "Replaced", StructDoc{Comment: "old"})
	Register("example.com/demo/model", "Replaced", StructDoc{Comment: "new"})

	doc, ok := Lookup("example.com/demo/model", "Replaced")
	if !ok {
		t.Fatal("Lookup() ok = false, want true")
	}
	if doc.Comment != "new" {
		t.Fatalf("doc.Comment = %q, want %q", doc.Comment, "new")
	}
}

func TestRegisterAndLookupCopyFields(t *testing.T) {
	fields := map[string]string{"Name": "original"}
	Register("example.com/demo/model", "Isolated", StructDoc{Fields: fields})

	// Mutating the caller's map after Register must not affect the registry.
	fields["Name"] = "mutated by caller"

	doc, _ := Lookup("example.com/demo/model", "Isolated")
	if doc.Fields["Name"] != "original" {
		t.Fatalf("doc.Fields[Name] = %q, want %q", doc.Fields["Name"], "original")
	}

	// Mutating the looked-up map must not affect later lookups.
	doc.Fields["Name"] = "mutated by reader"
	again, _ := Lookup("example.com/demo/model", "Isolated")
	if again.Fields["Name"] != "original" {
		t.Fatalf("again.Fields[Name] = %q, want %q", again.Fields["Name"], "original")
	}
}
