package modelinfo_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/modelinfo"
)

// TestScanModelsReadsTheModelsTheWayGenDoes pins what ScanModels hands
// back for a project of two models and a gst.yaml ignoring one route and
// one model: the routes resolved under the parent parameter, the ignored
// action dropped and reported, the ignored model kept off registration and
// reported, the module path carried along.
func TestScanModelsReadsTheModelsTheWayGenDoes(t *testing.T) {
	sources := map[string]string{
		filepath.Join("model", "sample.go"): `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	model.Base
}

func (Sample) TableName() string { return "samples" }

func (Sample) Design() {
	dsl.Migrate()
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
	dsl.List(func() {})
	dsl.Create(func() {})
}
`,
	}
	cfg := &ggconfig.Config{}
	cfg.Gen.Routes.Ignore = parseRules(t, "GET /api/samples/:sample/items")
	cfg.Gen.Models.Ignore = []ggconfig.ModelRule{{Name: "Sample", Raw: "Sample"}}

	scanned := scanModels(t, sources, cfg)

	if scanned.Module != "tmpapp" {
		t.Fatalf("Module = %q, want tmpapp", scanned.Module)
	}
	if got := findDesign(t, scanned.Models, "Item").Endpoint; got != "samples/:sample/items" {
		t.Fatalf("Item endpoint = %q, want samples/:sample/items", got)
	}
	if remaining := remainingPhases(collectActions(findDesign(t, scanned.Models, "Item"))); !slices.Equal(remaining, []consts.Phase{consts.Create}) {
		t.Fatalf("remaining Item actions = %v, want only Create", remaining)
	}
	if want := []modelinfo.RouteIgnoreMatch{{Method: "GET", Path: "/api/samples/:sample/items", Model: "Item"}}; !slices.Equal(scanned.RouteIgnores.Matches, want) {
		t.Fatalf("RouteIgnores.Matches = %+v, want %+v", scanned.RouteIgnores.Matches, want)
	}
	if want := []modelinfo.ModelIgnoreMatch{{Model: "Sample", File: "model/sample.go"}}; !slices.Equal(scanned.ModelIgnores.Matches, want) {
		t.Fatalf("ModelIgnores.Matches = %+v, want %+v", scanned.ModelIgnores.Matches, want)
	}
	if findDesign(t, scanned.Models, "Sample").Migrate {
		t.Fatal("the ignored model still migrates")
	}
}

// scanModels writes sources into a project directory, the way findModels
// does, and scans it with cfg.
func scanModels(t *testing.T, sources map[string]string, cfg *ggconfig.Config) modelinfo.ScannedModels {
	t.Helper()
	findModels(t, sources)
	scanned, err := modelinfo.ScanModels("tmpapp", "model", gghelper.NewProjectIgnore(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return scanned
}
