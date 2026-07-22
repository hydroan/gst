package service

import (
	"testing"

	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/types/consts"
)

func BenchmarkResolveRegisteredService(b *testing.B) {
	type svc = Base[*testUser, *testUser, *testUser]
	Register[*svc](consts.PHASE_CREATE, "samples/bench")

	key := serviceregistry.Key(consts.PHASE_CREATE, "samples/bench")
	for b.Loop() {
		_ = serviceregistry.Resolve[*testUser, *testUser, *testUser](key)
	}
}
