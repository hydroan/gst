package types_test

import (
	"github.com/hydroan/gst/sse"
	"github.com/hydroan/gst/types"
)

// ExampleServiceContext_SSE demonstrates streaming a fixed number of events.
func ExampleServiceContext_SSE() {
	var sc *types.ServiceContext // acquired from a service method in real code

	_ = sc.SSE(func(conn *sse.Conn) error {
		for i := 1; i <= 3; i++ {
			if err := conn.Send(sse.Event{Event: "message", Data: i}); err != nil {
				return err
			}
		}
		return nil
	})
}

// ExampleServiceContext_SSE_waitForEvents demonstrates the event-driven shape:
// the callback blocks on an event source and stops when the client is gone.
func ExampleServiceContext_SSE_waitForEvents() {
	var sc *types.ServiceContext // acquired from a service method in real code
	events := make(chan sse.Event)

	_ = sc.SSE(func(conn *sse.Conn) error {
		for {
			select {
			case <-conn.Context().Done():
				return nil
			case event := <-events:
				if err := conn.Send(event); err != nil {
					return err
				}
			}
		}
	})
}
