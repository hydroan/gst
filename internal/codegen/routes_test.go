package codegen_test

import (
	"maps"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen"
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

		codegen.ResolveRoutes(models, nil)

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

		result := codegen.ResolveRoutes(models, parseRules(t, "GET /api/samples/:sample/items"))

		want := []codegen.RouteIgnoreMatch{{Method: "GET", Path: "/api/samples/:sample/items", Model: "Item"}}
		if !slices.Equal(result.Matches, want) {
			t.Fatalf("Matches = %+v, want %+v", result.Matches, want)
		}
		remaining := remainingPhases(collectActions(findDesign(t, models, "Item")))
		if !slices.Equal(remaining, []consts.Phase{consts.PHASE_CREATE}) {
			t.Fatalf("remaining Item actions = %v, want only PHASE_CREATE", remaining)
		}
	})
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
		{name: "item action with a declared parameter", route: "samples", param: ":sample", phase: consts.PHASE_GET, wantRoute: "samples/:sample", wantParam: "sample"},
		{name: "item action without a declared parameter", route: "samples", phase: consts.PHASE_GET, wantRoute: "samples/:id", wantParam: "id"},
		{name: "collection action", route: "samples", param: ":sample", phase: consts.PHASE_LIST, wantRoute: "samples"},
		{name: "batch action", route: "samples", phase: consts.PHASE_CREATE_MANY, wantRoute: "samples/batch"},
		{name: "import action", route: "samples", phase: consts.PHASE_IMPORT, wantRoute: "samples/import"},
		{name: "export action", route: "samples", phase: consts.PHASE_EXPORT, wantRoute: "samples/export"},
		{name: "exact action", route: "iam/admin/users/:id/sessions", param: ":id", phase: consts.PHASE_DELETE, exact: true, wantRoute: "iam/admin/users/:id/sessions", wantParam: "id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			design := &dsl.Design{Param: tt.param}
			action := &dsl.Action{Phase: tt.phase}
			action.Exact = tt.exact

			route, paramName := codegen.RouterTargetForAction(tt.route, design, action)

			if route != tt.wantRoute {
				t.Fatalf("route = %q, want %q", route, tt.wantRoute)
			}
			if paramName != tt.wantParam {
				t.Fatalf("paramName = %q, want %q", paramName, tt.wantParam)
			}
		})
	}
}
