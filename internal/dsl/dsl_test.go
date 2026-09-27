package dsl_test

import (
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/dsl"
)

func TestAction_RoleName(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		phase       consts.Phase
		want        string
	}{
		{name: "default_create", serviceName: "", phase: consts.Create, want: "Creator"},
		{name: "default_delete", serviceName: "", phase: consts.Delete, want: "Deleter"},
		{name: "default_list", serviceName: "", phase: consts.List, want: "Lister"},
		{name: "custom_archive", serviceName: "archive", phase: consts.Create, want: "Archive"},
		{name: "custom_restore", serviceName: "restore", phase: consts.Create, want: "Restore"},
		{name: "snake_case", serviceName: "item_archive", phase: consts.Create, want: "ItemArchive"},
		{name: "uppercase", serviceName: "Archive", phase: consts.Create, want: "Archive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			act := &dsl.Action{ServiceName: tt.serviceName, Phase: tt.phase}
			got := act.RoleName()
			if got != tt.want {
				t.Errorf("RoleName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAction_ServiceFilename(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		phase       consts.Phase
		want        string
	}{
		{name: "simple_name", serviceName: "archive", phase: consts.Create, want: "archive.go"},
		{name: "snake_case", serviceName: "item_archive", phase: consts.Create, want: "item_archive.go"},
		{name: "uppercase", serviceName: "Archive", phase: consts.Create, want: "archive.go"},
		{name: "empty_falls_back_to_phase", serviceName: "", phase: consts.Create, want: "create.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			act := &dsl.Action{ServiceName: tt.serviceName, Phase: tt.phase}
			got := act.ServiceFilename()
			if got != tt.want {
				t.Errorf("ServiceFilename() = %q, want %q", got, tt.want)
			}
		})
	}
}
