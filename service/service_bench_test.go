package service_test

import (
	"testing"

	internalservice "github.com/hydroan/gst/internal/service"
	"github.com/hydroan/gst/service"
)

func BenchmarkResolveRegisteredService(b *testing.B) {
	type svc = service.Base[*testUser, *testUser, *testUser]
	route := newRoute("samples/bench")
	service.Register[*svc](service.Create, route)

	key := internalservice.Key(service.Create, route)
	for b.Loop() {
		_ = internalservice.Resolve[*testUser, *testUser, *testUser](key)
	}
}
