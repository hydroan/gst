package openapigen

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/apidoc"
	"github.com/hydroan/gst/types/consts"
)

func TestOperationIDDerivesFromPath(t *testing.T) {
	tests := []struct {
		path string
		op   consts.HTTPVerb
		want string
	}{
		{"/api/sample/records/{id}", consts.Patch, "sample_records_patch"},
		{"/api/groups", consts.List, "groups_list"},
		{"/api/hello-world", consts.Create, "hello_world_create"},
		{"/api/groups/{group}/items", consts.List, "groups_items_list"},
	}
	for _, tt := range tests {
		if got := operationID(tt.path, tt.op); got != tt.want {
			t.Fatalf("operationID(%s, %s) = %q, want %q", tt.path, tt.op, got, tt.want)
		}
	}
}

func TestTagsSkipPathParameters(t *testing.T) {
	got := tags("/api/{tenant}/groups", consts.List, reflect.TypeFor[*summaryFirstLineModel]())
	if len(got) != 1 || got[0] != "groups" {
		t.Fatalf("tags() = %v, want [groups]", got)
	}
}

type summaryFirstLineModel struct {
	Name string `json:"name"`
}

func init() {
	registerFixtureDoc("summaryFirstLineModel",
		"summaryFirstLineModel is the human readable summary line.\nThe second comment line must not leak into the summary.", nil)
}

func TestSummaryCombinesVerbAndStructCommentFirstLine(t *testing.T) {
	types := map[string]reflect.Type{
		"value":             reflect.TypeFor[summaryFirstLineModel](),
		"pointer":           reflect.TypeFor[*summaryFirstLineModel](),
		"slice":             reflect.TypeFor[[]summaryFirstLineModel](),
		"slice of pointers": reflect.TypeFor[[]*summaryFirstLineModel](),
	}

	for name, typ := range types {
		t.Run(name, func(t *testing.T) {
			got := summary("/api/sample/records", consts.Patch, typ, false)
			if got != "Patch The human readable summary line" {
				t.Fatalf("summary() = %q, want the verb plus the first comment line", got)
			}
		})
	}
}

func TestSummaryUsesTrailingActionSegmentForCustomTypes(t *testing.T) {
	typ := reflect.TypeFor[*summaryFirstLineModel]()
	got := summary("/api/users/{id}/disable", consts.Create, typ, true)
	if got != "Disable The human readable summary line" {
		t.Fatalf("summary() = %q, want the action segment plus the first comment line", got)
	}
}

func TestSummaryKeepsVerbForDefaultCRUDNestedCollection(t *testing.T) {
	typ := reflect.TypeFor[*summaryFirstLineModel]()
	got := summary("/api/tenants/{tenant}/users", consts.Create, typ, false)
	if got != "Create The human readable summary line" {
		t.Fatalf("summary() = %q, want the verb for a default CRUD nested collection", got)
	}
}

func TestSummaryFallsBackToPathSegments(t *testing.T) {
	typ := reflect.TypeOf(&struct{ Name string }{})
	got := summary("/api/sample/records/{id}", consts.Patch, typ, false)
	if got != "Patch sample records" {
		t.Fatalf("summary() = %q, want the path segment fallback", got)
	}
}

func TestDescriptionRemovesStructNameAndKeepsRemainingLines(t *testing.T) {
	want := "The human readable summary line.\nThe second comment line must not leak into the summary."
	types := map[string]reflect.Type{
		"value":             reflect.TypeFor[summaryFirstLineModel](),
		"pointer":           reflect.TypeFor[*summaryFirstLineModel](),
		"slice":             reflect.TypeFor[[]summaryFirstLineModel](),
		"slice of pointers": reflect.TypeFor[[]*summaryFirstLineModel](),
	}

	for name, typ := range types {
		t.Run(name, func(t *testing.T) {
			got := description("/api/sample/records", consts.Patch, typ, false)
			if got != want {
				t.Fatalf("description() = %q, want API-facing full struct comment", got)
			}
		})
	}
}

func TestSummaryAndDescriptionPreferRegisteredOperationDoc(t *testing.T) {
	apidoc.RegisterOperation("POST", "/api/override-users/:id/disable", apidoc.OperationDoc{
		Summary:     "The registered summary",
		Description: "The registered description.",
	})

	typ := reflect.TypeFor[*summaryFirstLineModel]()
	path := "/api/override-users/{id}/disable"
	if got := summary(path, consts.Create, typ, true); got != "The registered summary" {
		t.Fatalf("summary() = %q, want the registered override", got)
	}
	if got := description(path, consts.Create, typ, true); got != "The registered description." {
		t.Fatalf("description() = %q, want the registered override", got)
	}
}
