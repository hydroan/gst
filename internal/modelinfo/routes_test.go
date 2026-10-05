package modelinfo_test

import (
	"maps"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/modelinfo"
)

// TestResolveRoutesNestsEndpointsUnderParentParams pins the example of
// ResolveRoutes and propagateParentParams: the endpoints of model/sample.go,
// model/sample/item.go and model/sample/item/entry.go resolve to samples,
// samples/:sample/items and samples/:sample/items/:item/entries, and the rule
// GET /api/samples/:sample/items disables the Item List action alone.
func TestResolveRoutesNestsEndpointsUnderParentParams(t *testing.T) {
	sources := map[string]string{
		filepath.Join("model", "sample.go"): `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	model.Base
}

func (Sample) Design() {
	dsl.Endpoint("samples")
	dsl.Param("sample")
	dsl.List(func() {})
}
`,
		filepath.Join("model", "sample", "item.go"): `package sample

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Item struct {
	model.Base
}

func (Item) Design() {
	dsl.Endpoint("items")
	dsl.Param("item")
	dsl.List(func() {})
	dsl.Create(func() {})
}
`,
		filepath.Join("model", "sample", "item", "entry.go"): `package item

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Entry struct {
	model.Base
}

func (Entry) Design() {
	dsl.Endpoint("entries")
	dsl.List(func() {})
}
`,
	}

	t.Run("endpoints", func(t *testing.T) {
		models := findModels(t, sources)

		modelinfo.ResolveRoutes(models, nil)

		got := make(map[string]string, len(models))
		for _, m := range models {
			got[m.ModelName] = m.Design.Endpoint
		}
		want := map[string]string{
			"Sample": "samples",
			"Item":   "samples/:sample/items",
			"Entry":  "samples/:sample/items/:item/entries",
		}
		if !maps.Equal(got, want) {
			t.Fatalf("endpoints = %v, want %v", got, want)
		}
	})

	t.Run("ignore rule", func(t *testing.T) {
		models := findModels(t, sources)

		result := modelinfo.ResolveRoutes(models, parseRules(t, "GET /api/samples/:sample/items"))

		want := []modelinfo.RouteIgnoreMatch{{Method: "GET", Path: "/api/samples/:sample/items", Model: "Item"}}
		if !slices.Equal(result.Matches, want) {
			t.Fatalf("Matches = %+v, want %+v", result.Matches, want)
		}
		remaining := remainingPhases(collectActions(findDesign(t, models, "Item")))
		if !slices.Equal(remaining, []consts.Phase{consts.Create}) {
			t.Fatalf("remaining Item actions = %v, want only Create", remaining)
		}
	})
}

// TestItemParamDefaultsToID pins the examples of ItemParam: the parameter the
// design declares, or :id.
func TestItemParamDefaultsToID(t *testing.T) {
	if got := modelinfo.ItemParam(&dsl.Design{Param: ":sample"}); got != ":sample" {
		t.Fatalf("ItemParam(Param(\"sample\")) = %q, want %q", got, ":sample")
	}
	if got := modelinfo.ItemParam(&dsl.Design{}); got != ":id" {
		t.Fatalf("ItemParam(no Param) = %q, want %q", got, ":id")
	}
	if got := modelinfo.ItemParam(nil); got != ":id" {
		t.Fatalf("ItemParam(nil) = %q, want %q", got, ":id")
	}
}

// TestRouterTargetForAction pins the examples of RouterTargetForAction: item
// actions append the design's parameter or :id, batch, import and export
// actions append their segment, and an Exact action keeps the route it
// declares.
func TestRouterTargetForAction(t *testing.T) {
	tests := []struct {
		name      string
		route     string
		param     string
		phase     consts.Phase
		exact     bool
		wantRoute string
		wantParam string
	}{
		{name: "item action with a declared parameter", route: "samples", param: ":sample", phase: consts.Get, wantRoute: "/api/samples/:sample", wantParam: "sample"},
		{name: "item action without a declared parameter", route: "samples", phase: consts.Get, wantRoute: "/api/samples/:id", wantParam: "id"},
		{name: "collection action", route: "samples", param: ":sample", phase: consts.List, wantRoute: "/api/samples"},
		{name: "batch action", route: "samples", phase: consts.CreateMany, wantRoute: "/api/samples/batch"},
		{name: "import action", route: "samples", phase: consts.Import, wantRoute: "/api/samples/import"},
		{name: "export action", route: "samples", phase: consts.Export, wantRoute: "/api/samples/export"},
		{name: "exact action", route: "iam/admin/users/:id/sessions", param: ":id", phase: consts.Delete, exact: true, wantRoute: "/api/iam/admin/users/:id/sessions", wantParam: "id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			design := &dsl.Design{Param: tt.param}
			action := &dsl.Action{Phase: tt.phase}
			action.Exact = tt.exact

			route, paramName := modelinfo.RouterTargetForAction(tt.route, design, action)

			if route != tt.wantRoute {
				t.Fatalf("route = %q, want %q", route, tt.wantRoute)
			}
			if paramName != tt.wantParam {
				t.Fatalf("paramName = %q, want %q", paramName, tt.wantParam)
			}
		})
	}
}

// TestRouteConflictsNamesBothActionsOfAPath pins the example of
// RouteConflicts and the forms the router or the service registry would
// refuse at startup: two models resolving to one path, an Exact action on
// the path of another, SSE beside List, one Route block written twice, two
// Stream actions on one route, and two parameter names at one position of
// one method's paths; the batch path beside the item path, and a route
// naming the parameter the item path names, conflict with nothing.
func TestRouteConflictsNamesBothActionsOfAPath(t *testing.T) {
	model := func(pkg, name, design string) string {
		return "package " + pkg + `

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type ` + name + ` struct {
	model.Base
}

func (` + name + `) Design() {
` + design + `}
`
	}
	tests := []struct {
		name    string
		sources map[string]string
		want    []string
	}{
		{
			name: "two models resolving to one path",
			sources: map[string]string{
				filepath.Join("model", "token.go"):        model("model", "Token", "\tdsl.Endpoint(\"tokens\")\n\tdsl.List(func() {})\n"),
				filepath.Join("model", "api", "token.go"): model("api", "Token", "\tdsl.Endpoint(\"tokens\")\n\tdsl.List(func() {})\n"),
			},
			want: []string{"model/token.go: the List action of Token registers GET /api/tokens, as the List action of Token in model/api/token.go does; a path is served by one action"},
		},
		{
			name: "an Exact action on the path of another",
			sources: map[string]string{
				filepath.Join("model", "item.go"): model("model", "Item", "\tdsl.Endpoint(\"items\")\n\tdsl.List(func() {})\n\tdsl.Export(func() {\n\t\tdsl.Service()\n\t\tdsl.Exact()\n\t})\n"),
			},
			want: []string{"model/item.go: the Export action of Item registers GET /api/items, as the List action of Item in model/item.go does; a path is served by one action"},
		},
		{
			name: "SSE beside List",
			sources: map[string]string{
				filepath.Join("model", "record.go"): model("model", "Record", "\tdsl.Endpoint(\"records\")\n\tdsl.SSE(func() {\n\t\tdsl.Service()\n\t})\n\tdsl.List(func() {})\n"),
			},
			want: []string{"model/record.go: the SSE action of Record registers GET /api/records, as the List action of Record in model/record.go does; a path is served by one action"},
		},
		{
			name: "a Route block written twice",
			sources: map[string]string{
				filepath.Join("model", "item.go"): model("model", "Item", "\tdsl.Endpoint(\"items\")\n\tdsl.Route(\"items/archive\", func() {\n\t\tdsl.List(func() {})\n\t})\n\tdsl.Route(\"items/archive\", func() {\n\t\tdsl.List(func() {})\n\t})\n"),
			},
			want: []string{"model/item.go: the List action of Item registers GET /api/items/archive, as the List action of Item in model/item.go does; a path is served by one action"},
		},
		{
			name: "two Stream actions on one route",
			sources: map[string]string{
				filepath.Join("model", "feed.go"):        model("model", "Feed", "\tdsl.GRPC()\n\tdsl.Endpoint(\"feeds\")\n\tdsl.Route(\"feeds/chat\", func() {\n\t\tdsl.Stream(func() {\n\t\t\tdsl.Service(\"chat\")\n\t\t\tdsl.StreamingPayload[*FeedEvent]()\n\t\t\tdsl.StreamingResult[*FeedEvent]()\n\t\t})\n\t})\n"),
				filepath.Join("model", "api", "feed.go"): model("api", "Feed", "\tdsl.GRPC()\n\tdsl.Endpoint(\"feeds\")\n\tdsl.Route(\"feeds/chat\", func() {\n\t\tdsl.Stream(func() {\n\t\t\tdsl.Service(\"chat\")\n\t\t\tdsl.StreamingPayload[*FeedEvent]()\n\t\t\tdsl.StreamingResult[*FeedEvent]()\n\t\t})\n\t})\n"),
			},
			want: []string{"model/feed.go: the Stream action of Feed registers the stream on /api/feeds/chat, as the Stream action of Feed in model/api/feed.go does; a route serves one Stream action"},
		},
		{
			name: "two parameter names at one position",
			sources: map[string]string{
				filepath.Join("model", "record.go"): model("model", "Record", "\tdsl.Endpoint(\"records\")\n\tdsl.Get(func() {})\n\tdsl.Route(\"records/:record/items\", func() {\n\t\tdsl.List(func() {})\n\t})\n"),
			},
			want: []string{"model/record.go: the List action of Record registers GET /api/records/:record/items, naming the parameter :record where the Get action of Record in model/record.go, registering GET /api/records/:id, names :id; the router reads one parameter name at a position"},
		},
		{
			name: "the batch path beside the item path",
			sources: map[string]string{
				filepath.Join("model", "item.go"): model("model", "Item", "\tdsl.Endpoint(\"items\")\n\tdsl.Create(func() {})\n\tdsl.CreateMany(func() {})\n\tdsl.List(func() {})\n\tdsl.Get(func() {})\n\tdsl.Route(\"items/:id/notes\", func() {\n\t\tdsl.List(func() {})\n\t})\n"),
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			models := findModels(t, tt.sources)
			modelinfo.ResolveRoutes(models, nil)

			var got []string
			for _, err := range modelinfo.RouteConflicts(models) {
				got = append(got, err.Error())
			}

			if !slices.Equal(got, tt.want) {
				t.Fatalf("RouteConflicts = %q, want %q", got, tt.want)
			}
		})
	}
}
