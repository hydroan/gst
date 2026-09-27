package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Notice is a stream of server-sent events: a long-lived GET response a
// browser reads through an EventSource, or a Go program through
// client.Stream. SSE always delegates to service code, which opens the
// stream and decides what to send; how many events a stream carries here is
// configuration, see configx.Notice. GET /api/notices.
type Notice struct {
	model.Empty
}

func (Notice) Design() {
	SSE(func() {
		Public()
		Service()
	})
}
