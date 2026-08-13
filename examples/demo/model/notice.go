package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Notice is a virtual resource demonstrating the SSE streaming action: the
// route serves a long-lived text/event-stream that a browser consumes through
// an EventSource, or a Go caller through client.Stream.
type Notice struct {
	model.Empty
}

func (Notice) Design() {
	// SSE declares a streaming GET route. The action always delegates to the
	// service's SSE method, which opens the stream via ctx.SSE and blocks
	// until it is over. The route is automatically marked as streaming, which
	// exempts it from request-scoped response treatment (body capture,
	// circuit breaking, request timeouts) and clears the server's per-request
	// deadlines for the connection.
	dsl.SSE(func() {
		dsl.Public()
		dsl.Service()
	})
}
