package dsl

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"sort"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/kr/pretty"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		code string
		want map[string]*Design
	}{
		{
			name: "user",
			code: userSource,
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
					SSE:        &Action{Enabled: false, Service: false, Public: false, Payload: "*User", Result: "*User"},
				},
			},
		},
		{
			name: "user2",
			code: user2Source,
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
					SSE:        &Action{Enabled: false, Service: false, Public: false, Payload: "*User2", Result: "*User2"},
				},
			},
		},
		{
			name: "user3_4",
			code: user3And4Source,
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
					SSE:        &Action{Enabled: false, Service: false, Public: false, Payload: "*User3", Result: "*User3"},
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
					SSE:        &Action{Enabled: false, Service: false, Public: false, Payload: "*User4", Result: "*User4"},
				},
			},
		},
		{
			name: "user4",
			code: user4Source,
			want: map[string]*Design{},
		},
		{
			name: "user5",
			code: user5Source,
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
					SSE:        &Action{Enabled: false, Service: false, Public: false, Payload: "*User5", Result: "*User5"},
				},
			},
		},
		{
			name: "user6_7",
			code: user6And7Source,
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
					SSE:        &Action{Enabled: false, Service: false, Public: false, Payload: "*User6", Result: "*User6"},
				},
			},
		},
		{
			name: "user8_9",
			code: user8And9Source,
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
					SSE:        &Action{Enabled: false, Service: false, Public: false, Payload: "*User8", Result: "*User8"},
				},
				"SampleRecord": {
					Enabled:    true,
					Endpoint:   "sample_records",
					Migrate:    false,
					IsEmpty:    true,
					Create:     &Action{Enabled: false, Service: false, Public: false, Payload: "*SampleRecord", Result: "*SampleRecord"},
					Delete:     &Action{Enabled: false, Service: false, Public: false, Payload: "*SampleRecord", Result: "*SampleRecord"},
					Update:     &Action{Enabled: false, Service: false, Public: false, Payload: "*SampleRecord", Result: "*SampleRecord"},
					Patch:      &Action{Enabled: false, Service: false, Public: false, Payload: "*SampleRecord", Result: "*SampleRecord"},
					List:       &Action{Enabled: false, Service: false, Public: false, Payload: "*SampleRecord", Result: "*SampleRecord"},
					Get:        &Action{Enabled: false, Service: false, Public: false, Payload: "*SampleRecord", Result: "*SampleRecord"},
					CreateMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*SampleRecord", Result: "*SampleRecord"},
					DeleteMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*SampleRecord", Result: "*SampleRecord"},
					UpdateMany: &Action{Enabled: false, Service: false, Public: false, Payload: "*SampleRecord", Result: "*SampleRecord"},
					PatchMany:  &Action{Enabled: false, Service: false, Public: false, Payload: "*SampleRecord", Result: "*SampleRecord"},
					Import:     &Action{Enabled: false, Service: false, Public: false, Payload: "*SampleRecord", Result: "*SampleRecord"},
					Export:     &Action{Enabled: false, Service: false, Public: false, Payload: "*SampleRecord", Result: "*SampleRecord"},
					SSE:        &Action{Enabled: false, Service: false, Public: false, Payload: "*SampleRecord", Result: "*SampleRecord"},
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
			got := Parse(f)
			if len(got) != len(tt.want) {
				t.Fatalf("Parse() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
			var gotKeys []string
			var wantKeys []string
			for k := range got {
				gotKeys = append(gotKeys, k)
			}
			for k := range tt.want {
				wantKeys = append(wantKeys, k)
			}
			sort.Strings(gotKeys)
			sort.Strings(wantKeys)
			if !reflect.DeepEqual(gotKeys, wantKeys) {
				t.Fatalf("Parse() = %v, want %v", got, tt.want)
			}
			for _, k := range gotKeys {
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

func TestParseFilename(t *testing.T) {
	design := parseDesignFromSource(t, filenameSource, "Record")

	// Collect actions by route path
	routeActions := make(map[string]*Action)
	design.Range(func(route string, act *Action) {
		routeActions[route] = act
	})

	if len(routeActions) != 2 {
		t.Fatalf("expected 2 route actions, got %d", len(routeActions))
	}

	// Route: record/archive with Filename("archive")
	archiveAct, ok := routeActions["record/archive"]
	if !ok {
		t.Fatal("expected route 'record/archive' not found")
	}
	if archiveAct.Filename != "archive" {
		t.Errorf("expected Filename 'archive', got %q", archiveAct.Filename)
	}
	if archiveAct.ServiceFilename() != "archive.go" {
		t.Errorf("expected ServiceFilename 'archive.go', got %q", archiveAct.ServiceFilename())
	}
	if archiveAct.RoleName() != "Archive" {
		t.Errorf("expected RoleName 'Archive', got %q", archiveAct.RoleName())
	}

	// Route: record/restore with Filename("restore")
	restoreAct, ok := routeActions["record/restore"]
	if !ok {
		t.Fatal("expected route 'record/restore' not found")
	}
	if restoreAct.Filename != "restore" {
		t.Errorf("expected Filename 'restore', got %q", restoreAct.Filename)
	}
	if restoreAct.ServiceFilename() != "restore.go" {
		t.Errorf("expected ServiceFilename 'restore.go', got %q", restoreAct.ServiceFilename())
	}
	if restoreAct.RoleName() != "Restore" {
		t.Errorf("expected RoleName 'Restore', got %q", restoreAct.RoleName())
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

const filenameSource = `
package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Migrate()
	Route("/record/archive", func() {
		Create(func() {
			Enabled(true)
			Service()
			Filename("archive")
		})
	})
	Route("/record/restore", func() {
		Create(func() {
			Enabled(true)
			Service()
			Filename("restore")
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

func TestParseActionTypeDefaults(t *testing.T) {
	design := parseDesignFromSource(t, actionTypeDefaultsSource, "Session")

	// List with Result declared delegates to a custom service method,
	// so the request type defaults to *model.Empty.
	if design.List.Payload != PayloadEmpty {
		t.Fatalf("List.Payload = %q, want %q", design.List.Payload, PayloadEmpty)
	}
	if design.List.Result != "*SessionListRsp" {
		t.Fatalf("List.Result = %q, want *SessionListRsp", design.List.Result)
	}

	// Get with neither side declared keeps the built-in controller defaults:
	// both sides stay on the model type.
	if design.Get.Payload != "*Session" {
		t.Fatalf("Get.Payload = %q, want *Session", design.Get.Payload)
	}
	if design.Get.Result != "*Session" {
		t.Fatalf("Get.Result = %q, want *Session", design.Get.Result)
	}

	// Create with Result only must not guess the model as the request type:
	// the missing side defaults to *model.Empty.
	if design.Create.Payload != PayloadEmpty {
		t.Fatalf("Create.Payload = %q, want %q", design.Create.Payload, PayloadEmpty)
	}

	// Update with Payload only defaults the missing response side to
	// *model.Empty as well.
	if design.Update.Result != PayloadEmpty {
		t.Fatalf("Update.Result = %q, want %q", design.Update.Result, PayloadEmpty)
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

const actionTypeDefaultsSource = `
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
	Update(func() {
		Service()
		Payload[*SessionUpdateReq]()
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

func TestParseImportExportPayloadResultDeclarationsDiscarded(t *testing.T) {
	design := parseDesignFromSource(t, importExportPayloadResultDeclaredSource, "Record")

	// Payload and Result declarations on Import and Export are invalid
	// (rejected by Validate); the parser discards them so the generated
	// registration keeps the model type as the request and response types.
	if design.Import.Payload != "*Record" {
		t.Fatalf("Import.Payload = %q, want *Record", design.Import.Payload)
	}
	if design.Import.Result != "*Record" {
		t.Fatalf("Import.Result = %q, want *Record", design.Import.Result)
	}
	if design.Export.Payload != "*Record" {
		t.Fatalf("Export.Payload = %q, want *Record", design.Export.Payload)
	}
	if design.Export.Result != "*Record" {
		t.Fatalf("Export.Result = %q, want *Record", design.Export.Result)
	}
}

const importExportPayloadResultDeclaredSource = `
package model

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
		Payload[*RecordImportReq]()
		Result[*RecordImportRsp]()
	})
	Export(func() {
		Service()
		Payload[*RecordExportReq]()
		Result[*RecordExportRsp]()
	})
}
`

func TestParseSplitsBaseAndEmptyModels(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		wantBase  map[string]struct{}
		wantEmpty map[string]struct{}
	}{
		{
			name:      "user",
			code:      userSource,
			wantBase:  map[string]struct{}{"User": {}},
			wantEmpty: map[string]struct{}{},
		},
		{
			name:      "user2",
			code:      user2Source,
			wantBase:  map[string]struct{}{"User2": {}},
			wantEmpty: map[string]struct{}{},
		},
		{
			name:      "user3_4",
			code:      user3And4Source,
			wantBase:  map[string]struct{}{"User3": {}, "User4": {}},
			wantEmpty: map[string]struct{}{},
		},
		{
			name:      "user4",
			code:      user4Source,
			wantBase:  map[string]struct{}{},
			wantEmpty: map[string]struct{}{},
		},
		{
			name:      "user5",
			code:      user5Source,
			wantBase:  map[string]struct{}{"User5": {}},
			wantEmpty: map[string]struct{}{},
		},
		{
			name:      "user6_7",
			code:      user6And7Source,
			wantBase:  map[string]struct{}{},
			wantEmpty: map[string]struct{}{"User6": {}},
		},
		{
			name:      "user8_9",
			code:      user8And9Source,
			wantBase:  map[string]struct{}{},
			wantEmpty: map[string]struct{}{"User8": {}, "SampleRecord": {}},
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
			base, empty := parse(f)
			gotBase := make(map[string]struct{})
			for k := range base {
				gotBase[k] = struct{}{}
			}
			gotEmpty := make(map[string]struct{})
			for k := range empty {
				gotEmpty[k] = struct{}{}
			}
			if !reflect.DeepEqual(gotBase, tt.wantBase) {
				t.Errorf("parse() return 1 = %v, want %v", gotBase, tt.wantBase)
			}
			if !reflect.DeepEqual(gotEmpty, tt.wantEmpty) {
				t.Errorf("parse() return 2 = %v, want %v", gotEmpty, tt.wantEmpty)
			}
		})
	}
}

func TestFindAllModelBase(t *testing.T) {
	tests := []struct {
		name string
		code string
		want []string
	}{
		{
			name: "user",
			code: userSource,
			want: []string{"User"},
		},
		{
			name: "user2",
			code: user2Source,
			want: []string{"User2"},
		},
		{
			name: "user3_4",
			code: user3And4Source,
			want: []string{"User3", "User4"},
		},
		{
			name: "user4",
			code: user4Source,
			want: []string{},
		},
		{
			name: "user5",
			code: user5Source,
			want: []string{"User5"},
		},
		{
			name: "user6_7",
			code: user6And7Source,
			want: []string{},
		},
		{
			name: "user8_9",
			code: user8And9Source,
			want: []string{},
		},
		{
			name: "user10_11",
			code: user10And11Source,
			want: []string{"User10", "User11"},
		},
		{
			name: "user12",
			code: user12Source,
			want: []string{"User12"},
		},
		{
			name: "user13",
			code: user13Source,
			want: []string{"User13"},
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
		name string
		code string
		want []string
	}{
		{
			name: "user",
			code: userSource,
			want: []string{},
		},
		{
			name: "user2",
			code: user2Source,
			want: []string{},
		},
		{
			name: "user3_4",
			code: user3And4Source,
			want: []string{},
		},
		{
			name: "user4",
			code: user4Source,
			want: []string{},
		},
		{
			name: "user5",
			code: user5Source,
			want: []string{},
		},
		{
			name: "user6_7",
			code: user6And7Source,
			want: []string{"User6"},
		},
		{
			name: "user8_9",
			code: user8And9Source,
			want: []string{"User8", "SampleRecord"},
		},
		{
			name: "user10_11",
			code: user10And11Source,
			want: []string{},
		},
		{
			name: "user12",
			code: user12Source,
			want: []string{},
		},
		{
			name: "user13",
			code: user13Source,
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
			got := FindAllModelEmpty(f)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FindAllModelEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsModelBase(t *testing.T) {
	fset := token.NewFileSet()

	tests := []struct {
		name string
		code string
		want []bool
	}{
		{
			name: "user",
			code: userSource,
			want: []bool{true},
		},
		{
			name: "user2",
			code: user2Source,
			want: []bool{true},
		},
		{
			name: "user3_4",
			code: user3And4Source,
			want: []bool{true, true},
		},
		{
			name: "user4",
			code: user4Source,
			want: []bool{false},
		},
		{
			name: "user5",
			code: user5Source,
			want: []bool{true},
		},
		{
			name: "user6_7",
			code: user6And7Source,
			want: []bool{false, false},
		},
		{
			name: "user8_9",
			code: user8And9Source,
			want: []bool{false, false, false},
		},
		{
			name: "user10_11",
			code: user10And11Source,
			want: []bool{true, true},
		},
		{
			name: "user12",
			code: user12Source,
			want: []bool{true},
		},
		{
			name: "user13",
			code: user13Source,
			want: []bool{true},
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
		name string
		code string
		want []bool
	}{
		{
			name: "user",
			code: userSource,
			want: []bool{false},
		},
		{
			name: "user2",
			code: user2Source,
			want: []bool{false},
		},
		{
			name: "user3_4",
			code: user3And4Source,
			want: []bool{false, false},
		},
		{
			name: "user4",
			code: user4Source,
			want: []bool{false},
		},
		{
			name: "user5",
			code: user5Source,
			want: []bool{false},
		},
		{
			name: "user6_7",
			code: user6And7Source,
			want: []bool{true, false},
		},
		{
			name: "user8_9",
			code: user8And9Source,
			want: []bool{true, false, true},
		},
		{
			name: "user10_11",
			code: user10And11Source,
			want: []bool{false, false},
		},
		{
			name: "user12",
			code: user12Source,
			want: []bool{false},
		},
		{
			name: "user13",
			code: user13Source,
			want: []bool{false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := parser.ParseFile(fset, "", tt.code, parser.ParseComments)
			if err != nil {
				t.Error(err)
				return
			}
			modelEmpties := []bool{}
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
						modelEmpties = append(modelEmpties, true)
					} else {
						modelEmpties = append(modelEmpties, false)
					}
				}

			}
			if !reflect.DeepEqual(modelEmpties, tt.want) {
				t.Errorf("IsModelEmpty() = %v, want %v", modelEmpties, tt.want)
			}
		})
	}
}
