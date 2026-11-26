// Package sse exposes the framework's Server-Sent Events support to
// application code.
//
// Services stream events through Serve, whose callback receives a live
// *Conn. Fan-out from business code to the open connections goes through a
// Hub: mutations publish to a topic, every connection subscribed to that
// topic receives the event. This package carries the types application code
// names when using it; the implementation lives in the internal sse package.
//
// The publishing side, wherever the mutation happens:
//
//	hub.Publish("records:"+recordID, sse.Event{Event: "records", Data: hint})
//
// The streaming side, in the SSE method of the service an SSE action
// declares; the hub is a variable of the package the publishers and the
// service share, since the generated service struct holds the embedded Base
// alone:
//
//	var hub = sse.NewHub()
//
//	func (s *Streamer) SSE(ctx *gst.ServiceContext) error {
//		events, cancel := hub.Subscribe("records:" + ctx.Query().Get("record_id"))
//		defer cancel()
//		return sse.Serve(ctx, func(conn *sse.Conn) error {
//			for {
//				select {
//				case <-conn.Context().Done():
//					return nil
//				case event, ok := <-events:
//					if !ok {
//						return nil
//					}
//					if err := conn.Send(event); err != nil {
//						return err
//					}
//				}
//			}
//		})
//	}
package sse

import (
	"time"

	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/sse"
)

type (
	// Event is one Server-Sent Event frame.
	Event = sse.Event

	// Conn is one live SSE connection, valid inside the streaming callback.
	Conn = sse.Conn

	// ServeOption configures a streaming connection.
	ServeOption = sse.ServeOption

	// Hub is an in-process publish/subscribe broker fanning events out to
	// streaming connections.
	Hub = sse.Hub

	// HubOption configures NewHub.
	HubOption = sse.HubOption

	// HubStats is a point-in-time snapshot of hub activity.
	HubStats = sse.HubStats
)

// Serve turns the response of the request ctx serves into a Server-Sent
// Events stream and runs fn with the live connection; a service calls it in
// its SSE method. The framework owns the connection: it lifts the server's
// per-request deadlines, writes the headers of the stream, sends keep-alive
// comment frames until fn returns and refuses further sends afterwards. fn
// blocks until the stream is over, selecting on conn.Context().Done() to
// notice the client leaving or the server shutting down. The error is fn's
// own, or the failure that kept the stream from opening.
func Serve(ctx *gst.ServiceContext, fn func(conn *Conn) error, opts ...ServeOption) error {
	return sse.Serve(ctx, fn, opts...)
}

// WithHeartbeatInterval sets the pause between the keep-alive comment frames
// of one connection, 15 seconds unless set. The interval must be positive;
// heartbeats cannot be disabled.
func WithHeartbeatInterval(interval time.Duration) ServeOption {
	return sse.WithHeartbeatInterval(interval)
}

// NewHub returns a ready Hub. It panics on invalid options.
func NewHub(opts ...HubOption) *Hub {
	return sse.NewHub(opts...)
}

// WithSubscriberBuffer sets how many events one Hub buffers for each
// subscriber, 64 unless set. The capacity must be positive.
func WithSubscriberBuffer(capacity int) HubOption {
	return sse.WithSubscriberBuffer(capacity)
}
