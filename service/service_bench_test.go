package service

import (
	"testing"

	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/types/consts"
)

func BenchmarkResolveRegisteredService(b *testing.B) {
	type svc = Base[*testUser, *testUser, *testUser]
	Register[*svc](consts.PHASE_CREATE)

	key := serviceregistry.KeyFor[*testUser, *testUser, *testUser](consts.PHASE_CREATE)
	for b.Loop() {
		_ = serviceregistry.Resolve[*testUser, *testUser, *testUser](key)
	}
}
