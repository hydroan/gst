package service_test

import (
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/service"
)

func BenchmarkResolveRegisteredService(b *testing.B) {
	type svc = service.Base[*testUser, *testUser, *testUser]
	service.Register[*svc](consts.PHASE_CREATE, "samples/bench")

	key := serviceregistry.Key(consts.PHASE_CREATE, "samples/bench")
	for b.Loop() {
		_ = serviceregistry.Resolve[*testUser, *testUser, *testUser](key)
	}
}
