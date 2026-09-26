package service_test

import (
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/service"
)

func BenchmarkResolveRegisteredService(b *testing.B) {
	type svc = service.Base[*testUser, *testUser, *testUser]
	route := newRoute("samples/bench")
	service.Register[*svc](consts.Create, route)

	key := serviceregistry.Key(consts.Create, route)
	for b.Loop() {
		_ = serviceregistry.Resolve[*testUser, *testUser, *testUser](key)
	}
}
