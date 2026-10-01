package dsl_test

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/dsl"

	"github.com/stretchr/testify/require"
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
			name:      "flatten_without_service_name",
			source:    validateFlattenWithoutServiceNameSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/authz/role.go",
			wantError: "names no service",
		},
		{
			name:      "flatten_with_empty_service_name",
			source:    validateFlattenEmptyServiceNameSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/authz/role.go",
			wantError: "names no service",
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
			Flatten()
			Service("role")
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
		Flatten()
		Service("role")
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
			dsl.Flatten()
			dsl.Service("role")
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

const validateFlattenWithoutServiceNameSource = `
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

const validateFlattenEmptyServiceNameSource = `
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
		Flatten()
		Service("")
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
			source:    validateImportExportWithoutServiceSource,
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

const validateImportExportWithoutServiceSource = `
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
	})
	Export(func() {
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
			source:   validateActionsWithoutServiceSource,
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
			Exact()
			Service("shared")
			Result[*DetailGetRsp]()
		})
	})
	Route("sample/list", func() {
		List(func() {
			Service("shared")
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
			Service("get")
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
			Flatten()
			Service("shared")
		})
	})
	Route("sample/restore", func() {
		Update(func() {
			Flatten()
			Service("shared")
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
			Flatten()
			Service("shared")
		})
	})
	Route("sample/restore", func() {
		Update(func() {
			Service("shared")
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
			Exact()
			Service("detail")
			Result[*DetailGetRsp]()
		})
	})
	Route("sample/list", func() {
		List(func() {
			Service("list")
			Result[*RecordListRsp]()
		})
	})
}
`

const validateActionsWithoutServiceSource = `
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
		Get(func() {})
	})
	Route("sample/list", func() {
		Get(func() {})
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

func TestValidateBaseTypeEmbedding(t *testing.T) {
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
		{
			name:      "pointer_base_embedding_is_rejected",
			source:    validateBasePointerEmbeddingSource,
			wantError: "struct Sample embeds *model.Base; embed model.Base by value: the framework recognizes model.Base only when it is embedded by value",
		},
		{
			name:      "pointer_auto_base_embedding_is_rejected",
			source:    validateAutoBasePointerEmbeddingSource,
			wantError: "struct Counter embeds *model.AutoBase; embed model.AutoBase by value: the framework recognizes model.AutoBase only when it is embedded by value",
		},
		{
			name:      "pointer_base_embedding_in_a_struct_beside_the_model_is_rejected",
			source:    validateHelperPointerBaseEmbeddingSource,
			wantError: "struct sampleView embeds *model.Base; embed model.Base by value: the framework recognizes model.Base only when it is embedded by value",
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

const validateBasePointerEmbeddingSource = `
package report

import "github.com/hydroan/gst/model"

type Sample struct {
	Name string

	*model.Base
}

func (Sample) TableName() string { return "samples" }
`

const validateAutoBasePointerEmbeddingSource = `
package report

import "github.com/hydroan/gst/model"

type Counter struct {
	Name string

	*model.AutoBase
}

func (Counter) TableName() string { return "counters" }
`

// validateHelperPointerBaseEmbeddingSource declares, beside a model embedding
// model.Base by value, a struct that is no model and embeds *model.Base: every
// struct of a model file is held to the value form, not the models alone.
const validateHelperPointerBaseEmbeddingSource = `
package report

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	Name string

	model.Base
}

func (Sample) Design() {
	Migrate()
}

type sampleView struct {
	Label string

	*model.Base
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

// TestValidateRejectsARouteParameterInBraces pins that a Route writing a
// parameter as {name}, which the router would serve as a literal segment,
// is refused and told the :name form.
func TestValidateRejectsARouteParameterInBraces(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "/repo/model/sample/record.go", validateRouteBraceParameterSource, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse source failed: %v", err)
	}

	errs := dsl.Validate(file, "/repo/model", "/repo/model/sample/record.go")

	want := `/repo/model/sample/record.go: the Route("archive/boxes/{box}/documents") of Record writes the parameter box as {box}; write :box, the form the router reads`
	if len(errs) != 1 || errs[0].Error() != want {
		t.Fatalf("Validate errors = %v, want exactly %q", errs, want)
	}
}

const validateRouteBraceParameterSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Route("archive/boxes/{box}/documents", func() {
		List(func() {})
	})
}
`

// TestValidateGRPCUsage pins where GRPC() may be declared, at Design() top
// level only, and that a model declaring it has an action gRPC can serve:
// Import, Export and SSE are HTTP only.
func TestValidateGRPCUsage(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError string
	}{
		{
			name:   "grpc_with_a_served_action",
			source: validateGRPCSource,
		},
		{
			name:      "grpc_inside_an_action_block",
			source:    validateGRPCInActionSource,
			wantError: "GRPC() can only be used at Design() top level",
		},
		{
			name:      "grpc_inside_a_route_block",
			source:    validateGRPCInRouteSource,
			wantError: "GRPC() can only be used at Design() top level",
		},
		{
			name:      "grpc_with_http_only_actions",
			source:    validateGRPCWithSSEOnlySource,
			wantError: "Record declares GRPC() but no action gRPC can serve: Import, Export and SSE are HTTP only; declare another action or remove GRPC()",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "/repo/model/sample/record.go", tt.source, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse source failed: %v", err)
			}

			errs := dsl.Validate(file, "/repo/model", "/repo/model/sample/record.go")
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
				t.Fatalf("Validate errors = %q, want one containing %q", got.String(), tt.wantError)
			}
		})
	}
}

const validateGRPCSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	GRPC()
	Migrate()
	Create(func() {})
	SSE(func() {
		Service()
	})
}
`

const validateGRPCInActionSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Create(func() {
		GRPC()
	})
}
`

const validateGRPCInRouteSource = `
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
		GRPC()
		List(func() {})
	})
}
`

const validateGRPCWithSSEOnlySource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Empty
}

func (Record) Design() {
	GRPC()
	Route("sample/records/events", func() {
		SSE(func() {
			Service()
		})
	})
}
`

// TestHTTPOnlyActionNamesTheActionsGRPCCannotServe pins the examples of the
// HTTPOnlyAction doc comment.
// TestValidateStreamUsage pins the rules of a Stream action: it streams one
// side of the call or both, each side declared either unary or streaming,
// it is named by Service("name"), shaped by no Exact, and
// only a model declaring GRPC() may declare one; no other action streams.
func TestValidateStreamUsage(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError string
	}{
		{
			name:   "three_kinds_of_stream",
			source: validateStreamSource,
		},
		{
			name:   "grpc_with_a_stream_alone",
			source: validateStreamOnlySource,
		},
		{
			name:      "stream_streaming_neither_side",
			source:    validateStreamWithoutStreamingSideSource,
			wantError: "Stream action must declare StreamingPayload or StreamingResult; a call streaming neither side is a plain action, declare it with Create",
		},
		{
			name:      "stream_with_payload_and_streaming_payload",
			source:    validateStreamBothPayloadFormsSource,
			wantError: "Stream action declares both Payload and StreamingPayload; the request is either one message or a stream of them",
		},
		{
			name:      "stream_with_result_and_streaming_result",
			source:    validateStreamBothResultFormsSource,
			wantError: "Stream action declares both Result and StreamingResult; the response is either one message or a stream of them",
		},
		{
			name:      "stream_without_service_name",
			source:    validateStreamWithoutServiceNameSource,
			wantError: `Stream action must name its service, Service("name"), which names its rpc`,
		},
		{
			name:      "stream_with_empty_service_name",
			source:    validateStreamEmptyServiceNameSource,
			wantError: `Stream action must name its service, Service("name"), which names its rpc`,
		},
		{
			name:      "stream_without_service",
			source:    validateStreamWithoutServiceSource,
			wantError: "Stream action has no built-in implementation and must declare Service()",
		},
		{
			name:      "stream_without_grpc",
			source:    validateStreamWithoutGRPCSource,
			wantError: "Record declares a Stream action but no GRPC(); a stream is served over gRPC alone, declare GRPC() or remove the Stream action",
		},
		{
			name:      "streaming_result_on_create",
			source:    validateStreamingResultOnCreateSource,
			wantError: "Create action cannot declare StreamingResult; only a Stream action streams",
		},
		{
			name:      "streaming_payload_at_the_design_top_level",
			source:    validateStreamingPayloadTopLevelSource,
			wantError: "StreamingPayload() can only be used inside an action block",
		},
		{
			name:      "stream_with_exact",
			source:    validateStreamWithExactSource,
			wantError: "Stream action has no HTTP route for dsl.Exact() to shape; remove Exact()",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "/repo/model/sample/record.go", tt.source, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse source failed: %v", err)
			}

			errs := dsl.Validate(file, "/repo/model", "/repo/model/sample/record.go")
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
				t.Fatalf("Validate errors = %q, want one containing %q", got.String(), tt.wantError)
			}
		})
	}
}

const validateStreamSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

type RecordWatchReq struct{}

type RecordEvent struct{}

type RecordUploadRsp struct{}

func (Record) Design() {
	GRPC()
	Migrate()
	Create(func() {})
	Stream(func() {
		Service("tail")
		StreamingResult[*RecordEvent]()
	})
	Route("records/watch", func() {
		Stream(func() {
			Service("watch")
			Payload[*RecordWatchReq]()
			StreamingResult[*RecordEvent]()
		})
	})
	Route("records/upload", func() {
		Stream(func() {
			Service("upload")
			StreamingPayload[*RecordEvent]()
			Result[*RecordUploadRsp]()
		})
	})
	Route("records/chat", func() {
		Stream(func() {
			Service("chat")
			StreamingPayload[*RecordEvent]()
			StreamingResult[*RecordEvent]()
		})
	})
}
`

const validateStreamOnlySource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

type RecordEvent struct{}

func (Record) Design() {
	GRPC()
	Stream(func() {
		Service("tail")
		StreamingResult[*RecordEvent]()
	})
}
`

const validateStreamWithoutStreamingSideSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

type RecordWatchReq struct{}

type RecordUploadRsp struct{}

func (Record) Design() {
	GRPC()
	Stream(func() {
		Service("plain")
		Payload[*RecordWatchReq]()
		Result[*RecordUploadRsp]()
	})
}
`

const validateStreamBothPayloadFormsSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

type RecordWatchReq struct{}

type RecordEvent struct{}

func (Record) Design() {
	GRPC()
	Stream(func() {
		Service("upload")
		Payload[*RecordWatchReq]()
		StreamingPayload[*RecordEvent]()
	})
}
`

const validateStreamBothResultFormsSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

type RecordEvent struct{}

type RecordUploadRsp struct{}

func (Record) Design() {
	GRPC()
	Stream(func() {
		Service("watch")
		StreamingResult[*RecordEvent]()
		Result[*RecordUploadRsp]()
	})
}
`

const validateStreamWithoutServiceNameSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

type RecordEvent struct{}

func (Record) Design() {
	GRPC()
	Stream(func() {
		Service()
		StreamingResult[*RecordEvent]()
	})
}
`

const validateStreamEmptyServiceNameSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

type RecordEvent struct{}

func (Record) Design() {
	GRPC()
	Stream(func() {
		Service("")
		StreamingResult[*RecordEvent]()
	})
}
`

const validateStreamWithoutServiceSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

type RecordEvent struct{}

func (Record) Design() {
	GRPC()
	Stream(func() {
		StreamingResult[*RecordEvent]()
	})
}
`

const validateStreamWithoutGRPCSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

type RecordEvent struct{}

func (Record) Design() {
	Migrate()
	Create(func() {})
	Route("records/watch", func() {
		Stream(func() {
			Service("watch")
			StreamingResult[*RecordEvent]()
		})
	})
}
`

const validateStreamingResultOnCreateSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

type RecordEvent struct{}

func (Record) Design() {
	GRPC()
	Create(func() {
		Service()
		StreamingResult[*RecordEvent]()
	})
}
`

const validateStreamingPayloadTopLevelSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

type RecordEvent struct{}

func (Record) Design() {
	GRPC()
	StreamingPayload[*RecordEvent]()
	Create(func() {})
}
`

const validateStreamWithExactSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

type RecordEvent struct{}

func (Record) Design() {
	GRPC()
	Route("records/watch", func() {
		Stream(func() {
			Exact()
			Service("watch")
			StreamingResult[*RecordEvent]()
		})
	})
}
`

// TestGRPCOnlyActionNamesTheActionsHTTPCannotServe pins the counterpart of
// HTTPOnlyAction: Stream is served over gRPC alone, every other action over
// HTTP.
func TestGRPCOnlyActionNamesTheActionsHTTPCannotServe(t *testing.T) {
	if !dsl.GRPCOnlyAction("Stream") {
		t.Errorf("GRPCOnlyAction(%q) = false, want true", "Stream")
	}
	for _, name := range []string{"Create", "List", "SSE", "Import"} {
		if dsl.GRPCOnlyAction(name) {
			t.Errorf("GRPCOnlyAction(%q) = true, want false", name)
		}
	}
}

func TestHTTPOnlyActionNamesTheActionsGRPCCannotServe(t *testing.T) {
	for _, name := range []string{"Import", "Export", "SSE"} {
		if !dsl.HTTPOnlyAction(name) {
			t.Errorf("HTTPOnlyAction(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"Create", "List", "Get", "DeleteMany"} {
		if dsl.HTTPOnlyAction(name) {
			t.Errorf("HTTPOnlyAction(%q) = true, want false", name)
		}
	}
}

// TestValidateServiceName pins the report on the name a Service call gives:
// an empty name is pointed at Service(), and a refused name is suggested
// another only when that one is accepted, so each report is compared whole.
func TestValidateServiceName(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError string
	}{
		{
			name:   "bare_name",
			source: validateServiceBareNameSource,
		},
		{
			name:      "empty_name",
			source:    validateServiceEmptyNameSource,
			wantError: `Create action names its service ""; write Service() to name the service after the action, or Service("name") to name it`,
		},
		{
			name:      "file_name",
			source:    validateServiceFileNameSource,
			wantError: `Create action names its service "archive.go"; a service name is letters, digits and underscores, starting with a letter, naming the service file, its type and its rpc: Service("archive")`,
		},
		{
			name:      "path",
			source:    validateServicePathNameSource,
			wantError: `Create action names its service "sample/record/archive.go"; a service name is letters, digits and underscores, starting with a letter, naming the service file, its type and its rpc: Service("archive")`,
		},
		{
			name:      "leading_digit",
			source:    validateServiceLeadingDigitSource,
			wantError: `Create action names its service "1archive"; a service name is letters, digits and underscores, starting with a letter, naming the service file, its type and its rpc`,
		},
		{
			name:      "hyphen",
			source:    validateServiceHyphenSource,
			wantError: `Create action names its service "item-archive"; a service name is letters, digits and underscores, starting with a letter, naming the service file, its type and its rpc`,
		},
		{
			name:      "file_name_with_leading_digit",
			source:    validateServiceLeadingDigitFileNameSource,
			wantError: `Create action names its service "1archive.go"; a service name is letters, digits and underscores, starting with a letter, naming the service file, its type and its rpc`,
		},
		{
			name:      "extension_alone",
			source:    validateServiceExtensionAloneSource,
			wantError: `Create action names its service ".go"; a service name is letters, digits and underscores, starting with a letter, naming the service file, its type and its rpc`,
		},
		{
			name:      "file_name_ending_in_test",
			source:    validateServiceTestSuffixFileNameSource,
			wantError: `Create action names its service "merge_test.go"; a service name is letters, digits and underscores, starting with a letter, naming the service file, its type and its rpc`,
		},
		{
			name:      "file_name_ending_in_a_platform",
			source:    validateServiceOSSuffixFileNameSource,
			wantError: `Create action names its service "sync_windows.go"; a service name is letters, digits and underscores, starting with a letter, naming the service file, its type and its rpc`,
		},
		{
			name:      "two_arguments",
			source:    validateServiceTwoArgumentsSource,
			wantError: "Create action calls Service with 2 arguments; Service takes one at most, the name of the service",
		},
		{
			name:      "constant_argument",
			source:    validateServiceConstantArgumentSource,
			wantError: `Create action names its service with something other than a string literal; write Service("name")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "/repo/model/sample/record.go", tt.source, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse source failed: %v", err)
			}

			errs := dsl.Validate(file, "/repo/model", "/repo/model/sample/record.go")
			if tt.wantError == "" {
				if len(errs) != 0 {
					t.Fatalf("Validate returned errors: %v", errs)
				}
				return
			}
			want := "/repo/model/sample/record.go: " + tt.wantError
			for _, err := range errs {
				if err.Error() == want {
					return
				}
			}
			t.Fatalf("Validate errors = %q, want one equal to %q", errs, want)
		})
	}
}

const validateServiceBareNameSource = `
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
			Service("item_archive")
		})
	})
}
`

const validateServiceEmptyNameSource = `
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
			Service("")
		})
	})
}
`

const validateServiceFileNameSource = `
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
			Service("archive.go")
		})
	})
}
`

const validateServicePathNameSource = `
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
			Service("sample/record/archive.go")
		})
	})
}
`

const validateServiceLeadingDigitSource = `
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
			Service("1archive")
		})
	})
}
`

const validateServiceHyphenSource = `
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
			Service("item-archive")
		})
	})
}
`

const validateServiceLeadingDigitFileNameSource = `
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
			Service("1archive.go")
		})
	})
}
`

const validateServiceExtensionAloneSource = `
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
			Service(".go")
		})
	})
}
`

const validateServiceTestSuffixFileNameSource = `
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
			Service("merge_test.go")
		})
	})
}
`

const validateServiceOSSuffixFileNameSource = `
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
			Service("sync_windows.go")
		})
	})
}
`

const validateServiceTwoArgumentsSource = `
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
			Service("archive", "restore")
		})
	})
}
`

const validateServiceConstantArgumentSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

const archiveService = "archive"

type Record struct {
	model.Base
}

func (Record) Design() {
	Route("sample/archive", func() {
		Create(func() {
			Service(archiveService)
		})
	})
}
`

func TestValidateActionBlockMustBeAFunctionLiteral(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError string
	}{
		{
			name:      "nil_action_block",
			source:    validateNilActionBlockSource,
			wantError: "Create takes a function literal, Create(func() {...}); a call passing anything else, nil included, declares no action: delete it or write the block",
		},
		{
			name:      "named_function_as_action_block",
			source:    validateNamedActionBlockSource,
			wantError: "Create takes a function literal",
		},
		{
			name:      "nil_action_block_in_a_route",
			source:    validateNilActionBlockInRouteSource,
			wantError: "Create takes a function literal",
		},
		{
			name:      "nil_route_block",
			source:    validateNilRouteBlockSource,
			wantError: `Route takes a function literal, Route("path", func() {...}); a call passing anything else, nil included, declares no route: delete it or write the block`,
		},
		{
			name:      "named_function_as_route_block",
			source:    validateNamedRouteBlockSource,
			wantError: "Route takes a function literal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "/repo/model/sample/record.go", tt.source, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse source failed: %v", err)
			}

			errs := dsl.Validate(file, "/repo/model", "/repo/model/sample/record.go")
			if len(errs) == 0 {
				t.Fatalf("Validate returned no errors, want %q", tt.wantError)
			}
			var got strings.Builder
			for _, err := range errs {
				got.WriteString(err.Error())
				got.WriteString("\n")
			}
			if !strings.Contains(got.String(), tt.wantError) {
				t.Fatalf("Validate errors = %q, want one containing %q", got.String(), tt.wantError)
			}
		})
	}
}

const validateNilActionBlockSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Create(nil)
}
`

const validateNamedActionBlockSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func createBlock() {
	Service()
}

func (Record) Design() {
	Create(createBlock)
}
`

const validateNilActionBlockInRouteSource = `
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
		Create(nil)
	})
}
`

const validateNilRouteBlockSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Route("sample/archive", nil)
}
`

const validateNamedRouteBlockSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func archiveRoute() {
	Create(func() {})
}

func (Record) Design() {
	Route("sample/archive", archiveRoute)
}
`

func TestValidateDesignReadsKeywordsAlone(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError string
	}{
		{
			name:      "call_of_a_builtin_at_the_design_top_level",
			source:    validateBuiltinCallInDesignSource,
			wantError: `Design() of Record reads DSL keywords alone; delete println("x")`,
		},
		{
			name:      "call_of_a_helper_at_the_design_top_level",
			source:    validateHelperCallInDesignSource,
			wantError: "Design() of Record reads DSL keywords alone; delete declareRoutes()",
		},
		{
			name:      "call_of_another_package_at_the_design_top_level",
			source:    validatePackageCallInDesignSource,
			wantError: "Design() of Record reads DSL keywords alone; delete time.Sleep(0)",
		},
		{
			name:      "assignment_at_the_design_top_level",
			source:    validateAssignmentInDesignSource,
			wantError: "Design() of Record reads DSL keywords alone; delete _ = 1",
		},
		{
			name:      "call_of_a_builtin_in_a_route",
			source:    validateBuiltinCallInRouteSource,
			wantError: `the Route("sample/archive") block of Record reads DSL keywords alone; delete println("x")`,
		},
		{
			name:      "call_of_a_builtin_in_an_action",
			source:    validateBuiltinCallInActionSource,
			wantError: `the Create block reads DSL keywords alone; delete println("x")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "/repo/model/sample/record.go", tt.source, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse source failed: %v", err)
			}

			errs := dsl.Validate(file, "/repo/model", "/repo/model/sample/record.go")
			if len(errs) == 0 {
				t.Fatalf("Validate returned no errors, want %q", tt.wantError)
			}
			var got strings.Builder
			for _, err := range errs {
				got.WriteString(err.Error())
				got.WriteString("\n")
			}
			if !strings.Contains(got.String(), tt.wantError) {
				t.Fatalf("Validate errors = %q, want one containing %q", got.String(), tt.wantError)
			}
		})
	}
}

const validateBuiltinCallInDesignSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Migrate()
	println("x")
	Create(func() {})
}
`

const validateHelperCallInDesignSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func declareRoutes() {
	Route("sample/archive", func() {
		Create(func() {})
	})
}

func (Record) Design() {
	Migrate()
	declareRoutes()
}
`

const validatePackageCallInDesignSource = `
package sample

import (
	"time"

	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Migrate()
	time.Sleep(0)
	Create(func() {})
}
`

const validateAssignmentInDesignSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Migrate()
	_ = 1
	Create(func() {})
}
`

const validateBuiltinCallInRouteSource = `
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
		println("x")
		Create(func() {})
	})
}
`

const validateBuiltinCallInActionSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Create(func() {
		println("x")
		Service()
	})
}
`

// TestValidateRejectsAnActionDeclaredTwice pins that an action keyword is
// declared once per scope: twice at Design() top level, where the parser
// would keep the last alone, or twice in one Route block, where both would
// register under one route and phase, is refused where it is written; the
// same action at top level and in a route is two scopes, and an Exact()
// action beside one without in a route is two endpoints.
func TestValidateRejectsAnActionDeclaredTwice(t *testing.T) {
	for _, tt := range []struct{ name, source, wantError string }{
		{name: "top_level_twice", source: validateTopLevelActionTwiceSource, wantError: "Record declares Stream twice at Design() top level; declare an action once"},
		{name: "route_twice", source: validateRouteActionTwiceSource, wantError: `the Route("feeds/live") block of Record declares Stream twice; declare an action once per route, an Exact() one and one without at most`},
		{name: "top_level_and_route", source: validateTopLevelAndRouteActionSource},
		{name: "route_exact_and_item", source: validateRouteExactAndItemActionSource},
	} {
		t.Run(tt.name, func(t *testing.T) { requireValidateError(t, tt.source, tt.wantError) })
	}
}

// TestValidateRejectsServiceTypeCollisions pins that two Service actions
// generating one service type are refused like two generating one file:
// Service() on Create and Service("creator") on another action both name
// the type Creator, and Service("item_archive") and Service("itemArchive")
// both name ItemArchive while writing different files.
func TestValidateRejectsServiceTypeCollisions(t *testing.T) {
	for _, tt := range []struct{ name, source, wantError string }{
		{name: "default_and_named", source: validateServiceRoleCollisionSource, wantError: `service type "Creator" is generated by multiple actions: Create on Record, Update on Record; give each Service action a distinct name, Service("name")`},
		{name: "case_and_underscore", source: validateServiceRoleCaseCollisionSource, wantError: `service type "ItemArchive" is generated by multiple actions: Create on Record (route "sample/archive"), Create on Record (route "sample/seal"); give each Service action a distinct name, Service("name")`},
		{name: "type_beside_a_file_collision", source: validateServiceRoleAndFileCollisionSource, wantError: `service type "ItemArchive" is generated by multiple actions: Create on Record (route "sample/seal"), Create on Record (route "sample/archive"); give each Service action a distinct name, Service("name")`},
	} {
		t.Run(tt.name, func(t *testing.T) { requireValidateError(t, tt.source, tt.wantError) })
	}
}

// TestValidateRejectsServiceNamesTheGoToolReads pins that a service name the
// go command would read as a suffix of the file it names is refused: _test
// makes the file a test file, _windows or _amd64 make it build for that
// platform alone, and either leaves service.gen.go referring to a type the
// build never sees.
func TestValidateRejectsServiceNamesTheGoToolReads(t *testing.T) {
	for _, tt := range []struct{ name, source, wantError string }{
		{name: "test_suffix", source: validateServiceTestSuffixSource, wantError: `Create action names its service "merge_test"; a name ending in _test names a test file, choose another name`},
		{name: "os_suffix", source: validateServiceOSSuffixSource, wantError: `Create action names its service "sync_windows"; a name ending in _windows names a file built for that platform alone, choose another name`},
		{name: "arch_suffix", source: validateServiceArchSuffixSource, wantError: `Create action names its service "sync_amd64"; a name ending in _amd64 names a file built for that platform alone, choose another name`},
	} {
		t.Run(tt.name, func(t *testing.T) { requireValidateError(t, tt.source, tt.wantError) })
	}
}

// TestValidateRejectsActionTypesOutsideTheModelPackage pins that Payload,
// Result, StreamingPayload and StreamingResult name a type of the model
// package, T or *T: the parser reads those alone, so a type of another
// package or a slice would declare nothing and leave the action on the
// model itself.
func TestValidateRejectsActionTypesOutsideTheModelPackage(t *testing.T) {
	for _, tt := range []struct{ name, source, wantError string }{
		{name: "payload_of_another_package", source: validatePayloadForeignTypeSource, wantError: "Create action declares Payload[*shared.Event]; Payload, Result, StreamingPayload and StreamingResult name a type of the model package, T or *T"},
		{name: "result_slice", source: validateResultSliceTypeSource, wantError: "Create action declares Result[[]Event]; Payload, Result, StreamingPayload and StreamingResult name a type of the model package, T or *T"},
		{name: "streaming_result_of_another_package", source: validateStreamingResultForeignTypeSource, wantError: "Stream action declares StreamingResult[*shared.Event]; Payload, Result, StreamingPayload and StreamingResult name a type of the model package, T or *T"},
		{name: "model_package_types", source: validateActionTypesOfTheModelPackageSource},
	} {
		t.Run(tt.name, func(t *testing.T) { requireValidateError(t, tt.source, tt.wantError) })
	}
}

// requireValidateError validates source as /repo/model/sample/record.go and
// requires an error containing wantError, or none when wantError is empty.
func requireValidateError(t *testing.T, source, wantError string) {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "/repo/model/sample/record.go", source, parser.ParseComments)
	require.NoError(t, err)

	errs := dsl.Validate(file, "/repo/model", "/repo/model/sample/record.go")
	if wantError == "" {
		require.Empty(t, errs)
		return
	}
	var got strings.Builder
	for _, err := range errs {
		got.WriteString(err.Error())
		got.WriteString("\n")
	}
	require.Contains(t, got.String(), wantError)
}

const validateTopLevelActionTwiceSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

type WatchRsp struct{}

func (Record) Design() {
	GRPC()
	Stream(func() {
		Service("watch")
		StreamingResult[*WatchRsp]()
	})
	Stream(func() {
		Service("chat")
		StreamingResult[*WatchRsp]()
	})
}
`

const validateRouteActionTwiceSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

type WatchRsp struct{}

func (Record) Design() {
	GRPC()
	Route("feeds/live", func() {
		Stream(func() {
			Service("watch")
			StreamingResult[*WatchRsp]()
		})
		Stream(func() {
			Service("tail")
			StreamingResult[*WatchRsp]()
		})
	})
}
`

const validateRouteExactAndItemActionSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

type ClearReq struct{}

type ClearRsp struct{}

func (Record) Design() {
	Route("sample/records", func() {
		Delete(func() {
			Service("remove")
		})
		Delete(func() {
			Exact()
			Service("clear")
			Payload[*ClearReq]()
			Result[*ClearRsp]()
		})
	})
}
`

const validateTopLevelAndRouteActionSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

type WatchRsp struct{}

func (Record) Design() {
	GRPC()
	Stream(func() {
		Service("watch")
		StreamingResult[*WatchRsp]()
	})
	Route("feeds/live", func() {
		Stream(func() {
			Service("tail")
			StreamingResult[*WatchRsp]()
		})
	})
}
`

const validateServiceRoleCollisionSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Create(func() {
		Service()
	})
	Update(func() {
		Service("creator")
	})
}
`

const validateServiceRoleCaseCollisionSource = `
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
			Service("item_archive")
		})
	})
	Route("sample/seal", func() {
		Create(func() {
			Service("itemArchive")
		})
	})
}
`

// validateServiceRoleAndFileCollisionSource declares Service("itemArchive")
// and Service("item_archive"), which name one type from two files, beside
// Service("itemarchive"), which shares its file with the first: the type
// collision is reported together with the file collision, not once that
// one is fixed.
const validateServiceRoleAndFileCollisionSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Route("sample/seal", func() {
		Create(func() {
			Service("itemArchive")
		})
	})
	Route("sample/lock", func() {
		Create(func() {
			Service("itemarchive")
		})
	})
	Route("sample/archive", func() {
		Create(func() {
			Service("item_archive")
		})
	})
}
`

const validateServiceTestSuffixSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Create(func() {
		Service("merge_test")
	})
}
`

const validateServiceOSSuffixSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Create(func() {
		Service("sync_windows")
	})
}
`

const validateServiceArchSuffixSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Create(func() {
		Service("sync_amd64")
	})
}
`

const validatePayloadForeignTypeSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"example.com/app/model/shared"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Route("sample/notify", func() {
		Create(func() {
			Service("notify")
			Payload[*shared.Event]()
		})
	})
}
`

const validateResultSliceTypeSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

type Event struct{}

func (Record) Design() {
	Route("sample/notify", func() {
		Create(func() {
			Service("notify")
			Result[[]Event]()
		})
	})
}
`

const validateStreamingResultForeignTypeSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"example.com/app/model/shared"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	GRPC()
	Stream(func() {
		Service("watch")
		StreamingResult[*shared.Event]()
	})
}
`

const validateActionTypesOfTheModelPackageSource = `
package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

type NotifyReq struct{}

type NotifyRsp struct{}

type WatchRsp struct{}

func (Record) Design() {
	GRPC()
	Route("sample/notify", func() {
		Create(func() {
			Service("notify")
			Payload[*NotifyReq]()
			Result[*NotifyRsp]()
		})
	})
	Stream(func() {
		Service("watch")
		StreamingResult[*WatchRsp]()
	})
}
`
