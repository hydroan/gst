package service_test

import (
	"testing"

	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/service"
)

func BenchmarkResolveRegisteredService(b *testing.B) {
	type svc = service.Base[*testUser, *testUser, *testUser]
	route := newRoute("samples/bench")
	service.Register[*svc](service.Create, route)

	key := serviceregistry.Key(service.Create, route)
	for b.Loop() {
		_ = serviceregistry.Resolve[*testUser, *testUser, *testUser](key)
	}
}
