package testutil_test

import (
	"context"
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/grpcserver"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// SampleRecord is the neutral database model the assertion helpers are tested
// against. It keeps the base default of soft deletion so the soft-delete
// assertions have a kept row to find.
type SampleRecord struct {
	Name string
	Tag  string

	modelregistry.Base
}

func (r *SampleRecord) TableName() string { return "testutil_sample_records" }

// createSampleRecord seeds one sample row and returns it. Callers keep names
// unique to their test so the shared table stays free of cross-test matches,
// and the row is purged once the test ends, so a repeated run (go test -count)
// finds the table as the first run did.
func createSampleRecord(t *testing.T, name, tag string) *SampleRecord {
	t.Helper()

	record := &SampleRecord{Name: name, Tag: tag}
	record.SetID()
	require.NoError(t, database.Database[*SampleRecord](t.Context()).Create(record))
	t.Cleanup(func() {
		// The test context is done by the time cleanups run.
		ctx := context.WithoutCancel(t.Context())
		require.NoError(t, database.Database[*SampleRecord](ctx).WithPurge(true).Delete(record))
	})
	return record
}

// probeMethod is the one rpc of the probe gRPC service the test server
// serves: gst.test.Probe/Ping, public, answering an empty message.
const probeMethod = "/gst.test.Probe/Ping"

// registerProbeService registers the probe service with the gRPC server the
// way a project's generated pb/pb.gen.go registers its services, so that
// Run brings the gRPC listener up for this test binary: its one rpc runs
// the server's interceptors first, the way the code the protobuf plugin
// generates does, and answers an empty message.
func registerProbeService() {
	ping := grpc.MethodDesc{
		MethodName: "Ping",
		Handler: func(_ any, ctx context.Context, dec func(any) error, unary grpc.UnaryServerInterceptor) (any, error) {
			in := new(emptypb.Empty)
			if err := dec(in); err != nil {
				return nil, err
			}
			handler := func(context.Context, any) (any, error) { return &emptypb.Empty{}, nil }
			return unary(ctx, in, &grpc.UnaryServerInfo{FullMethod: probeMethod}, handler)
		},
	}
	grpcserver.Register(func(r grpc.ServiceRegistrar) {
		r.RegisterService(&grpc.ServiceDesc{
			ServiceName: "gst.test.Probe",
			HandlerType: (*any)(nil),
			Methods:     []grpc.MethodDesc{ping},
			Metadata:    "gst/test/probe.proto",
		}, nil)
	}, grpcserver.Method{Name: probeMethod, Public: true})
}
