// Package sse exposes the framework's Server-Sent Events support to
// application code.
//
// Services stream events through the ServiceContext.SSE entry point, whose
// callback receives a live *Conn; this package carries the types application
// code names when using it: the Event frame, the Conn handle, and the Serve
// options. The implementation lives in the internal sse package.
//
//	func (s *Watcher) List(ctx *types.ServiceContext, req *model.Empty) (*model.Empty, error) {
//		return nil, ctx.SSE(func(conn *sse.Conn) error {
//			for {
//				select {
//				case <-conn.Context().Done():
//					return nil
//				case item := <-s.updates:
//					if err := conn.Send(sse.Event{Event: "update", Data: item}); err != nil {
//						return err
//					}
//				}
//			}
//		})
//	}
package sse

import (
	"time"

	"github.com/hydroan/gst/internal/sse"
)

// DefaultHeartbeatInterval is the pause between automatic keep-alive comment
// frames sent on every streaming connection.
const DefaultHeartbeatInterval = sse.DefaultHeartbeatInterval

type (
	// Event is one Server-Sent Event frame.
	Event = sse.Event

	// Conn is one live SSE connection, valid inside the streaming callback.
	Conn = sse.Conn

	// Option configures a streaming connection.
	Option = sse.Option
)

// WithHeartbeatInterval overrides DefaultHeartbeatInterval for one
// connection. The interval must be positive; heartbeats cannot be disabled.
func WithHeartbeatInterval(interval time.Duration) Option {
	return sse.WithHeartbeatInterval(interval)
}
