package dsl

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
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
			name:     "valid route action",
			source:   validateNestedModelSource,
			modelDir: "/repo/model",
			filename: "/repo/model/authz/role.go",
		},
		{
			name:      "flatten on root model file",
			source:    validateRootModelSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/role.go",
			wantError: "root model file",
		},
		{
			name:     "framework module package scan is not root model file",
			source:   validateFrameworkModuleModelSource,
			modelDir: "/repo/internal/model/authz",
			filename: "/repo/internal/model/authz/role.go",
		},
		{
			name:      "flatten outside action",
			source:    validateFlattenTopLevelSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/authz/role.go",
			wantError: "Flatten() can only be used inside an action block",
		},
		{
			name:      "flatten missing filename",
			source:    validateFlattenMissingFilenameSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/authz/role.go",
			wantError: "missing Filename",
		},
		{
			name:      "flatten without service",
			source:    validateFlattenWithoutServiceSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/authz/role.go",
			wantError: "does not enable Service()",
		},
		{
			name:      "service outside action",
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

			errs := Validate(file, tt.modelDir, tt.filename)
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
			name:     "exact delete with payload and result",
			source:   validateExactDeleteWithPayloadSource,
			modelDir: "/repo/model",
			filename: "/repo/model/iam/session.go",
		},
		{
			name:      "exact delete without payload or result",
			source:    validateExactDeleteWithoutPayloadSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/iam/session.go",
			wantError: "uses dsl.Exact() but relies on the built-in controller",
		},
		{
			name:      "exact get in route block without payload or result",
			source:    validateExactGetWithoutPayloadSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/iam/session.go",
			wantError: "uses dsl.Exact() but relies on the built-in controller",
		},
		{
			name:     "exact list without payload or result",
			source:   validateExactListSource,
			modelDir: "/repo/model",
			filename: "/repo/model/iam/session.go",
		},
		{
			name:     "exact get with result only",
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

			errs := Validate(file, tt.modelDir, tt.filename)
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
			name:      "payload on list action",
			source:    validatePayloadOnListSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/iam/session.go",
			wantError: "List action handles an HTTP GET request and cannot declare Payload",
		},
		{
			name:      "payload on get action in route block",
			source:    validatePayloadOnGetInRouteSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/iam/session.go",
			wantError: "Get action handles an HTTP GET request and cannot declare Payload",
		},
		{
			name:     "result only on list action",
			source:   validateResultOnlyOnListSource,
			modelDir: "/repo/model",
			filename: "/repo/model/iam/session.go",
		},
		{
			name:     "payload on create action",
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

			errs := Validate(file, tt.modelDir, tt.filename)
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
			name:      "payload on export action",
			source:    validatePayloadOnExportSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/sample/record.go",
			wantError: "Export action delegates to the fixed service method Export(ctx, ...M) ([]byte, error) and cannot declare Payload",
		},
		{
			name:      "result on export action",
			source:    validateResultOnExportSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/sample/record.go",
			wantError: "Export action delegates to the fixed service method Export(ctx, ...M) ([]byte, error) and cannot declare Result",
		},
		{
			name:      "payload on import action in route block",
			source:    validatePayloadOnImportInRouteSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/sample/record.go",
			wantError: "Import action delegates to the fixed service method Import(ctx, io.Reader) ([]M, error) and cannot declare Payload",
		},
		{
			name:      "result on import action",
			source:    validateResultOnImportSource,
			modelDir:  "/repo/model",
			filename:  "/repo/model/sample/record.go",
			wantError: "Import action delegates to the fixed service method Import(ctx, io.Reader) ([]M, error) and cannot declare Result",
		},
		{
			name:     "import and export with enabled only",
			source:   validateEnabledOnlyImportExportSource,
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

			errs := Validate(file, tt.modelDir, tt.filename)
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
