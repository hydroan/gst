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
		_ = serviceKey[*testUser, *testUser, *testUser](consts.PHASE_CREATE)
	}
}

// BenchmarkResolve measures the per-request cost when the key is built once
// up front, which is how controller factories resolve services.
func BenchmarkResolve(b *testing.B) {
	key := KeyFor[*testUser, *testUser, *testUser](consts.PHASE_CREATE)
	for b.Loop() {
		_ = Resolve[*testUser, *testUser, *testUser](key)
	}
}
