package sse_test

import (
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/sse"
)

// ExampleServe demonstrates streaming a fixed number of events.
func ExampleServe() {
	var sc *gst.ServiceContext // acquired from a service method in real code

	_ = sse.Serve(sc, func(conn *sse.Conn) error {
		for i := 1; i <= 3; i++ {
			if err := conn.Send(sse.Event{Event: "message", Data: i}); err != nil {
				return err
			}
		}
		return nil
	})
}

// ExampleServe_waitForEvents demonstrates the event-driven shape:
// the callback blocks on an event source and stops when the client is gone.
func ExampleServe_waitForEvents() {
	var sc *gst.ServiceContext // acquired from a service method in real code
	events := make(chan sse.Event)

	_ = sse.Serve(sc, func(conn *sse.Conn) error {
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
