package consts_test

import (
	"net/http"
	"testing"

	"github.com/hydroan/gst/consts"
)

// TestPhase_Name pins that the name of every phase is the identifier its
// constant is declared with: generated code refers to a phase by
// consts.<Name>, and the DSL parser and the service skeletons take the same
// name for the keyword and the method.
func TestPhase_Name(t *testing.T) {
	tests := []struct {
		phase consts.Phase
		want  string
	}{
		{consts.Create, "Create"},
		{consts.Delete, "Delete"},
		{consts.Update, "Update"},
		{consts.Patch, "Patch"},
		{consts.List, "List"},
		{consts.Get, "Get"},
		{consts.CreateMany, "CreateMany"},
		{consts.DeleteMany, "DeleteMany"},
		{consts.UpdateMany, "UpdateMany"},
		{consts.PatchMany, "PatchMany"},
		{consts.CreateBefore, "CreateBefore"},
		{consts.CreateAfter, "CreateAfter"},
		{consts.DeleteBefore, "DeleteBefore"},
		{consts.DeleteAfter, "DeleteAfter"},
		{consts.UpdateBefore, "UpdateBefore"},
		{consts.UpdateAfter, "UpdateAfter"},
		{consts.PatchBefore, "PatchBefore"},
		{consts.PatchAfter, "PatchAfter"},
		{consts.ListBefore, "ListBefore"},
		{consts.ListAfter, "ListAfter"},
		{consts.GetBefore, "GetBefore"},
		{consts.GetAfter, "GetAfter"},
		{consts.CreateManyBefore, "CreateManyBefore"},
		{consts.CreateManyAfter, "CreateManyAfter"},
		{consts.DeleteManyBefore, "DeleteManyBefore"},
		{consts.DeleteManyAfter, "DeleteManyAfter"},
		{consts.UpdateManyBefore, "UpdateManyBefore"},
		{consts.UpdateManyAfter, "UpdateManyAfter"},
		{consts.PatchManyBefore, "PatchManyBefore"},
		{consts.PatchManyAfter, "PatchManyAfter"},
		{consts.Import, "Import"},
		{consts.Export, "Export"},
		{consts.SSE, "SSE"},
		{consts.Stream, "Stream"},
		{consts.Phase("custom_thing"), "CustomThing"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.phase.Name(); got != tt.want {
				t.Errorf("Name() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPhase_RoleName(t *testing.T) {
	tests := []struct {
		name  string
		phase consts.Phase
		want  string
	}{
		// single CRUD
		{"creator", consts.Create, "Creator"},
		{"updater", consts.Update, "Updater"},
		{"deleter", consts.Delete, "Deleter"},
		{"patcher", consts.Patch, "Patcher"},
		{"lister", consts.List, "Lister"},
		{"getter", consts.Get, "Getter"},

		// Many CRUD
		{"many_creator", consts.CreateMany, "ManyCreator"},
		{"many_updater", consts.UpdateMany, "ManyUpdater"},
		{"many_deleter", consts.DeleteMany, "ManyDeleter"},
		{"many_patcher", consts.PatchMany, "ManyPatcher"},

		// before/after - single
		{"create_before", consts.CreateBefore, "Creator"},
		{"create_after", consts.CreateAfter, "Creator"},
		{"update_before", consts.UpdateBefore, "Updater"},
		{"update_after", consts.UpdateAfter, "Updater"},
		{"delete_before", consts.DeleteBefore, "Deleter"},
		{"delete_after", consts.DeleteAfter, "Deleter"},
		{"patch_before", consts.PatchBefore, "Patcher"},
		{"patch_after", consts.PatchAfter, "Patcher"},
		{"list_before", consts.ListBefore, "Lister"},
		{"list_after", consts.ListAfter, "Lister"},
		{"get_before", consts.GetBefore, "Getter"},
		{"get_after", consts.GetAfter, "Getter"},

		// before/after - Many
		{"many_create_before", consts.CreateManyBefore, "ManyCreator"},
		{"many_create_after", consts.CreateManyAfter, "ManyCreator"},
		{"many_update_before", consts.UpdateManyBefore, "ManyUpdater"},
		{"many_update_after", consts.UpdateManyAfter, "ManyUpdater"},
		{"many_delete_before", consts.DeleteManyBefore, "ManyDeleter"},
		{"many_delete_after", consts.DeleteManyAfter, "ManyDeleter"},
		{"many_patch_before", consts.PatchManyBefore, "ManyPatcher"},
		{"many_patch_after", consts.PatchManyAfter, "ManyPatcher"},

		// Filter hooks are hosted on the Lister service struct, so they map
		// to the "Lister" role, mirroring list_before/list_after above.

		// non-CRUD operations
		{"import", consts.Import, "Importer"},
		{"export", consts.Export, "Exporter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.phase.RoleName()
			if got != tt.want {
				t.Errorf("RoleName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPhase_BeforeAfter(t *testing.T) {
	tests := []struct {
		name       string
		phase      consts.Phase
		wantBefore consts.Phase
		wantAfter  consts.Phase
	}{
		// Single CRUD
		{"create", consts.Create, consts.CreateBefore, consts.CreateAfter},
		{"update", consts.Update, consts.UpdateBefore, consts.UpdateAfter},
		{"delete", consts.Delete, consts.DeleteBefore, consts.DeleteAfter},
		{"patch", consts.Patch, consts.PatchBefore, consts.PatchAfter},
		{"list", consts.List, consts.ListBefore, consts.ListAfter},
		{"get", consts.Get, consts.GetBefore, consts.GetAfter},

		// Many CRUD
		{"create_many", consts.CreateMany, consts.CreateManyBefore, consts.CreateManyAfter},
		{"update_many", consts.UpdateMany, consts.UpdateManyBefore, consts.UpdateManyAfter},
		{"delete_many", consts.DeleteMany, consts.DeleteManyBefore, consts.DeleteManyAfter},
		{"patch_many", consts.PatchMany, consts.PatchManyBefore, consts.PatchManyAfter},

		// Already before/after → no change
		{"create_before", consts.CreateBefore, consts.CreateBefore, consts.CreateBefore},
		{"update_after", consts.UpdateAfter, consts.UpdateAfter, consts.UpdateAfter},
		{"many_delete_before", consts.DeleteManyBefore, consts.DeleteManyBefore, consts.DeleteManyBefore},
		{"many_patch_after", consts.PatchManyAfter, consts.PatchManyAfter, consts.PatchManyAfter},

		// Non CRUD → no change
		{"import", consts.Import, consts.Import, consts.Import},
		{"export", consts.Export, consts.Export, consts.Export},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.phase.Before(); got != tt.wantBefore {
				t.Errorf("Before() = %v, want %v", got, tt.wantBefore)
			}
			if got := tt.phase.After(); got != tt.wantAfter {
				t.Errorf("After() = %v, want %v", got, tt.wantAfter)
			}
		})
	}
}

// TestPhase_HTTPMethod pins the one table of HTTP methods (see
// Phase.HTTPMethod): every action phase maps to the method its route is
// registered under, and a hook phase, Stream, served over gRPC alone, and
// any unknown phase map to no method.
func TestPhase_HTTPMethod(t *testing.T) {
	tests := []struct {
		name  string
		phase consts.Phase
		want  string
	}{
		{"create", consts.Create, http.MethodPost},
		{"delete", consts.Delete, http.MethodDelete},
		{"update", consts.Update, http.MethodPut},
		{"patch", consts.Patch, http.MethodPatch},
		{"list", consts.List, http.MethodGet},
		{"get", consts.Get, http.MethodGet},

		{"create_many", consts.CreateMany, http.MethodPost},
		{"delete_many", consts.DeleteMany, http.MethodDelete},
		{"update_many", consts.UpdateMany, http.MethodPut},
		{"patch_many", consts.PatchMany, http.MethodPatch},

		{"create_before", consts.CreateBefore, ""},
		{"delete_after", consts.DeleteAfter, ""},
		{"update_many_before", consts.UpdateManyBefore, ""},
		{"patch_many_after", consts.PatchManyAfter, ""},

		{"import", consts.Import, http.MethodPost},
		{"export", consts.Export, http.MethodGet},
		{"sse", consts.SSE, http.MethodGet},

		{"stream", consts.Stream, ""},
		{"unknown", consts.Phase(""), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.phase.HTTPMethod()
			if got != tt.want {
				t.Errorf("HTTPMethod() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPhase_Filename(t *testing.T) {
	tests := []struct {
		name  string
		phase consts.Phase
		want  string
	}{
		// single CRUD
		{"create", consts.Create, "create.go"},
		{"update", consts.Update, "update.go"},
		{"delete", consts.Delete, "delete.go"},
		{"patch", consts.Patch, "patch.go"},
		{"list", consts.List, "list.go"},
		{"get", consts.Get, "get.go"},

		// Many CRUD
		{"create_many", consts.CreateMany, "create_many.go"},
		{"update_many", consts.UpdateMany, "update_many.go"},
		{"delete_many", consts.DeleteMany, "delete_many.go"},
		{"patch_many", consts.PatchMany, "patch_many.go"},

		// before/after single
		{"create_before", consts.CreateBefore, "create.go"},
		{"create_after", consts.CreateAfter, "create.go"},
		{"update_before", consts.UpdateBefore, "update.go"},
		{"update_after", consts.UpdateAfter, "update.go"},
		{"delete_before", consts.DeleteBefore, "delete.go"},
		{"delete_after", consts.DeleteAfter, "delete.go"},
		{"patch_before", consts.PatchBefore, "patch.go"},
		{"patch_after", consts.PatchAfter, "patch.go"},
		{"list_before", consts.ListBefore, "list.go"},
		{"list_after", consts.ListAfter, "list.go"},
		{"get_before", consts.GetBefore, "get.go"},
		{"get_after", consts.GetAfter, "get.go"},

		// before/after Many
		{"create_many_before", consts.CreateManyBefore, "create_many.go"},
		{"create_many_after", consts.CreateManyAfter, "create_many.go"},
		{"update_many_before", consts.UpdateManyBefore, "update_many.go"},
		{"update_many_after", consts.UpdateManyAfter, "update_many.go"},
		{"delete_many_before", consts.DeleteManyBefore, "delete_many.go"},
		{"delete_many_after", consts.DeleteManyAfter, "delete_many.go"},
		{"patch_many_before", consts.PatchManyBefore, "patch_many.go"},
		{"patch_many_after", consts.PatchManyAfter, "patch_many.go"},

		// other phases
		{"import", consts.Import, "import.go"},
		{"export", consts.Export, "export.go"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.phase.Filename()
			if got != tt.want {
				t.Errorf("Filename() = %v, want %v", got, tt.want)
			}
		})
	}
}
