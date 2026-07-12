package apidoc

import (
	"testing"

	"github.com/hydroan/gst/types/consts"
)

func TestRegisterOperationAndLookupOperation(t *testing.T) {
	RegisterOperation("POST", "/api/users/{id}/disable", OperationDoc{
		Summary:     "Disable the user",
		Description: "Disable the user and revoke all of its sessions.",
	})

	doc, ok := LookupOperation("POST", "/api/users/{id}/disable")
	if !ok {
		t.Fatal("LookupOperation() ok = false, want true")
	}
	if doc.Summary != "Disable the user" {
		t.Fatalf("doc.Summary = %q, want %q", doc.Summary, "Disable the user")
	}
	if doc.Description != "Disable the user and revoke all of its sessions." {
		t.Fatalf("doc.Description = %q, want the registered description", doc.Description)
	}
}

func TestLookupOperationNormalizesMethodAndParamStyle(t *testing.T) {
	// Registering with gin-style ":id" params and a lowercase method must be
	// found by a lookup that uses OpenAPI-style "{id}" params, and vice versa.
	RegisterOperation("post", "/api/users/:id/enable", OperationDoc{Summary: "Enable the user"})

	doc, ok := LookupOperation("POST", "/api/users/{id}/enable")
	if !ok {
		t.Fatal("LookupOperation() ok = false, want true for equivalent param styles")
	}
	if doc.Summary != "Enable the user" {
		t.Fatalf("doc.Summary = %q, want %q", doc.Summary, "Enable the user")
	}
}

func TestLookupOperationMissing(t *testing.T) {
	if _, ok := LookupOperation("GET", "/api/not/registered"); ok {
		t.Fatal("LookupOperation() ok = true, want false for unregistered operation")
	}
}

func TestRegisterOperationReplacesPreviousEntry(t *testing.T) {
	RegisterOperation("PUT", "/api/replaced", OperationDoc{Summary: "old"})
	RegisterOperation("PUT", "/api/replaced", OperationDoc{Summary: "new"})

	doc, ok := LookupOperation("PUT", "/api/replaced")
	if !ok {
		t.Fatal("LookupOperation() ok = false, want true")
	}
	if doc.Summary != "new" {
		t.Fatalf("doc.Summary = %q, want %q", doc.Summary, "new")
	}
}

func TestDefaultSummary(t *testing.T) {
	tests := []struct {
		name string
		op   Operation
		want string
	}{
		{
			name: "verb with model comment",
			op: Operation{
				Path:         "/api/users",
				Verb:         consts.List,
				ModelComment: "The user record.",
			},
			want: "List The user record",
		},
		{
			name: "trailing Chinese period of the comment line is trimmed",
			op: Operation{
				Path:         "/api/users",
				Verb:         consts.Create,
				ModelComment: "用户。",
			},
			want: "Create 用户",
		},
		{
			name: "only the first comment line is used",
			op: Operation{
				Path:         "/api/users/{id}",
				Verb:         consts.Update,
				ModelComment: "The user record.\nThe second line must not leak into the summary.",
			},
			want: "Update The user record",
		},
		{
			name: "many verb becomes a batch action",
			op: Operation{
				Path:         "/api/users/batch",
				Verb:         consts.CreateMany,
				ModelComment: "The user record.",
			},
			want: "Batch Create The user record",
		},
		{
			name: "trailing action segment after a path param wins over the verb",
			op: Operation{
				Path:         "/api/users/{id}/disable",
				Verb:         consts.Create,
				CustomTypes:  true,
				ModelComment: "The user record.",
			},
			want: "Disable The user record",
		},
		{
			name: "gin-style trailing action segment",
			op: Operation{
				Path:         "/api/users/:id/reset_password",
				Verb:         consts.Create,
				CustomTypes:  true,
				ModelComment: "The user record.",
			},
			want: "Reset Password The user record",
		},
		{
			name: "default CRUD nested collection route keeps the verb",
			op: Operation{
				Path:         "/api/tenants/{tenant}/users",
				Verb:         consts.Create,
				ModelComment: "The user record.",
			},
			want: "Create The user record",
		},
		{
			name: "custom list route keeps the verb",
			op: Operation{
				Path:         "/api/tenants/{tenant}/users",
				Verb:         consts.List,
				CustomTypes:  true,
				ModelComment: "The user record.",
			},
			want: "List The user record",
		},
		{
			name: "no comment falls back to resource path segments",
			op: Operation{
				Path: "/api/play/customizations/{id}",
				Verb: consts.Patch,
			},
			want: "Patch play customizations",
		},
		{
			name: "no comment with a trailing action segment does not repeat the action",
			op: Operation{
				Path:        "/api/users/{id}/disable",
				Verb:        consts.Create,
				CustomTypes: true,
			},
			want: "Disable users",
		},
		{
			name: "no comment and no resource segments falls back to the model name",
			op: Operation{
				Path:      "/api",
				Verb:      consts.Get,
				ModelName: "User",
			},
			want: "Get User",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DefaultSummary(tt.op); got != tt.want {
				t.Errorf("DefaultSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultDescription(t *testing.T) {
	t.Run("uses the full model comment", func(t *testing.T) {
		op := Operation{
			Path:         "/api/users",
			Verb:         consts.List,
			ModelComment: "The user record.\nIt keeps the account and status fields.",
		}
		want := "The user record.\nIt keeps the account and status fields."
		if got := DefaultDescription(op); got != want {
			t.Errorf("DefaultDescription() = %q, want the full comment", got)
		}
	})

	t.Run("falls back to the default summary without a comment", func(t *testing.T) {
		op := Operation{
			Path: "/api/play/customizations/{id}",
			Verb: consts.Patch,
		}
		if got := DefaultDescription(op); got != DefaultSummary(op) {
			t.Errorf("DefaultDescription() = %q, want DefaultSummary fallback", got)
		}
	})
}
