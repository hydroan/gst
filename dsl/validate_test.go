package dsl_test

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/hydroan/gst/dsl"
)

func TestValidateFlattenUsage(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		modelDir  string
		filename  string
		wantError string
	}{
		{
			name:     "valid_route_action",
			source:   validateNestedModelSource,
			modelDir: "/repo/model",
			filename: "/repo/model/authz/role.go",
		},
		{
			name:      "flatten_on_root_model_file",
			source:    validateRootModelSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/role.go",
			wantError: "root model file",
		},
		{
			name:     "framework_module_package_scan_is_not_root_model_file",
			source:   validateFrameworkModuleModelSource,
			modelDir: "/repo/internal/model/authz",
			filename: "/repo/internal/model/authz/role.go",
		},
		{
			name:      "flatten_outside_action",
			source:    validateFlattenTopLevelSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/authz/role.go",
			wantError: "Flatten() can only be used inside an action block",
		},
		{
			name:      "flatten_missing_filename",
			source:    validateFlattenMissingFilenameSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/authz/role.go",
			wantError: "missing Filename",
		},
		{
			name:      "flatten_without_service",
			source:    validateFlattenWithoutServiceSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/authz/role.go",
			wantError: "does not enable Service()",
		},
		{
			name:      "service_outside_action",
			source:    validateServiceTopLevelSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/authz/role.go",
			wantError: "Service() can only be used inside an action block",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, tt.filename, tt.source, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse source failed: %v", err)
			}

			errs := dsl.Validate(file, tt.modelDir, tt.filename)
			if tt.wantError == "" {
				if len(errs) != 0 {
					t.Fatalf("Validate returned errors: %v", errs)
				}
				return
			}
			if len(errs) == 0 {
				t.Fatalf("Validate returned no errors, want %q", tt.wantError)
			}
			var got strings.Builder
			for _, err := range errs {
				got.WriteString(err.Error())
				got.WriteString("\n")
			}
			if !strings.Contains(got.String(), tt.wantError) {
				t.Fatalf("Validate errors = %q, want substring %q", got.String(), tt.wantError)
			}
		})
	}
}

const validateNestedModelSource = `
package authz

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Role struct {
	model.Base
}

func (Role) Design() {
	Route("authz/roles", func() {
		Create(func() {
			Service()
			Filename("role.go")
			Flatten()
		})
	})
}
`

const validateRootModelSource = `
package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Role struct {
	model.Base
}

func (Role) Design() {
	Create(func() {
		Service()
		Filename("role.go")
		Flatten()
	})
}
`

const validateFrameworkModuleModelSource = `
package modelauthz

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Role struct {
	model.Base
}

func (Role) Design() {
	dsl.Route("authz/roles", func() {
		dsl.Create(func() {
			dsl.Service()
			dsl.Filename("role.go")
			dsl.Flatten()
		})
	})
}
`

const validateFlattenTopLevelSource = `
package authz

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Role struct {
	model.Base
}

func (Role) Design() {
	Flatten()
}
`

const validateFlattenMissingFilenameSource = `
package authz

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Role struct {
	model.Base
}

func (Role) Design() {
	Create(func() {
		Service()
		Flatten()
	})
}
`

const validateFlattenWithoutServiceSource = `
package authz

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Role struct {
	model.Base
}

func (Role) Design() {
	Create(func() {
		Filename("role.go")
		Flatten()
	})
}
`

const validateServiceTopLevelSource = `
package authz

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Role struct {
	model.Base
}

func (Role) Design() {
	Service()
}
`

func TestValidateExactUsage(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		modelDir  string
		filename  string
		wantError string
	}{
		{
			name:     "exact_delete_with_payload_and_result",
			source:   validateExactDeleteWithPayloadSource,
			modelDir: "/repo/model",
			filename: "/repo/model/iam/session.go",
		},
		{
			name:      "exact_delete_without_payload_or_result",
			source:    validateExactDeleteWithoutPayloadSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/iam/session.go",
			wantError: "uses dsl.Exact() but relies on the built-in controller",
		},
		{
			name:      "exact_get_in_route_block_without_payload_or_result",
			source:    validateExactGetWithoutPayloadSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/iam/session.go",
			wantError: "uses dsl.Exact() but relies on the built-in controller",
		},
		{
			name:     "exact_list_without_payload_or_result",
			source:   validateExactListSource,
			modelDir: "/repo/model",
			filename: "/repo/model/iam/session.go",
		},
		{
			name:     "exact_get_with_result_only",
			source:   validateExactGetWithResultSource,
			modelDir: "/repo/model",
			filename: "/repo/model/iam/session.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, tt.filename, tt.source, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse source failed: %v", err)
			}

			errs := dsl.Validate(file, tt.modelDir, tt.filename)
			if tt.wantError == "" {
				if len(errs) != 0 {
					t.Fatalf("Validate returned errors: %v", errs)
				}
				return
			}
			if len(errs) == 0 {
				t.Fatalf("Validate returned no errors, want %q", tt.wantError)
			}
			var got strings.Builder
			for _, err := range errs {
				got.WriteString(err.Error())
				got.WriteString("\n")
			}
			if !strings.Contains(got.String(), tt.wantError) {
				t.Fatalf("Validate errors = %q, want substring %q", got.String(), tt.wantError)
			}
		})
	}
}

const validateExactDeleteWithPayloadSource = `
package iam

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Session struct {
	model.Base
}

func (Session) Design() {
	Route("iam/sessions", func() {
		Delete(func() {
			Service()
			Exact()
			Payload[*SessionDeleteReq]()
			Result[*SessionDeleteRsp]()
		})
	})
}
`

const validateExactDeleteWithoutPayloadSource = `
package iam

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Session struct {
	model.Base
}

func (Session) Design() {
	Delete(func() {
		Service()
		Exact()
	})
}
`

const validateExactGetWithoutPayloadSource = `
package iam

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Session struct {
	model.Base
}

func (Session) Design() {
	Route("iam/sessions/current", func() {
		Get(func() {
			Service()
			Exact()
		})
	})
}
`

const validateExactListSource = `
package iam

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Session struct {
	model.Base
}

func (Session) Design() {
	List(func() {
		Service()
		Exact()
	})
}
`

const validateExactGetWithResultSource = `
package iam

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Session struct {
	model.Base
}

func (Session) Design() {
	Route("iam/sessions/current", func() {
		Get(func() {
			Service()
			Exact()
			Result[*CurrentGetRsp]()
		})
	})
}
`

func TestValidateListGetPayloadUsage(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		modelDir  string
		filename  string
		wantError string
	}{
		{
			name:      "payload_on_list_action",
			source:    validatePayloadOnListSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/iam/session.go",
			wantError: "List action handles an HTTP GET request and cannot declare Payload",
		},
		{
			name:      "payload_on_get_action_in_route_block",
			source:    validatePayloadOnGetInRouteSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/iam/session.go",
			wantError: "Get action handles an HTTP GET request and cannot declare Payload",
		},
		{
			name:     "result_only_on_list_action",
			source:   validateResultOnlyOnListSource,
			modelDir: "/repo/model",
			filename: "/repo/model/iam/session.go",
		},
		{
			name:     "payload_on_create_action",
			source:   validatePayloadOnCreateSource,
			modelDir: "/repo/model",
			filename: "/repo/model/iam/session.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, tt.filename, tt.source, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse source failed: %v", err)
			}

			errs := dsl.Validate(file, tt.modelDir, tt.filename)
			if tt.wantError == "" {
				if len(errs) != 0 {
					t.Fatalf("Validate returned errors: %v", errs)
				}
				return
			}
			if len(errs) == 0 {
				t.Fatalf("Validate returned no errors, want %q", tt.wantError)
			}
			var got strings.Builder
			for _, err := range errs {
				got.WriteString(err.Error())
				got.WriteString("\n")
			}
			if !strings.Contains(got.String(), tt.wantError) {
				t.Fatalf("Validate errors = %q, want substring %q", got.String(), tt.wantError)
			}
		})
	}
}

const validatePayloadOnListSource = `
package iam

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Session struct {
	model.Base
}

func (Session) Design() {
	List(func() {
		Service()
		Payload[*SessionListReq]()
		Result[*SessionListRsp]()
	})
}
`

const validatePayloadOnGetInRouteSource = `
package iam

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Session struct {
	model.Base
}

func (Session) Design() {
	Route("iam/sessions/current", func() {
		Get(func() {
			Service()
			Payload[*CurrentGetReq]()
			Result[*CurrentGetRsp]()
		})
	})
}
`

const validateResultOnlyOnListSource = `
package iam

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Session struct {
	model.Base
}

func (Session) Design() {
	List(func() {
		Service()
		Result[*SessionListRsp]()
	})
}
`

const validatePayloadOnCreateSource = `
package iam

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Session struct {
	model.Base
}

func (Session) Design() {
	Create(func() {
		Service()
		Payload[*SessionCreateReq]()
		Result[*SessionCreateRsp]()
	})
}
`

func TestValidateImportExportPayloadResultUsage(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		modelDir  string
		filename  string
		wantError string
	}{
		{
			name:      "payload_on_export_action",
			source:    validatePayloadOnExportSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/sample/record.go",
			wantError: "Export action delegates to the fixed service method Export(ctx, ...M) ([]byte, error) and cannot declare Payload",
		},
		{
			name:      "result_on_export_action",
			source:    validateResultOnExportSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/sample/record.go",
			wantError: "Export action delegates to the fixed service method Export(ctx, ...M) ([]byte, error) and cannot declare Result",
		},
		{
			name:      "payload_on_import_action_in_route_block",
			source:    validatePayloadOnImportInRouteSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/sample/record.go",
			wantError: "Import action delegates to the fixed service method Import(ctx, io.Reader) ([]M, error) and cannot declare Payload",
		},
		{
			name:      "result_on_import_action",
			source:    validateResultOnImportSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/sample/record.go",
			wantError: "Import action delegates to the fixed service method Import(ctx, io.Reader) ([]M, error) and cannot declare Result",
		},
		{
			name:      "import_and_export_without_service",
			source:    validateEnabledOnlyImportExportSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/sample/record.go",
			wantError: "action has no built-in implementation and must declare Service()",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, tt.filename, tt.source, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse source failed: %v", err)
			}

			errs := dsl.Validate(file, tt.modelDir, tt.filename)
			if tt.wantError == "" {
				if len(errs) != 0 {
					t.Fatalf("Validate returned errors: %v", errs)
				}
				return
			}
			if len(errs) == 0 {
				t.Fatalf("Validate returned no errors, want %q", tt.wantError)
			}
			var got strings.Builder
			for _, err := range errs {
				got.WriteString(err.Error())
				got.WriteString("\n")
			}
			if !strings.Contains(got.String(), tt.wantError) {
				t.Fatalf("Validate errors = %q, want substring %q", got.String(), tt.wantError)
			}
		})
	}
}

const validatePayloadOnExportSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Export(func() {
		Service()
		Payload[*RecordExportReq]()
	})
}
`

const validateResultOnExportSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Export(func() {
		Service()
		Result[*RecordExportRsp]()
	})
}
`

const validatePayloadOnImportInRouteSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Route("sample/records", func() {
		Import(func() {
			Service()
			Payload[*RecordImportReq]()
		})
	})
}
`

const validateResultOnImportSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Import(func() {
		Service()
		Result[*RecordImportRsp]()
	})
}
`

const validateEnabledOnlyImportExportSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Import(func() {
		Enabled(true)
	})
	Export(func() {
		Enabled(true)
	})
}
`

func TestValidateServiceFilenameCollision(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		modelDir  string
		filename  string
		wantError string
	}{
		{
			name:      "two_route_actions_share_one_filename",
			source:    validateSharedFilenameRouteActionsSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/sample/record.go",
			wantError: `service file "shared.go" is generated by multiple actions: Get on Record (route "sample/detail"), List on Record (route "sample/list")`,
		},
		{
			name:      "explicit_filename_collides_with_phase_default_filename",
			source:    validateSharedDefaultFilenameSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/sample/record.go",
			wantError: `service file "get.go" is generated by multiple actions: Get on Record, Patch on Record (route "sample/detail")`,
		},
		{
			name:      "two_models_in_one_file_share_the_default_filename",
			source:    validateTwoModelsDefaultFilenameSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/sample/record.go",
			wantError: `service file "get.go" is generated by multiple actions: Get on Item, Get on Record`,
		},
		{
			name:      "both_flatten_actions_share_one_filename",
			source:    validateSharedFilenameBothFlattenSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/sample/record.go",
			wantError: `service file "shared.go" is generated by multiple actions`,
		},
		{
			name:     "same_filename_with_flatten_and_non-flatten_targets_different_dirs",
			source:   validateSharedFilenameFlattenMixSource,
			modelDir: "/repo/model",
			filename: "/repo/model/sample/record.go",
		},
		{
			name:     "distinct_filenames_per_action",
			source:   validateDistinctFilenamesSource,
			modelDir: "/repo/model",
			filename: "/repo/model/sample/record.go",
		},
		{
			name:     "action_without_service_does_not_generate_a_file",
			source:   validateSharedFilenameWithoutServiceSource,
			modelDir: "/repo/model",
			filename: "/repo/model/sample/record.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, tt.filename, tt.source, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse source failed: %v", err)
			}

			errs := dsl.Validate(file, tt.modelDir, tt.filename)
			if tt.wantError == "" {
				if len(errs) != 0 {
					t.Fatalf("Validate returned errors: %v", errs)
				}
				return
			}
			if len(errs) == 0 {
				t.Fatalf("Validate returned no errors, want %q", tt.wantError)
			}
			var got strings.Builder
			for _, err := range errs {
				got.WriteString(err.Error())
				got.WriteString("\n")
			}
			if !strings.Contains(got.String(), tt.wantError) {
				t.Fatalf("Validate errors = %q, want substring %q", got.String(), tt.wantError)
			}
		})
	}
}

const validateSharedFilenameRouteActionsSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Route("sample/detail", func() {
		Get(func() {
			Service()
			Exact()
			Filename("shared.go")
			Result[*DetailGetRsp]()
		})
	})
	Route("sample/list", func() {
		List(func() {
			Service()
			Filename("shared.go")
			Result[*RecordListRsp]()
		})
	})
}
`

const validateSharedDefaultFilenameSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Get(func() {
		Service()
	})
	Route("sample/detail", func() {
		Patch(func() {
			Service()
			Filename("get.go")
			Payload[*DetailPatchReq]()
			Result[*DetailPatchRsp]()
		})
	})
}
`

const validateTwoModelsDefaultFilenameSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Get(func() {
		Service()
	})
}

type Item struct {
	model.Base
}

func (Item) Design() {
	Get(func() {
		Service()
	})
}
`

const validateSharedFilenameBothFlattenSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Route("sample/archive", func() {
		Create(func() {
			Service()
			Filename("shared.go")
			Flatten()
		})
	})
	Route("sample/restore", func() {
		Update(func() {
			Service()
			Filename("shared.go")
			Flatten()
		})
	})
}
`

const validateSharedFilenameFlattenMixSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Route("sample/archive", func() {
		Create(func() {
			Service()
			Filename("shared.go")
			Flatten()
		})
	})
	Route("sample/restore", func() {
		Update(func() {
			Service()
			Filename("shared.go")
		})
	})
}
`

const validateDistinctFilenamesSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Route("sample/detail", func() {
		Get(func() {
			Service()
			Exact()
			Filename("detail.go")
			Result[*DetailGetRsp]()
		})
	})
	Route("sample/list", func() {
		List(func() {
			Service()
			Filename("list.go")
			Result[*RecordListRsp]()
		})
	})
}
`

const validateSharedFilenameWithoutServiceSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Route("sample/detail", func() {
		Get(func() {
			Service()
			Exact()
			Filename("shared.go")
			Result[*DetailGetRsp]()
		})
	})
	Route("sample/list", func() {
		List(func() {
			Filename("shared.go")
		})
	})
}
`

func TestValidateVirtualModelListResult(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError string
	}{
		{
			name:   "virtual_model_list_with_result_passes",
			source: validateVirtualListWithResultSource,
		},
		{
			name:      "virtual_model_list_without_result_is_rejected",
			source:    validateVirtualListWithoutResultSource,
			wantError: "virtual model has no table to list from",
		},
		{
			name:      "virtual_model_route_list_without_result_is_rejected",
			source:    validateVirtualRouteListWithoutResultSource,
			wantError: "virtual model has no table to list from",
		},
		{
			name:   "table-backed_model_list_without_result_passes",
			source: validateBaseListWithoutResultSource,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			filename := "/repo/model/report/sample.go"
			file, err := parser.ParseFile(fset, filename, tt.source, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse source failed: %v", err)
			}

			errs := dsl.Validate(file, "/repo/model", filename)
			if tt.wantError == "" {
				if len(errs) != 0 {
					t.Fatalf("Validate returned errors: %v", errs)
				}
				return
			}
			if len(errs) == 0 {
				t.Fatalf("Validate returned no error, want %q", tt.wantError)
			}
			for _, err := range errs {
				if strings.Contains(err.Error(), tt.wantError) {
					return
				}
			}
			t.Fatalf("Validate errors %v do not contain %q", errs, tt.wantError)
		})
	}
}

const validateVirtualListWithResultSource = `
package report

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	model.Query
	model.Empty
}

type SampleListRsp struct{}

func (Sample) Design() {
	List(func() {
		Service()
		Result[*SampleListRsp]()
	})
	Export(func() {
		Service()
	})
}
`

const validateVirtualListWithoutResultSource = `
package report

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	model.Query
	model.Empty
}

func (Sample) Design() {
	List(func() {
		Service()
	})
}
`

const validateVirtualRouteListWithoutResultSource = `
package report

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	model.Query
	model.Empty
}

func (Sample) Design() {
	Route("report/samples", func() {
		List(func() {
			Service()
		})
	})
}
`

const validateBaseListWithoutResultSource = `
package report

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	model.Base
}

func (Sample) Design() {
	List(func() {
		Service()
	})
}
`

func TestValidateEmptyEmbedding(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError string
	}{
		{
			name:   "value_embedding_passes",
			source: validateEmptyValueEmbeddingSource,
		},
		{
			name:      "pointer_embedding_is_rejected",
			source:    validateEmptyPointerEmbeddingSource,
			wantError: "struct Sample embeds *model.Empty; embed model.Empty by value",
		},
		{
			name:      "aliased_pointer_embedding_is_rejected",
			source:    validateEmptyAliasedPointerEmbeddingSource,
			wantError: "struct Sample embeds *model.Empty; embed model.Empty by value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			filename := "/repo/model/report/sample.go"
			file, err := parser.ParseFile(fset, filename, tt.source, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse source failed: %v", err)
			}

			errs := dsl.Validate(file, "/repo/model", filename)
			if tt.wantError == "" {
				if len(errs) != 0 {
					t.Fatalf("Validate returned errors: %v", errs)
				}
				return
			}
			for _, err := range errs {
				if strings.Contains(err.Error(), tt.wantError) {
					return
				}
			}
			t.Fatalf("Validate errors %v do not contain %q", errs, tt.wantError)
		})
	}
}

const validateEmptyValueEmbeddingSource = `
package report

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	Name string

	model.Empty
}

type SampleCreateRsp struct{}

func (Sample) Design() {
	Create(func() {
		Service()
		Result[*SampleCreateRsp]()
	})
}
`

const validateEmptyPointerEmbeddingSource = `
package report

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	Name string

	*model.Empty
}

type SampleCreateRsp struct{}

func (Sample) Design() {
	Create(func() {
		Service()
		Result[*SampleCreateRsp]()
	})
}
`

const validateEmptyAliasedPointerEmbeddingSource = `
package report

import gstmodel "github.com/hydroan/gst/model"

type Sample struct {
	*gstmodel.Empty
}
`

func TestValidateSSEUsage(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		modelDir  string
		filename  string
		wantError string
	}{
		{
			name:     "valid_sse_action",
			source:   validateSSESource,
			modelDir: "/repo/model",
			filename: "/repo/model/sample/record.go",
		},
		{
			name:     "sse_and_get_share_a_route",
			source:   validateSSEWithGetSource,
			modelDir: "/repo/model",
			filename: "/repo/model/sample/record.go",
		},
		{
			name:      "sse_without_service",
			source:    validateSSEWithoutServiceSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/sample/record.go",
			wantError: "SSE action has no built-in implementation and must declare Service()",
		},
		{
			name:      "payload_on_sse_action",
			source:    validatePayloadOnSSESource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/sample/record.go",
			wantError: "SSE action delegates to the fixed service method SSE(ctx) error and cannot declare Payload",
		},
		{
			name:      "result_on_sse_action",
			source:    validateResultOnSSESource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/sample/record.go",
			wantError: "SSE action delegates to the fixed service method SSE(ctx) error and cannot declare Result",
		},
		{
			name:      "sse_and_list_share_a_route_block",
			source:    validateSSEWithListInRouteSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/sample/record.go",
			wantError: "SSE and List cannot share one route: both register the GET route path itself",
		},
		{
			name:      "sse_and_list_share_the_design_top_level",
			source:    validateSSEWithListTopLevelSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/sample/record.go",
			wantError: "SSE and List cannot share one route: both register the GET route path itself",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, tt.filename, tt.source, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse source failed: %v", err)
			}

			errs := dsl.Validate(file, tt.modelDir, tt.filename)
			if tt.wantError == "" {
				if len(errs) != 0 {
					t.Fatalf("Validate returned errors: %v", errs)
				}
				return
			}
			if len(errs) == 0 {
				t.Fatalf("Validate returned no errors, want %q", tt.wantError)
			}
			var got strings.Builder
			for _, err := range errs {
				got.WriteString(err.Error())
				got.WriteString("\n")
			}
			if !strings.Contains(got.String(), tt.wantError) {
				t.Fatalf("Validate errors = %q, want substring %q", got.String(), tt.wantError)
			}
		})
	}
}

const validateSSESource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Empty
}

func (Record) Design() {
	Route("sample/records/events", func() {
		SSE(func() {
			Service()
		})
	})
}
`

const validateSSEWithGetSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Route("sample/records", func() {
		SSE(func() {
			Service()
		})
		Get(func() {
			Service()
			Result[*RecordGetRsp]()
		})
	})
}
`

const validateSSEWithoutServiceSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

func (Record) Design() {
	SSE(func() {
		Public()
	})
}

type Record struct {
	model.Empty
}
`

const validatePayloadOnSSESource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Empty
}

func (Record) Design() {
	SSE(func() {
		Service()
		Payload[*RecordSSEReq]()
	})
}
`

const validateResultOnSSESource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Empty
}

func (Record) Design() {
	SSE(func() {
		Service()
		Result[*RecordSSERsp]()
	})
}
`

const validateSSEWithListInRouteSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Route("sample/records", func() {
		SSE(func() {
			Service()
		})
		List(func() {
			Service()
			Result[*RecordListRsp]()
		})
	})
}
`

const validateSSEWithListTopLevelSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	SSE(func() {
		Service()
	})
	List(func() {
		Service()
		Result[*RecordListRsp]()
	})
}
`
