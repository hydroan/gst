package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/codegen/constants"
	"github.com/stretchr/testify/require"
)

// TestGenRunAliasesCollidingImports runs gg gen against projects with a
// package whose name a framework import of a generated registration file, or
// another package that file imports, already takes. The generated file
// imports the package under an alias and registers through it, and the
// project builds.
func TestGenRunAliasesCollidingImports(t *testing.T) {
	modelFile := filepath.Join("model", constants.FileModelGen)
	serviceFile := filepath.Join("service", constants.FileServiceGen)
	routerFile := filepath.Join("router", constants.FileRouterGen)

	tests := []struct {
		name  string
		files map[string]string
		// want maps each generated file to the lines it must contain.
		want map[string][]string
	}{
		{
			// The service package of a model named Service is named service,
			// like the framework's service package.
			name: "service_package_named_service",
			files: map[string]string{
				"model/sample/service.go": `package sample

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Service struct {
	Name string ` + "`json:\"name\"`" + `

	model.Base
}

func (Service) TableName() string { return "sample_services" }

func (Service) Design() {
	dsl.Migrate()
	dsl.Endpoint("services")
	dsl.Create(func() {
		dsl.Service()
	})
}
`,
			},
			want: map[string][]string{
				serviceFile: {
					`sample_service "tmpapp/service/sample/service"`,
					`service.Register[*sample_service.Creator]`,
				},
			},
		},
		{
			// A package named model below the root model package is imported
			// by the model registration file, which already imports the
			// framework's model package.
			name: "model_package_below_the_root",
			files: map[string]string{
				"model/sample/model/entry.go": `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Entry struct {
	Name string ` + "`json:\"name\"`" + `

	model.Base
}

func (Entry) TableName() string { return "sample_entries" }

func (Entry) Design() {
	dsl.Migrate()
}
`,
			},
			want: map[string][]string{
				modelFile: {
					`sample_model "tmpapp/model/sample/model"`,
					`model.Register[*sample_model.Entry]()`,
				},
			},
		},
		{
			// The router registration file imports both the root model package
			// and a package named model below it.
			name: "routed_model_packages_sharing_a_name",
			files: map[string]string{
				"model/record.go": `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Empty
}

func (Record) Design() {
	dsl.Endpoint("records")
	dsl.Create(func() {})
}
`,
				"model/sample/model/item.go": `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Item struct {
	model.Empty
}

func (Item) Design() {
	dsl.Endpoint("items")
	dsl.Create(func() {})
}
`,
			},
			want: map[string][]string{
				routerFile: {
					`tmpapp_model "tmpapp/model"`,
					`sample_model "tmpapp/model/sample/model"`,
					`router.Register[*tmpapp_model.Record`,
					`router.Register[*sample_model.Item`,
				},
			},
		},
		{
			// Both service packages are named recorditem, though their
			// directories, named after the model files, differ.
			name: "service_packages_sharing_a_name_under_distinct_directories",
			files: map[string]string{
				"model/sample/record_item.go": `package sample

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type RecordItem struct {
	Name string ` + "`json:\"name\"`" + `

	model.Base
}

func (RecordItem) TableName() string { return "sample_record_items" }

func (RecordItem) Design() {
	dsl.Migrate()
	dsl.Endpoint("record-items")
	dsl.Create(func() {
		dsl.Service()
	})
}
`,
				"model/account/recorditem.go": `package account

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type RecordItem struct {
	Name string ` + "`json:\"name\"`" + `

	model.Base
}

func (RecordItem) TableName() string { return "account_record_items" }

func (RecordItem) Design() {
	dsl.Migrate()
	dsl.Endpoint("record-items")
	dsl.Create(func() {
		dsl.Service()
	})
}
`,
			},
			want: map[string][]string{
				serviceFile: {
					`sample_record_item "tmpapp/service/sample/record_item"`,
					`account_recorditem "tmpapp/service/account/recorditem"`,
					`service.Register[*sample_record_item.Creator]`,
					`service.Register[*account_recorditem.Creator]`,
				},
			},
		},
		{
			name: "model_packages_sharing_a_name",
			files: map[string]string{
				"model/tenant/sample/record.go": `package sample

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	Name string ` + "`json:\"name\"`" + `

	model.Base
}

func (Record) TableName() string { return "tenant_records" }

func (Record) Design() {
	dsl.Migrate()
}
`,
				"model/account/sample/entry.go": `package sample

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Entry struct {
	Name string ` + "`json:\"name\"`" + `

	model.Base
}

func (Entry) TableName() string { return "account_entries" }

func (Entry) Design() {
	dsl.Migrate()
}
`,
			},
			want: map[string][]string{
				modelFile: {
					`tenant_sample "tmpapp/model/tenant/sample"`,
					`account_sample "tmpapp/model/account/sample"`,
					`model.Register[*tenant_sample.Record]()`,
					`model.Register[*account_sample.Entry]()`,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectDir := newGenProject(t)
			for path, content := range tt.files {
				writeCheckFile(t, filepath.Join(projectDir, path), content)
			}

			require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
			for file, lines := range tt.want {
				code, err := os.ReadFile(file)
				require.NoError(t, err)
				for _, line := range lines {
					require.Contains(t, string(code), line)
				}
			}

			output, err := exec.Command("go", "build", "-mod=mod", "./...").CombinedOutput()
			require.NoError(t, err, "go build:\n%s", output)
		})
	}
}
