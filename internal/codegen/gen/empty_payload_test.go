package gen_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/codegen/gen"
)

func TestRouterGstModelUse(t *testing.T) {
	subPkgListSource := `package sample

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

type RecordListRsp struct {
	Total int ` + "`json:\"total\"`" + `
}

func (Record) Design() {
	dsl.Endpoint("records")
	dsl.List(func() {
		dsl.Service()
		dsl.Result[*RecordListRsp]()
	})
}
`
	rootPkgListSource := `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Item struct {
	model.Base
}

type ItemListRsp struct {
	Total int ` + "`json:\"total\"`" + `
}

func (Item) Design() {
	dsl.Endpoint("items")
	dsl.List(func() {
		dsl.Service()
		dsl.Result[*ItemListRsp]()
	})
}
`
	subPkgCreateEmptyResultSource := `package sample

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

type RecordCreateReq struct {
	Name string ` + "`json:\"name\"`" + `
}

func (Record) Design() {
	dsl.Endpoint("records")
	dsl.Create(func() {
		dsl.Service()
		dsl.Payload[*RecordCreateReq]()
	})
}
`
	subPkgCreateBothSidesSource := `package sample

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

type RecordCreateReq struct {
	Name string ` + "`json:\"name\"`" + `
}

type RecordCreateRsp struct {
	ID string ` + "`json:\"id\"`" + `
}

func (Record) Design() {
	dsl.Endpoint("records")
	dsl.Create(func() {
		dsl.Service()
		dsl.Payload[*RecordCreateReq]()
		dsl.Result[*RecordCreateRsp]()
	})
}
`
	unroutedRootPkgSource := `package model

import "github.com/hydroan/gst/model"

type Snapshot struct {
	model.Base
}
`

	tests := []struct {
		name       string
		models     []*gen.ModelInfo
		wantPkg    string
		wantNeeded bool
	}{
		{
			name:       "sub_package_empty_payload_uses_plain_model_qualifier",
			models:     modelInfosFromSource(t, "sample", "record.go", subPkgListSource),
			wantPkg:    "model",
			wantNeeded: true,
		},
		{
			name:       "routed_root_model_package_falls_back_to_gstmodel_alias",
			models:     modelInfosFromSource(t, "", "item.go", rootPkgListSource),
			wantPkg:    "gstmodel",
			wantNeeded: true,
		},
		{
			name:       "defaulted_empty_result_alone_still_needs_the_import",
			models:     modelInfosFromSource(t, "sample", "record.go", subPkgCreateEmptyResultSource),
			wantPkg:    "model",
			wantNeeded: true,
		},
		{
			name:       "no_empty_side_leaves_the_import_out",
			models:     modelInfosFromSource(t, "sample", "record.go", subPkgCreateBothSidesSource),
			wantPkg:    "model",
			wantNeeded: false,
		},
		{
			name: "unrouted_root_model_file_does_not_force_the_alias",
			models: append(
				modelInfosFromSource(t, "", "snapshot.go", unroutedRootPkgSource),
				modelInfosFromSource(t, "sample", "record.go", subPkgListSource)...,
			),
			wantPkg:    "model",
			wantNeeded: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPkg, gotNeeded := gen.RouterGstModelUse(tt.models)
			if gotPkg != tt.wantPkg {
				t.Errorf("RouterGstModelUse() pkgName = %q, want %q", gotPkg, tt.wantPkg)
			}
			if gotNeeded != tt.wantNeeded {
				t.Errorf("RouterGstModelUse() needed = %v, want %v", gotNeeded, tt.wantNeeded)
			}
		})
	}
}

func TestGstModelImportEntry(t *testing.T) {
	tests := []struct {
		name    string
		pkgName string
		want    string
	}{
		{
			name:    "plain_qualifier_imports_without_alias",
			pkgName: "model",
			want:    "github.com/hydroan/gst/model",
		},
		{
			name:    "gstmodel_qualifier_imports_under_the_alias",
			pkgName: "gstmodel",
			want:    "gstmodel github.com/hydroan/gst/model",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gen.GstModelImportEntry(tt.pkgName); got != tt.want {
				t.Errorf("GstModelImportEntry(%q) = %q, want %q", tt.pkgName, got, tt.want)
			}
		})
	}
}

// modelInfosFromSource writes source into a temporary model package directory
// and scans it with FindModels, so Design values are built by the DSL parser:
// a hand-built dsl.Design leaves undeclared action pointers nil, which panics
// inside dsl.Design.Range. pkgDir is relative to the model directory; an
// empty pkgDir places the file in the model root package.
func modelInfosFromSource(t *testing.T, pkgDir, filename, source string) []*gen.ModelInfo {
	t.Helper()
	modelDir := filepath.Join(t.TempDir(), "model")
	fixtureDir := filepath.Join(modelDir, pkgDir)
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(fixtureDir, filename)
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	models, err := gen.FindModels("tmpapp", modelDir, path)
	if err != nil {
		t.Fatal(err)
	}
	return models
}
