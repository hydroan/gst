package controller

import (
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/types"
)

// This file holds what the transports of a Stream action share, the gRPC
// calls of stream_grpc.go and the WebSocket handler of stream_http.go: the
// requests of a request stream, read with the refusal one of them met kept
// for the answer, and the failure of an action no Stream method serves.

// requests reads the requests of a request stream through the function the
// handler hands the call: recv normalizes and validates each the way
// ServiceCall treats a payload, and one the validator refuses, or one the
// handler's decoding refused before it got here, ends the stream, the
// refusal kept in refused for the call to answer whatever the service
// returns for the error it got. recv runs on whichever goroutine the
// service receives on, which grpc-go allows to be one beside the
// service's own, and the call reads the refusal once the service returned,
// without the service having waited for that goroutine: mu orders the two.
type requests[REQ types.Request] struct {
	recv func() (REQ, error)

	mu      sync.Mutex
	refused error
}

// refuse records err as the refusal.
func (r *requests[REQ]) refuse(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refused = err
}

// refusal returns the refusal recv recorded, nil for none.
func (r *requests[REQ]) refusal() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.refused
}

// unserved returns the failure of the Stream action on route whose service,
// the one registered for the route and the Stream phase, has no Stream method
// of the kind the action declares — the service file was not generated, or
// its method was changed. Which action, on which route, is for the log and
// the span, like the cause of any other internal failure: each transport
// answers it with unimplementedMsg and its own status for what nothing
// serves.
func (a *action[M, REQ, RSP]) unserved(route string) error {
	return errors.Newf("the Stream action of %s on %s is served by no Stream method of its kind; gg gen declares the method in the service file", a.name, route)
}
