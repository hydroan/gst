package dsl

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"sort"
	"testing"

	"github.com/hydroan/gst/types/consts"
	"github.com/kr/pretty"
)

func TestIsModelBase(t *testing.T) {
	fset := token.NewFileSet()

	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		code string
		want []bool
	}{
		{
			name: "input1",
			code: input1,
			want: []bool{true},
		},
		{
			name: "input2",
			code: input2,
			want: []bool{true},
		},
		{
			name: "input3",
			code: input3,
			want: []bool{true, true},
		},
		{
			name: "input4",
			code: input4,
			want: []bool{false},
		},
		{
			name: "input5",
			code: input5,
			want: []bool{true},
		},
		{
			name: "input6",
			code: input6,
			want: []bool{false, false},
		},
		{
			name: "input7",
			code: input7,
			want: []bool{false, false, false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := parser.ParseFile(fset, "", tt.code, parser.ParseComments)
			if err != nil {
				t.Error(err)
				return
			}
			modelBases := []bool{}
			for _, decl := range f.Decls {
				genDecl, ok := decl.(*ast.GenDecl)
				if !ok || genDecl == nil || genDecl.Tok != token.TYPE {
					continue
				}
				for _, spec := range genDecl.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok || typeSpec == nil {
						continue
					}
					structType, ok := typeSpec.Type.(*ast.StructType)
					if !ok || structType == nil || structType.Fields == nil {
						continue
					}
					var hasModelBase bool
					for _, field := range structType.Fields.List {
						if IsModelBase(f, field) {
							hasModelBase = true
							break
						}
					}
					if hasModelBase {
						modelBases = append(modelBases, true)
					} else {
						modelBases = append(modelBases, false)
					}
				}

			}
			if !reflect.DeepEqual(modelBases, tt.want) {
				t.Errorf("IsModelBase() = %v, want %v", modelBases, tt.want)
			}
		})
	}
}

func TestIsModelEmpty(t *testing.T) {
	fset := token.NewFileSet()

	tests := []struct {
		name string // description of this test case
		code string
		want []bool
	}{
		{
			name: "input1",
			code: input1,
			want: []bool{false},
		},
		{
			name: "input2",
			code: input2,
			want: []bool{false},
		},
		{
			name: "input3",
			code: input3,
			want: []bool{false, false},
		},
		{
			name: "input4",
			code: input4,
			want: []bool{false},
		},
		{
			name: "input5",
			code: input5,
			want: []bool{false},
		},
		{
			name: "input6",
			code: input6,
			want: []bool{true, false},
		},
		{
			name: "input7",
			code: input7,
			want: []bool{true, false, true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := parser.ParseFile(fset, "", tt.code, parser.ParseComments)
			if err != nil {
				t.Error(err)
				return
			}
			modelEmptys := []bool{}
			for _, decl := range f.Decls {
				genDecl, ok := decl.(*ast.GenDecl)
				if !ok || genDecl == nil || genDecl.Tok != token.TYPE {
					continue
				}
				for _, spec := range genDecl.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok || typeSpec == nil {
						continue
					}
					structType, ok := typeSpec.Type.(*ast.StructType)
					if !ok || structType == nil || structType.Fields == nil {
						continue
					}
					var hasModelEmpty bool
					for _, field := range structType.Fields.List {
						if IsModelEmpty(f, field) {
							hasModelEmpty = true
							break
						}
					}
					if hasModelEmpty {
						modelEmptys = append(modelEmptys, true)
					} else {
						modelEmptys = append(modelEmptys, false)
					}
				}

			}
			if !reflect.DeepEqual(modelEmptys, tt.want) {
				t.Errorf("IsModelBase() = %v, want %v", modelEmptys, tt.want)
			}
		})
	}
}

func Test_parse(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		code  string
		want1 map[string]struct{}
		want2 map[string]struct{}
	}{
		{
			name:  "input1",
			code:  input1,
			want1: map[string]struct{}{"User": {}},
			want2: map[string]struct{}{},
		},
		{
			name:  "input2",
			code:  input2,
			want1: map[string]struct{}{"User2": {}},
			want2: map[string]struct{}{},
		},
		{
			name:  "input3",
			code:  input3,
			want1: map[string]struct{}{"User3": {}, "User4": {}},
			want2: map[string]struct{}{},
		},
		{
			name:  "input4",
			code:  input4,
			want1: map[string]struct{}{},
			want2: map[string]struct{}{},
		},
		{
			name:  "input5",
			code:  input5,
			want1: map[string]struct{}{"User5": {}},
			want2: map[string]struct{}{},
		},
		{
			name:  "input6",
			code:  input6,
			want1: map[string]struct{}{},
			want2: map[string]struct{}{"User6": {}},
		},
		{
			name:  "input7",
			code:  input7,
			want1: map[string]struct{}{},
			want2: map[string]struct{}{"User8": {}, "ReceiveRobot": {}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "", tt.code, parser.ParseComments)
			if err != nil {
				t.Error(err)
				return
			}
			res1, res2 := parse(f)
			got1 := make(map[string]struct{})
			for k := range res1 {
				got1[k] = struct{}{}
			}
			got2 := make(map[string]struct{})
			for k := range res2 {
				got2[k] = struct{}{}
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("parse() return 1 = %v, want %v", got1, tt.want1)
			}
			if !reflect.DeepEqual(got2, tt.want2) {
				t.Errorf("parse() return 2 = %v, want %v", got2, tt.want2)
			}
		})
	}
}

func TestFindAllModelBase(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		code string
		want []string
	}{
		{
			name: "input1",
			code: input1,
			want: []string{"User"},
		},
		{
			name: "input2",
			code: input2,
			want: []string{"User2"},
		},
		{
			name: "input3",
			code: input3,
			want: []string{"User3", "User4"},
		},
		{
			name: "input4",
			code: input4,
			want: []string{},
		},
		{
			name: "input5",
			code: input5,
			want: []string{"User5"},
		},
		{
			name: "input6",
			code: input6,
			want: []string{},
		},
		{
			name: "input7",
			code: input7,
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "", tt.code, parser.ParseComments)
			if err != nil {
				t.Error(err)
				return
			}
			got := FindAllModelBase(f)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FindAllModelBase() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindAllModelEmpty(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		code string
		want []string
	}{
		{
			name: "input1",
			code: input1,
			want: []string{},
		},
		{
			name: "input2",
			code: input2,
			want: []string{},
		},
		{
			name: "input3",
			code: input3,
			want: []string{},
		},
		{
			name: "input4",
			code: input4,
			want: []string{},
		},
		{
			name: "input5",
			code: input5,
			want: []string{},
		},
		{
			name: "input6",
			code: input6,
			want: []string{"User6"},
		},
		{
			name: "input7",
			code: input7,
			want: []string{"User8", "ReceiveRobot"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "", tt.code, parser.ParseComments)
			if err != nil {
				t.Error(err)
				return
			}
			got := FindAllModelEmpty(f)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FindAllModelEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDesignRangeOrderDefaultRoute(t *testing.T) {
	design := parseDesignFromSource(t, defaultRouteOrderSource, "OrderHost")

	var got []consts.Phase
	design.Range(func(route string, act *Action) {
		got = append(got, act.Phase)
	})

	want := []consts.Phase{
		consts.PHASE_LIST,
		consts.PHASE_IMPORT,
		consts.PHASE_EXPORT,
		consts.PHASE_GET,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected action order: got %v want %v", got, want)
	}
}

func TestDesignRangeOrderCustomRoute(t *testing.T) {
	design := parseDesignFromSource(t, routeOrderSource, "RouteHost")

	var got []consts.Phase
	design.Range(func(route string, act *Action) {
		if route == "cmdb/hosts" {
			got = append(got, act.Phase)
		}
	})

	want := []consts.Phase{
		consts.PHASE_LIST,
		consts.PHASE_IMPORT,
		consts.PHASE_EXPORT,
		consts.PHASE_GET,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected route action order: got %v want %v", got, want)
	}
}

const defaultRouteOrderSource = `
package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type OrderHost struct {
	model.Base
}

func (OrderHost) Design() {
	Endpoint("cmdb/hosts")
	Get(func() {
		Enabled(true)
	})
	Export(func() {
		Enabled(true)
	})
	Import(func() {
		Enabled(true)
	})
	List(func() {
		Enabled(true)
	})
}
`

const routeOrderSource = `
package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type RouteHost struct {
	model.Base
}

func (RouteHost) Design() {
	Route("/cmdb/hosts", func() {
		Get(func() {
			Enabled(true)
		})
		Export(func() {
			Enabled(true)
		})
		Import(func() {
			Enabled(true)
		})
		List(func() {
			Enabled(true)
		})
	})
}
`

func TestParseFilename(t *testing.T) {
	design := parseDesignFromSource(t, filenameSource, "Attachment")

	// Collect actions by route path
	routeActions := make(map[string]*Action)
	design.Range(func(route string, act *Action) {
		routeActions[route] = act
	})

	if len(routeActions) != 2 {
		t.Fatalf("expected 2 route actions, got %d", len(routeActions))
	}

	// Route: attachment/upload with Filename("upload")
	uploadAct, ok := routeActions["attachment/upload"]
	if !ok {
		t.Fatal("expected route 'attachment/upload' not found")
	}
	if uploadAct.Filename != "upload" {
		t.Errorf("expected Filename 'upload', got %q", uploadAct.Filename)
	}
	if uploadAct.ServiceFilename() != "upload.go" {
		t.Errorf("expected ServiceFilename 'upload.go', got %q", uploadAct.ServiceFilename())
	}
	if uploadAct.RoleName() != "Upload" {
		t.Errorf("expected RoleName 'Upload', got %q", uploadAct.RoleName())
	}

	// Route: attachment/parse with Filename("parse")
	parseAct, ok := routeActions["attachment/parse"]
	if !ok {
		t.Fatal("expected route 'attachment/parse' not found")
	}
	if parseAct.Filename != "parse" {
		t.Errorf("expected Filename 'parse', got %q", parseAct.Filename)
	}
	if parseAct.ServiceFilename() != "parse.go" {
		t.Errorf("expected ServiceFilename 'parse.go', got %q", parseAct.ServiceFilename())
	}
	if parseAct.RoleName() != "Parse" {
		t.Errorf("expected RoleName 'Parse', got %q", parseAct.RoleName())
	}
}

func TestParseFilenameDefault(t *testing.T) {
	design := parseDesignFromSource(t, filenameDefaultSource, "SimpleModel")

	// Test that action without Filename uses Phase-based filename
	if design.Create.Filename != "" {
		t.Errorf("expected empty Filename, got %q", design.Create.Filename)
	}
	if design.Create.ServiceFilename() != "create.go" {
		t.Errorf("expected ServiceFilename 'create.go', got %q", design.Create.ServiceFilename())
	}
}

func TestParseFlatten(t *testing.T) {
	design := parseDesignFromSource(t, flattenSource, "Role")

	var got *Action
	design.Range(func(route string, act *Action) {
		if route == "authz/roles" && act.Phase == consts.PHASE_CREATE {
			got = act
		}
	})
	if got == nil {
		t.Fatal("expected create action for authz/roles")
	}
	if !got.Flatten {
		t.Fatal("expected Flatten to be parsed on the action")
	}
	if got.Filename != "role.go" {
		t.Fatalf("Filename = %q, want role.go", got.Filename)
	}
}

func TestParseExact(t *testing.T) {
	design := parseDesignFromSource(t, exactSource, "AdminUserSession")

	var got *Action
	design.Range(func(route string, act *Action) {
		if route == "iam/admin/users/:id/sessions" && act.Phase == consts.PHASE_DELETE {
			got = act
		}
	})
	if got == nil {
		t.Fatal("expected delete action for iam/admin/users/:id/sessions")
	}

	if !got.Exact {
		t.Fatal("expected Exact to be parsed on the action")
	}
}

func TestRoleName(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		phase    consts.Phase
		want     string
	}{
		{name: "default create", filename: "", phase: consts.PHASE_CREATE, want: "Creator"},
		{name: "default delete", filename: "", phase: consts.PHASE_DELETE, want: "Deleter"},
		{name: "default list", filename: "", phase: consts.PHASE_LIST, want: "Lister"},
		{name: "custom upload", filename: "upload", phase: consts.PHASE_CREATE, want: "Upload"},
		{name: "custom parse", filename: "parse", phase: consts.PHASE_CREATE, want: "Parse"},
		{name: "with directory and ext", filename: "a/b/user_upload.rs", phase: consts.PHASE_CREATE, want: "UserUpload"},
		{name: "uppercase", filename: "Upload", phase: consts.PHASE_CREATE, want: "Upload"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			act := &Action{Filename: tt.filename, Phase: tt.phase}
			got := act.RoleName()
			if got != tt.want {
				t.Errorf("RoleName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestServiceFilenameEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		phase    consts.Phase
		want     string
	}{
		{name: "simple name", filename: "upload", phase: consts.PHASE_CREATE, want: "upload.go"},
		{name: "with directory prefix", filename: "a/b/c", phase: consts.PHASE_CREATE, want: "c.go"},
		{name: "with extension", filename: "upload.rs", phase: consts.PHASE_CREATE, want: "upload.go"},
		{name: "with directory and extension", filename: "a/b/c.rs", phase: consts.PHASE_CREATE, want: "c.go"},
		{name: "uppercase", filename: "Upload", phase: consts.PHASE_CREATE, want: "upload.go"},
		{name: "empty falls back to phase", filename: "", phase: consts.PHASE_CREATE, want: "create.go"},
		{name: "with .go extension", filename: "upload.go", phase: consts.PHASE_CREATE, want: "upload.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			act := &Action{Filename: tt.filename, Phase: tt.phase}
			got := act.ServiceFilename()
			if got != tt.want {
				t.Errorf("ServiceFilename() = %q, want %q", got, tt.want)
			}
		})
	}
}

const filenameSource = `
package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Attachment struct {
	model.Base
}

func (Attachment) Design() {
	Migrate(true)
	Route("/attachment/upload", func() {
		Create(func() {
			Enabled(true)
			Service()
			Filename("upload")
		})
	})
	Route("/attachment/parse", func() {
		Create(func() {
			Enabled(true)
			Service()
			Filename("parse")
		})
	})
}
`

const filenameDefaultSource = `
package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type SimpleModel struct {
	model.Base
}

func (SimpleModel) Design() {
	Create(func() {
		Enabled(true)
		Service()
	})
}
`

const flattenSource = `
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

const exactSource = `
package session

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type AdminUserSession struct {
	model.Empty
}

func (AdminUserSession) Design() {
	Route("/iam/admin/users/:id/sessions", func() {
		Delete(func() {
			Exact()
			Service()
			Payload[*AdminUserSessionDeleteReq]()
			Result[*AdminUserSessionDeleteRsp]()
		})
	})
}
`

func parseDesignFromSource(t *testing.T, src, modelName string) *Design {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse source failed: %v", err)
	}

	designs := Parse(file, "")
	design, ok := designs[modelName]
	if !ok {
		t.Fatalf("model %s not found", modelName)
	}
	return design
}

func TestParseDeclaredActionDefaultEnabled(t *testing.T) {
	design := parseDesignFromSource(t, declaredActionDefaultEnabledSource, "DeclaredDefault")

	if !design.List.Enabled {
		t.Fatal("declared default-route action should be enabled by default")
	}
	if design.Get.Enabled {
		t.Fatal("undeclared action should remain disabled")
	}
	if design.Update.Enabled {
		t.Fatal("declared action with Enabled(false) should be disabled")
	}

	actions := design.routes["custom/defaults"]
	if len(actions) != 2 {
		t.Fatalf("custom route actions = %d, want 2", len(actions))
	}
	if !actions[0].Enabled {
		t.Fatal("declared custom-route action should be enabled by default")
	}
	if actions[1].Enabled {
		t.Fatal("declared custom-route action with Enabled(false) should be disabled")
	}
}

const declaredActionDefaultEnabledSource = `
package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type DeclaredDefault struct {
	model.Base
}

func (DeclaredDefault) Design() {
	List(func() {})
	Update(func() {
		Enabled(false)
	})
	Route("/custom/defaults", func() {
		Create(func() {})
		Delete(func() {
			Enabled(false)
		})
	})
}
`

func TestParseListGetPayloadDefaults(t *testing.T) {
	design := parseDesignFromSource(t, listGetPayloadDefaultsSource, "Session")

	// List with Result declared delegates to a custom service method,
	// so the request type defaults to *model.Empty.
	if design.List.Payload != PayloadEmpty {
		t.Fatalf("List.Payload = %q, want %q", design.List.Payload, PayloadEmpty)
	}
	if design.List.Result != "*SessionListRsp" {
		t.Fatalf("List.Result = %q, want *SessionListRsp", design.List.Result)
	}

	// Get without Result keeps the built-in controller defaults.
	if design.Get.Payload != "*Session" {
		t.Fatalf("Get.Payload = %q, want *Session", design.Get.Payload)
	}

	// Create with Result only keeps the model type as the request default.
	if design.Create.Payload != "*Session" {
		t.Fatalf("Create.Payload = %q, want *Session", design.Create.Payload)
	}

	// Get with Result declared inside a Route block also defaults to *model.Empty.
	actions := design.routes["iam/sessions/current"]
	if len(actions) != 1 {
		t.Fatalf("custom route actions = %d, want 1", len(actions))
	}
	if actions[0].Payload != PayloadEmpty {
		t.Fatalf("route Get.Payload = %q, want %q", actions[0].Payload, PayloadEmpty)
	}
}

func TestParseListPayloadDeclarationDiscarded(t *testing.T) {
	design := parseDesignFromSource(t, listPayloadDeclaredSource, "Session")

	// A Payload declaration on List is invalid (rejected by Validate); the
	// parser discards it so downstream code never sees a body-bound request
	// type on a GET action.
	if design.List.Payload != PayloadEmpty {
		t.Fatalf("List.Payload = %q, want %q", design.List.Payload, PayloadEmpty)
	}
}

const listGetPayloadDefaultsSource = `
package model

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
	Get(func() {})
	Create(func() {
		Service()
		Result[*SessionCreateRsp]()
	})
	Route("iam/sessions/current", func() {
		Get(func() {
			Service()
			Result[*CurrentGetRsp]()
		})
	})
}
`

const listPayloadDeclaredSource = `
package model

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

func TestParse(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		code     string
		endpoint string
		want     map[string]*Design
	}{
		{
			name:     "input1",
			code:     input1,
			endpoint: "",
			want: map[string]*Design{
				"User": {
					Enabled:  true,
					Endpoint: "iam-user2",
					Param:    ":user",
					Migrate:  true,
					routes: map[string][]*Action{
						"iam/users": {
							// The Payload[*UserReq] declaration in testdata/user.go is
							// discarded: List handles an HTTP GET request, so declaring
							// Result fixes the request type to PayloadEmpty.
							{Enabled: true, Service: true, Payload: PayloadEmpty, Result: "*UserRsp", Phase: consts.PHASE_LIST},
							{Enabled: true, Service: true, Payload: "*User", Result: "*User", Phase: consts.PHASE_GET},
						},
						"tenant/users": {
							{Enabled: true, Service: false, Payload: "*UserReq", Result: "*User", Phase: consts.PHASE_CREATE},
							{Enabled: true, Service: false, Payload: "*User", Result: "*User", Phase: consts.PHASE_UPDATE},
							{Enabled: true, Service: false, Payload: "*User", Result: "*User", Phase: consts.PHASE_PATCH},
							{Enabled: true, Service: false, Payload: "*User", Result: "*User", Phase: consts.PHASE_CREATE_MANY},
						},
					},
					Create:     &Action{Enabled: true, Service: true, Public: true, Payload: "User", Result: "*User", Phase: consts.PHASE_CREATE},
					Delete:     &Action{Enabled: true, Service: false, Public: false, Payload: "*User", Result: "*User", Phase: consts.PHASE_DELETE},
					Update:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User", Result: "User", Phase: consts.PHASE_UPDATE},
					Patch:      &Action{Enabled: false, Service: false, Public: false, Payload: "*User", Result: "*User"},
					List:       &Action{Enabled: true, Service: false, Public: false, Payload: "*User", Result: "*User", Phase: consts.PHASE_LIST},
					Get:        &Action{Enabled: false, Service: false, Public: false, Payload: "*User", Result: "*User"},
					CreateMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*User", Result: "*User"},
					DeleteMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*User", Result: "*User"},
					UpdateMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*User", Result: "*User"},
					PatchMany:  &Action{Enabled: false, Service: false, Public: false, Payload: "*User", Result: "*User"},
					Import:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User", Result: "*User"},
					Export:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User", Result: "*User"},
				},
			},
		},
		{
			name:     "input2",
			code:     input2,
			endpoint: "",
			want: map[string]*Design{
				"User2": {
					Enabled:    false,
					Endpoint:   "user2s",
					Param:      ":user",
					Migrate:    false,
					Create:     &Action{Enabled: true, Service: false, Public: false, Payload: "User2", Result: "*User3", Phase: consts.PHASE_CREATE},
					Delete:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User2", Result: "*User2"},
					Update:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User2", Result: "*User2"},
					Patch:      &Action{Enabled: true, Service: false, Public: false, Payload: "*User", Result: "User", Phase: consts.PHASE_PATCH},
					List:       &Action{Enabled: false, Service: false, Public: false, Payload: "*User2", Result: "*User2"},
					Get:        &Action{Enabled: false, Service: false, Public: false, Payload: "*User2", Result: "*User2"},
					CreateMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*User2", Result: "*User2"},
					DeleteMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*User2", Result: "*User2"},
					UpdateMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*User2", Result: "*User2"},
					PatchMany:  &Action{Enabled: false, Service: false, Public: false, Payload: "*User2", Result: "*User2"},
					Import:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User2", Result: "*User2"},
					Export:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User2", Result: "*User2"},
				},
			},
		},
		{
			name:     "input3",
			code:     input3,
			endpoint: "",
			want: map[string]*Design{
				"User3": {
					Enabled:    true,
					Endpoint:   "user",
					Migrate:    false,
					Create:     &Action{Enabled: false, Service: false, Public: false, Payload: "User", Result: "*User", Phase: consts.PHASE_CREATE},
					Delete:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User3", Result: "*User3"},
					Update:     &Action{Enabled: true, Service: false, Public: false, Payload: "*User", Result: "User", Phase: consts.PHASE_UPDATE},
					Patch:      &Action{Enabled: false, Service: false, Public: false, Payload: "*User3", Result: "*User3"},
					List:       &Action{Enabled: false, Service: false, Public: false, Payload: "*User3", Result: "*User3"},
					Get:        &Action{Enabled: false, Service: false, Public: false, Payload: "*User3", Result: "*User3"},
					CreateMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*User3", Result: "*User3"},
					DeleteMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*User3", Result: "*User3"},
					UpdateMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*User3", Result: "*User3"},
					PatchMany:  &Action{Enabled: false, Service: false, Public: false, Payload: "*User3", Result: "*User3"},
					Import:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User3", Result: "*User3"},
					Export:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User3", Result: "*User3"},
				},
				"User4": {
					Enabled:    true,
					Endpoint:   "user4s",
					Migrate:    false,
					Create:     &Action{Enabled: true, Service: false, Public: false, Payload: "User", Result: "*User", Phase: consts.PHASE_CREATE},
					Delete:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User4", Result: "*User4"},
					Update:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User", Result: "User", Phase: consts.PHASE_UPDATE},
					Patch:      &Action{Enabled: false, Service: false, Public: false, Payload: "*User4", Result: "*User4"},
					List:       &Action{Enabled: false, Service: false, Public: false, Payload: "*User4", Result: "*User4"},
					Get:        &Action{Enabled: false, Service: false, Public: false, Payload: "*User4", Result: "*User4"},
					CreateMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*User4", Result: "*User4"},
					DeleteMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*User4", Result: "*User4"},
					UpdateMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*User4", Result: "*User4"},
					PatchMany:  &Action{Enabled: false, Service: false, Public: false, Payload: "*User4", Result: "*User4"},
					Import:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User4", Result: "*User4"},
					Export:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User4", Result: "*User4"},
				},
			},
		},
		{
			name:     "input4",
			code:     input4,
			endpoint: "",
			want:     map[string]*Design{},
		},
		{
			name:     "input5",
			code:     input5,
			endpoint: "",
			want: map[string]*Design{
				"User5": {
					Enabled:    true,
					Endpoint:   "user5s",
					Migrate:    false,
					Create:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User5", Result: "*User5"},
					Delete:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User5", Result: "*User5"},
					Update:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User5", Result: "*User5"},
					Patch:      &Action{Enabled: false, Service: false, Public: false, Payload: "*User5", Result: "*User5"},
					List:       &Action{Enabled: false, Service: false, Public: false, Payload: "*User5", Result: "*User5"},
					Get:        &Action{Enabled: false, Service: false, Public: false, Payload: "*User5", Result: "*User5"},
					CreateMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*User5", Result: "*User5"},
					DeleteMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*User5", Result: "*User5"},
					UpdateMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*User5", Result: "*User5"},
					PatchMany:  &Action{Enabled: false, Service: false, Public: false, Payload: "*User5", Result: "*User5"},
					Import:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User5", Result: "*User5"},
					Export:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User5", Result: "*User5"},
				},
			},
		},
		{
			name:     "input6",
			code:     input6,
			endpoint: "",
			want: map[string]*Design{
				"User6": {
					Enabled:    true,
					Endpoint:   "user6s",
					Migrate:    false,
					IsEmpty:    true,
					Create:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User6", Result: "*User6"},
					Delete:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User6", Result: "*User6"},
					Update:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User6", Result: "*User6"},
					Patch:      &Action{Enabled: false, Service: false, Public: false, Payload: "*User6", Result: "*User6"},
					List:       &Action{Enabled: false, Service: false, Public: false, Payload: "*User6", Result: "*User6"},
					Get:        &Action{Enabled: false, Service: false, Public: false, Payload: "*User6", Result: "*User6"},
					CreateMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*User6", Result: "*User6"},
					DeleteMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*User6", Result: "*User6"},
					UpdateMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*User6", Result: "*User6"},
					PatchMany:  &Action{Enabled: false, Service: false, Public: false, Payload: "*User6", Result: "*User6"},
					Import:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User6", Result: "*User6"},
					Export:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User6", Result: "*User6"},
				},
			},
		},
		{
			name:     "input7",
			code:     input7,
			endpoint: "",
			want: map[string]*Design{
				"User8": {
					Enabled:    true,
					Endpoint:   "user8s",
					Migrate:    false,
					IsEmpty:    true,
					Create:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User8", Result: "*User8"},
					Delete:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User8", Result: "*User8"},
					Update:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User8", Result: "*User8"},
					Patch:      &Action{Enabled: false, Service: false, Public: false, Payload: "*User8", Result: "*User8"},
					List:       &Action{Enabled: false, Service: false, Public: false, Payload: "*User8", Result: "*User8"},
					Get:        &Action{Enabled: false, Service: false, Public: false, Payload: "*User8", Result: "*User8"},
					CreateMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*User8", Result: "*User8"},
					DeleteMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*User8", Result: "*User8"},
					UpdateMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*User8", Result: "*User8"},
					PatchMany:  &Action{Enabled: false, Service: false, Public: false, Payload: "*User8", Result: "*User8"},
					Import:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User8", Result: "*User8"},
					Export:     &Action{Enabled: false, Service: false, Public: false, Payload: "*User8", Result: "*User8"},
				},
				"ReceiveRobot": {
					Enabled:    true,
					Endpoint:   "receive_robots",
					Migrate:    false,
					IsEmpty:    true,
					Create:     &Action{Enabled: false, Service: false, Public: false, Payload: "*ReceiveRobot", Result: "*ReceiveRobot"},
					Delete:     &Action{Enabled: false, Service: false, Public: false, Payload: "*ReceiveRobot", Result: "*ReceiveRobot"},
					Update:     &Action{Enabled: false, Service: false, Public: false, Payload: "*ReceiveRobot", Result: "*ReceiveRobot"},
					Patch:      &Action{Enabled: false, Service: false, Public: false, Payload: "*ReceiveRobot", Result: "*ReceiveRobot"},
					List:       &Action{Enabled: false, Service: false, Public: false, Payload: "*ReceiveRobot", Result: "*ReceiveRobot"},
					Get:        &Action{Enabled: false, Service: false, Public: false, Payload: "*ReceiveRobot", Result: "*ReceiveRobot"},
					CreateMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*ReceiveRobot", Result: "*ReceiveRobot"},
					DeleteMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*ReceiveRobot", Result: "*ReceiveRobot"},
					UpdateMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*ReceiveRobot", Result: "*ReceiveRobot"},
					PatchMany:  &Action{Enabled: false, Service: false, Public: false, Payload: "*ReceiveRobot", Result: "*ReceiveRobot"},
					Import:     &Action{Enabled: false, Service: false, Public: false, Payload: "*ReceiveRobot", Result: "*ReceiveRobot"},
					Export:     &Action{Enabled: false, Service: false, Public: false, Payload: "*ReceiveRobot", Result: "*ReceiveRobot"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "", tt.code, parser.ParseComments)
			if err != nil {
				t.Error(err)
				return
			}
			got := Parse(f, tt.endpoint)
			if len(got) != len(tt.want) {
				t.Fatalf("Parse() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
			var keys1 []string
			var keys2 []string
			for k := range got {
				keys1 = append(keys1, k)
			}
			for k := range tt.want {
				keys2 = append(keys2, k)
			}
			sort.Strings(keys1)
			sort.Strings(keys2)
			if !reflect.DeepEqual(keys1, keys2) {
				t.Fatalf("Parse() = %v, want %v", got, tt.want)
			}
			for _, k := range keys1 {
				if !reflect.DeepEqual(got[k], tt.want[k]) {
					t.Fatalf("Parse() = \n%v\nwant \n%v\ndiff: \n%v\n",
						pretty.Sprintf("% #v", got[k]),
						pretty.Sprintf("% #v", tt.want[k]),
						pretty.Diff(got[k], tt.want[k]))
				}
			}
		})
	}
}
