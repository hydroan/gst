package serviceregistry

import (
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types/consts"
)

type testUser struct {
	Name string
	modelregistry.Base
}

func BenchmarkRegistryKey(b *testing.B) {
	for b.Loop() {
		_ = Key(consts.PHASE_CREATE, "samples")
	}
}

// BenchmarkResolve measures the per-request cost when the key is built once
// up front, which is how controller factories resolve services.
func BenchmarkResolve(b *testing.B) {
	key := Key(consts.PHASE_CREATE, "samples")
	for b.Loop() {
		_ = Resolve[*testUser, *testUser, *testUser](key)
	}
}
