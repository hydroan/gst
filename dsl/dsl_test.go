package dsl_test

import (
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
)

func TestAction_RoleName(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		phase    consts.Phase
		want     string
	}{
		{name: "default_create", filename: "", phase: consts.PHASE_CREATE, want: "Creator"},
		{name: "default_delete", filename: "", phase: consts.PHASE_DELETE, want: "Deleter"},
		{name: "default_list", filename: "", phase: consts.PHASE_LIST, want: "Lister"},
		{name: "custom_upload", filename: "upload", phase: consts.PHASE_CREATE, want: "Upload"},
		{name: "custom_publish", filename: "publish", phase: consts.PHASE_CREATE, want: "Publish"},
		{name: "with_directory_and_ext", filename: "a/b/user_upload.rs", phase: consts.PHASE_CREATE, want: "UserUpload"},
		{name: "uppercase", filename: "Upload", phase: consts.PHASE_CREATE, want: "Upload"},
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
		{name: "simple_name", filename: "upload", phase: consts.PHASE_CREATE, want: "upload.go"},
		{name: "with_directory_prefix", filename: "a/b/c", phase: consts.PHASE_CREATE, want: "c.go"},
		{name: "with_extension", filename: "upload.rs", phase: consts.PHASE_CREATE, want: "upload.go"},
		{name: "with_directory_and_extension", filename: "a/b/c.rs", phase: consts.PHASE_CREATE, want: "c.go"},
		{name: "uppercase", filename: "Upload", phase: consts.PHASE_CREATE, want: "upload.go"},
		{name: "empty_falls_back_to_phase", filename: "", phase: consts.PHASE_CREATE, want: "create.go"},
		{name: "with_.go_extension", filename: "upload.go", phase: consts.PHASE_CREATE, want: "upload.go"},
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
