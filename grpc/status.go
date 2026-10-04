package grpc

import "github.com/hydroan/gst/internal/grpcserver"

// StatusError returns the status error a call answers err with: a service
// error (see gst.NewError) answers with its own status and message,
// mapped to the gRPC code the way the HTTP listener maps it to a response;
// any other error answers Internal with a fixed message, its text kept out
// of the answer, so log it before mapping it. nil stays nil.
func StatusError(err error) error {
	return grpcserver.StatusError(err)
}
