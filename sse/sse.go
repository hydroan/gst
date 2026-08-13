// Package sse exposes the framework's Server-Sent Events support to
// application code.
//
// Services stream events through the ServiceContext.SSE entry point, whose
// callback receives a live *Conn. Fan-out from business code to the open
// connections goes through a Hub: mutations publish to a topic, every
// connection subscribed to that topic receives the event. This package
// carries the types application code names when using it; the implementation
// lives in the internal sse package.
//
// The publishing side, wherever the mutation happens:
//
//	hub.Publish("records:"+recordID, sse.Event{Event: "records", Data: hint})
//
// The streaming side, in a service method:
//
//	func (s *Watcher) List(ctx *types.ServiceContext, req *model.Empty) (*model.Empty, error) {
//		events, cancel := s.hub.Subscribe("records:" + ctx.Query().Get("record_id"))
//		defer cancel()
//		return nil, ctx.SSE(func(conn *sse.Conn) error {
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

	"github.com/hydroan/gst/internal/sse"
)

// DefaultHeartbeatInterval is the pause between automatic keep-alive comment
// frames sent on every streaming connection.
const DefaultHeartbeatInterval = sse.DefaultHeartbeatInterval

// DefaultSubscriberBuffer is the per-subscriber event buffer capacity of a Hub.
const DefaultSubscriberBuffer = sse.DefaultSubscriberBuffer

type (
	// Event is one Server-Sent Event frame.
	Event = sse.Event

	// Conn is one live SSE connection, valid inside the streaming callback.
	Conn = sse.Conn

	// Option configures a streaming connection.
	Option = sse.Option

	// Hub is an in-process publish/subscribe broker fanning events out to
	// streaming connections.
	Hub = sse.Hub

	// HubOption configures NewHub.
	HubOption = sse.HubOption

	// HubStats is a point-in-time snapshot of hub activity.
	HubStats = sse.HubStats
)

// WithHeartbeatInterval overrides DefaultHeartbeatInterval for one
// connection. The interval must be positive; heartbeats cannot be disabled.
func WithHeartbeatInterval(interval time.Duration) Option {
	return sse.WithHeartbeatInterval(interval)
}

// NewHub returns a ready Hub. It panics on invalid options.
func NewHub(opts ...HubOption) *Hub {
	return sse.NewHub(opts...)
}

// WithSubscriberBuffer overrides DefaultSubscriberBuffer for one Hub. The
// capacity must be positive.
func WithSubscriberBuffer(capacity int) HubOption {
	return sse.WithSubscriberBuffer(capacity)
}
