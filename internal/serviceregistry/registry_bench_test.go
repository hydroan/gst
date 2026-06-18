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
