package service

import (
	"testing"

	"github.com/hydroan/gst/types/consts"
)

func BenchmarkServiceKey(b *testing.B) {
	for b.Loop() {
		_ = serviceKey[*testUser, *testUser, *testUser](consts.PHASE_CREATE)
	}
}
