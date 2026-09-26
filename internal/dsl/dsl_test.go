package dsl_test

import (
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/dsl"
)

func TestAction_RoleName(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		phase    consts.Phase
		want     string
	}{
		{name: "default_create", filename: "", phase: consts.Create, want: "Creator"},
		{name: "default_delete", filename: "", phase: consts.Delete, want: "Deleter"},
		{name: "default_list", filename: "", phase: consts.List, want: "Lister"},
		{name: "custom_archive", filename: "archive", phase: consts.Create, want: "Archive"},
		{name: "custom_restore", filename: "restore", phase: consts.Create, want: "Restore"},
		{name: "with_directory_and_ext", filename: "a/b/item_archive.rs", phase: consts.Create, want: "ItemArchive"},
		{name: "uppercase", filename: "Archive", phase: consts.Create, want: "Archive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			act := &dsl.Action{Filename: tt.filename, Phase: tt.phase}
			got := act.RoleName()
			if got != tt.want {
				t.Errorf("RoleName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAction_ServiceFilename(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		phase    consts.Phase
		want     string
	}{
		{name: "simple_name", filename: "archive", phase: consts.Create, want: "archive.go"},
		{name: "with_directory_prefix", filename: "a/b/c", phase: consts.Create, want: "c.go"},
		{name: "with_extension", filename: "archive.rs", phase: consts.Create, want: "archive.go"},
		{name: "with_directory_and_extension", filename: "a/b/c.rs", phase: consts.Create, want: "c.go"},
		{name: "uppercase", filename: "Archive", phase: consts.Create, want: "archive.go"},
		{name: "empty_falls_back_to_phase", filename: "", phase: consts.Create, want: "create.go"},
		{name: "with_.go_extension", filename: "archive.go", phase: consts.Create, want: "archive.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			act := &dsl.Action{Filename: tt.filename, Phase: tt.phase}
			got := act.ServiceFilename()
			if got != tt.want {
				t.Errorf("ServiceFilename() = %q, want %q", got, tt.want)
			}
		})
	}
}
