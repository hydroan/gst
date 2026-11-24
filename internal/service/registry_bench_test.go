package service_test

import (
	"testing"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/model"
	"github.com/hydroan/gst/internal/service"
)

type testUser struct {
	Name string
	model.Base
}

func BenchmarkRegistryKey(b *testing.B) {
	for b.Loop() {
		_ = service.Key(apiaction.Create, "samples")
	}
}

// BenchmarkResolve measures the per-request cost when the key is built once
// up front, which is how the controller handlers resolve services.
func BenchmarkResolve(b *testing.B) {
	key := service.Key(apiaction.Create, "samples")
	for b.Loop() {
		_ = service.Resolve[*testUser, *testUser, *testUser](key)
	}
}
