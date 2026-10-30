package apiaction_test

import (
	"net/http"
	"testing"

	"github.com/hydroan/gst/internal/apiaction"
)

// TestPhase_Name pins that the name of every phase is the identifier its
// constant is declared with: generated code refers to a phase by
// service.<Name>, and the DSL parser and the service skeletons take the same
// name for the keyword and the method.
func TestPhase_Name(t *testing.T) {
	tests := []struct {
		phase apiaction.Phase
		want  string
	}{
		{apiaction.Create, "Create"},
		{apiaction.Delete, "Delete"},
		{apiaction.Update, "Update"},
		{apiaction.Patch, "Patch"},
		{apiaction.List, "List"},
		{apiaction.Get, "Get"},
		{apiaction.CreateMany, "CreateMany"},
		{apiaction.DeleteMany, "DeleteMany"},
		{apiaction.UpdateMany, "UpdateMany"},
		{apiaction.PatchMany, "PatchMany"},
		{apiaction.CreateBefore, "CreateBefore"},
		{apiaction.CreateAfter, "CreateAfter"},
		{apiaction.DeleteBefore, "DeleteBefore"},
		{apiaction.DeleteAfter, "DeleteAfter"},
		{apiaction.UpdateBefore, "UpdateBefore"},
		{apiaction.UpdateAfter, "UpdateAfter"},
		{apiaction.PatchBefore, "PatchBefore"},
		{apiaction.PatchAfter, "PatchAfter"},
		{apiaction.ListBefore, "ListBefore"},
		{apiaction.ListAfter, "ListAfter"},
		{apiaction.GetBefore, "GetBefore"},
		{apiaction.GetAfter, "GetAfter"},
		{apiaction.CreateManyBefore, "CreateManyBefore"},
		{apiaction.CreateManyAfter, "CreateManyAfter"},
		{apiaction.DeleteManyBefore, "DeleteManyBefore"},
		{apiaction.DeleteManyAfter, "DeleteManyAfter"},
		{apiaction.UpdateManyBefore, "UpdateManyBefore"},
		{apiaction.UpdateManyAfter, "UpdateManyAfter"},
		{apiaction.PatchManyBefore, "PatchManyBefore"},
		{apiaction.PatchManyAfter, "PatchManyAfter"},
		{apiaction.Import, "Import"},
		{apiaction.Export, "Export"},
		{apiaction.SSE, "SSE"},
		{apiaction.Stream, "Stream"},
		{apiaction.Phase("custom_thing"), "CustomThing"},
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
		phase apiaction.Phase
		want  string
	}{
		// single CRUD
		{"creator", apiaction.Create, "Creator"},
		{"updater", apiaction.Update, "Updater"},
		{"deleter", apiaction.Delete, "Deleter"},
		{"patcher", apiaction.Patch, "Patcher"},
		{"lister", apiaction.List, "Lister"},
		{"getter", apiaction.Get, "Getter"},

		// Many CRUD
		{"many_creator", apiaction.CreateMany, "ManyCreator"},
		{"many_updater", apiaction.UpdateMany, "ManyUpdater"},
		{"many_deleter", apiaction.DeleteMany, "ManyDeleter"},
		{"many_patcher", apiaction.PatchMany, "ManyPatcher"},

		// before/after - single
		{"create_before", apiaction.CreateBefore, "Creator"},
		{"create_after", apiaction.CreateAfter, "Creator"},
		{"update_before", apiaction.UpdateBefore, "Updater"},
		{"update_after", apiaction.UpdateAfter, "Updater"},
		{"delete_before", apiaction.DeleteBefore, "Deleter"},
		{"delete_after", apiaction.DeleteAfter, "Deleter"},
		{"patch_before", apiaction.PatchBefore, "Patcher"},
		{"patch_after", apiaction.PatchAfter, "Patcher"},
		{"list_before", apiaction.ListBefore, "Lister"},
		{"list_after", apiaction.ListAfter, "Lister"},
		{"get_before", apiaction.GetBefore, "Getter"},
		{"get_after", apiaction.GetAfter, "Getter"},

		// before/after - Many
		{"many_create_before", apiaction.CreateManyBefore, "ManyCreator"},
		{"many_create_after", apiaction.CreateManyAfter, "ManyCreator"},
		{"many_update_before", apiaction.UpdateManyBefore, "ManyUpdater"},
		{"many_update_after", apiaction.UpdateManyAfter, "ManyUpdater"},
		{"many_delete_before", apiaction.DeleteManyBefore, "ManyDeleter"},
		{"many_delete_after", apiaction.DeleteManyAfter, "ManyDeleter"},
		{"many_patch_before", apiaction.PatchManyBefore, "ManyPatcher"},
		{"many_patch_after", apiaction.PatchManyAfter, "ManyPatcher"},

		// Filter hooks are hosted on the Lister service struct, so they map
		// to the "Lister" role, mirroring list_before/list_after above.

		// non-CRUD operations
		{"import", apiaction.Import, "Importer"},
		{"export", apiaction.Export, "Exporter"},
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
		phase      apiaction.Phase
		wantBefore apiaction.Phase
		wantAfter  apiaction.Phase
	}{
		// Single CRUD
		{"create", apiaction.Create, apiaction.CreateBefore, apiaction.CreateAfter},
		{"update", apiaction.Update, apiaction.UpdateBefore, apiaction.UpdateAfter},
		{"delete", apiaction.Delete, apiaction.DeleteBefore, apiaction.DeleteAfter},
		{"patch", apiaction.Patch, apiaction.PatchBefore, apiaction.PatchAfter},
		{"list", apiaction.List, apiaction.ListBefore, apiaction.ListAfter},
		{"get", apiaction.Get, apiaction.GetBefore, apiaction.GetAfter},

		// Many CRUD
		{"create_many", apiaction.CreateMany, apiaction.CreateManyBefore, apiaction.CreateManyAfter},
		{"update_many", apiaction.UpdateMany, apiaction.UpdateManyBefore, apiaction.UpdateManyAfter},
		{"delete_many", apiaction.DeleteMany, apiaction.DeleteManyBefore, apiaction.DeleteManyAfter},
		{"patch_many", apiaction.PatchMany, apiaction.PatchManyBefore, apiaction.PatchManyAfter},

		// Already before/after → no change
		{"create_before", apiaction.CreateBefore, apiaction.CreateBefore, apiaction.CreateBefore},
		{"update_after", apiaction.UpdateAfter, apiaction.UpdateAfter, apiaction.UpdateAfter},
		{"many_delete_before", apiaction.DeleteManyBefore, apiaction.DeleteManyBefore, apiaction.DeleteManyBefore},
		{"many_patch_after", apiaction.PatchManyAfter, apiaction.PatchManyAfter, apiaction.PatchManyAfter},

		// Non CRUD → no change
		{"import", apiaction.Import, apiaction.Import, apiaction.Import},
		{"export", apiaction.Export, apiaction.Export, apiaction.Export},
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
		phase apiaction.Phase
		want  string
	}{
		{"create", apiaction.Create, http.MethodPost},
		{"delete", apiaction.Delete, http.MethodDelete},
		{"update", apiaction.Update, http.MethodPut},
		{"patch", apiaction.Patch, http.MethodPatch},
		{"list", apiaction.List, http.MethodGet},
		{"get", apiaction.Get, http.MethodGet},

		{"create_many", apiaction.CreateMany, http.MethodPost},
		{"delete_many", apiaction.DeleteMany, http.MethodDelete},
		{"update_many", apiaction.UpdateMany, http.MethodPut},
		{"patch_many", apiaction.PatchMany, http.MethodPatch},

		{"create_before", apiaction.CreateBefore, ""},
		{"delete_after", apiaction.DeleteAfter, ""},
		{"update_many_before", apiaction.UpdateManyBefore, ""},
		{"patch_many_after", apiaction.PatchManyAfter, ""},

		{"import", apiaction.Import, http.MethodPost},
		{"export", apiaction.Export, http.MethodGet},
		{"sse", apiaction.SSE, http.MethodGet},

		{"stream", apiaction.Stream, ""},
		{"unknown", apiaction.Phase(""), ""},
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
		phase apiaction.Phase
		want  string
	}{
		// single CRUD
		{"create", apiaction.Create, "create.go"},
		{"update", apiaction.Update, "update.go"},
		{"delete", apiaction.Delete, "delete.go"},
		{"patch", apiaction.Patch, "patch.go"},
		{"list", apiaction.List, "list.go"},
		{"get", apiaction.Get, "get.go"},

		// Many CRUD
		{"create_many", apiaction.CreateMany, "create_many.go"},
		{"update_many", apiaction.UpdateMany, "update_many.go"},
		{"delete_many", apiaction.DeleteMany, "delete_many.go"},
		{"patch_many", apiaction.PatchMany, "patch_many.go"},

		// before/after single
		{"create_before", apiaction.CreateBefore, "create.go"},
		{"create_after", apiaction.CreateAfter, "create.go"},
		{"update_before", apiaction.UpdateBefore, "update.go"},
		{"update_after", apiaction.UpdateAfter, "update.go"},
		{"delete_before", apiaction.DeleteBefore, "delete.go"},
		{"delete_after", apiaction.DeleteAfter, "delete.go"},
		{"patch_before", apiaction.PatchBefore, "patch.go"},
		{"patch_after", apiaction.PatchAfter, "patch.go"},
		{"list_before", apiaction.ListBefore, "list.go"},
		{"list_after", apiaction.ListAfter, "list.go"},
		{"get_before", apiaction.GetBefore, "get.go"},
		{"get_after", apiaction.GetAfter, "get.go"},

		// before/after Many
		{"create_many_before", apiaction.CreateManyBefore, "create_many.go"},
		{"create_many_after", apiaction.CreateManyAfter, "create_many.go"},
		{"update_many_before", apiaction.UpdateManyBefore, "update_many.go"},
		{"update_many_after", apiaction.UpdateManyAfter, "update_many.go"},
		{"delete_many_before", apiaction.DeleteManyBefore, "delete_many.go"},
		{"delete_many_after", apiaction.DeleteManyAfter, "delete_many.go"},
		{"patch_many_before", apiaction.PatchManyBefore, "patch_many.go"},
		{"patch_many_after", apiaction.PatchManyAfter, "patch_many.go"},

		// other phases
		{"import", apiaction.Import, "import.go"},
		{"export", apiaction.Export, "export.go"},
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
