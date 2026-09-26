package grpc_test

import (
	"net/http"
	"testing"

	gstgrpc "github.com/hydroan/gst/grpc"
	"google.golang.org/grpc"
)

// sampleServiceServer stands for the server interface the protobuf plugin
// generates for a service, sampleService for the type gg gen generates to
// serve it, and registerSampleServiceServer for the plugin's registration
// function.
type sampleServiceServer interface{ mustEmbedUnimplementedSampleServiceServer() }

type sampleService struct{}

func (sampleService) mustEmbedUnimplementedSampleServiceServer() {}

func registerSampleServiceServer(grpc.ServiceRegistrar, sampleServiceServer) {}

// TestRegisterTakesTheGeneratedRegistrationAndTheServer pins the entry point
// the generated pb/pb.gen.go registers a service through: the plugin's
// registration function and the serving type go in as they are, under the
// server interface spelled as the type argument, with the description of
// the rpcs after them. The registration runs once the listener starts, so
// nothing runs here.
func TestRegisterTakesTheGeneratedRegistrationAndTheServer(t *testing.T) {
	gstgrpc.Register[sampleServiceServer](registerSampleServiceServer, sampleService{},
		gstgrpc.Method{Name: "/gst.test.SampleService/CreateSample", HTTPMethod: http.MethodPost, Route: "/api/samples"},
	)
}
