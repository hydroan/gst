package openapigen

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/apidoc"
	"github.com/hydroan/gst/internal/consts"
)

func TestOperationIDDerivesFromPath(t *testing.T) {
	tests := []struct {
		path string
		op   consts.Phase
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
	got := tags("/api/{tenant}/groups", consts.List, reflect.TypeFor[*summarySentenceModel]())
	if len(got) != 1 || got[0] != "groups" {
		t.Fatalf("tags() = %v, want [groups]", got)
	}
}

type summarySentenceModel struct {
	Name string `json:"name"`
}

func init() {
	registerFixtureDoc("summarySentenceModel",
		"summarySentenceModel is the human readable summary\nsentence. The second sentence must not leak into the summary.", nil)
}

func TestSummaryCombinesPhaseAndStructCommentFirstSentence(t *testing.T) {
	types := map[string]reflect.Type{
		"value":             reflect.TypeFor[summarySentenceModel](),
		"pointer":           reflect.TypeFor[*summarySentenceModel](),
		"slice":             reflect.TypeFor[[]summarySentenceModel](),
		"slice of pointers": reflect.TypeFor[[]*summarySentenceModel](),
	}

	for name, typ := range types {
		t.Run(name, func(t *testing.T) {
			got := summary("/api/sample/records", consts.Patch, typ, false)
			if got != "Patch The human readable summary sentence" {
				t.Fatalf("summary() = %q, want the phase plus the first comment sentence", got)
			}
		})
	}
}

func TestSummaryUsesTrailingActionSegmentForCustomTypes(t *testing.T) {
	typ := reflect.TypeFor[*summarySentenceModel]()
	got := summary("/api/users/{id}/disable", consts.Create, typ, true)
	if got != "Disable The human readable summary sentence" {
		t.Fatalf("summary() = %q, want the action segment plus the first comment sentence", got)
	}
}

func TestSummaryKeepsPhaseForDefaultCRUDNestedCollection(t *testing.T) {
	typ := reflect.TypeFor[*summarySentenceModel]()
	got := summary("/api/tenants/{tenant}/users", consts.Create, typ, false)
	if got != "Create The human readable summary sentence" {
		t.Fatalf("summary() = %q, want the phase for a default CRUD nested collection", got)
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
	want := "The human readable summary\nsentence. The second sentence must not leak into the summary."
	types := map[string]reflect.Type{
		"value":             reflect.TypeFor[summarySentenceModel](),
		"pointer":           reflect.TypeFor[*summarySentenceModel](),
		"slice":             reflect.TypeFor[[]summarySentenceModel](),
		"slice of pointers": reflect.TypeFor[[]*summarySentenceModel](),
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

	typ := reflect.TypeFor[*summarySentenceModel]()
	path := "/api/override-users/{id}/disable"
	if got := summary(path, consts.Create, typ, true); got != "The registered summary" {
		t.Fatalf("summary() = %q, want the registered override", got)
	}
	if got := description(path, consts.Create, typ, true); got != "The registered description." {
		t.Fatalf("description() = %q, want the registered override", got)
	}
}
