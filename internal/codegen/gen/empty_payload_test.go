package gen

import (
	"os"
	"path/filepath"
	"testing"
)

// modelInfosFromSource writes source into a temporary model package directory
// and scans it with FindModels, so Design values are built by the DSL parser:
// a hand-built dsl.Design leaves undeclared action pointers nil, which panics
// inside dsl.Design.Range. pkgDir is relative to the model directory; an
// empty pkgDir places the file in the model root package.
func modelInfosFromSource(t *testing.T, pkgDir, filename, source string) []*ModelInfo {
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

	models, err := FindModels("tmpapp", modelDir, path)
	if err != nil {
		t.Fatal(err)
	}
	return models
}

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
		models     []*ModelInfo
		wantPkg    string
		wantNeeded bool
	}{
		{
			name:       "sub package empty payload uses plain model qualifier",
			models:     modelInfosFromSource(t, "sample", "record.go", subPkgListSource),
			wantPkg:    "model",
			wantNeeded: true,
		},
		{
			name:       "routed root model package falls back to gstmodel alias",
			models:     modelInfosFromSource(t, "", "item.go", rootPkgListSource),
			wantPkg:    "gstmodel",
			wantNeeded: true,
		},
		{
			name:       "defaulted empty result alone still needs the import",
			models:     modelInfosFromSource(t, "sample", "record.go", subPkgCreateEmptyResultSource),
			wantPkg:    "model",
			wantNeeded: true,
		},
		{
			name:       "no empty side leaves the import out",
			models:     modelInfosFromSource(t, "sample", "record.go", subPkgCreateBothSidesSource),
			wantPkg:    "model",
			wantNeeded: false,
		},
		{
			name: "unrouted root model file does not force the alias",
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
			gotPkg, gotNeeded := RouterGstModelUse(tt.models)
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
			name:    "plain qualifier imports without alias",
			pkgName: "model",
			want:    "github.com/hydroan/gst/model",
		},
		{
			name:    "gstmodel qualifier imports under the alias",
			pkgName: "gstmodel",
			want:    "gstmodel github.com/hydroan/gst/model",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GstModelImportEntry(tt.pkgName); got != tt.want {
				t.Errorf("GstModelImportEntry(%q) = %q, want %q", tt.pkgName, got, tt.want)
			}
		})
	}
}
