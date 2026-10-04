package feed

import (
	"io"
	"net/http"

	"demo/model/board"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/service"
)

// Chat serves the ChatFeed rpc, a bidirectional stream: events in and out,
// in no fixed order.
type Chat struct {
	service.Base[*board.Feed, *board.FeedEvent, *board.FeedEvent]
}

// Stream echoes every event back until the client closes its side, which
// Recv reports as io.EOF; returning then ends the stream.
func (c *Chat) Stream(_ *gst.ServiceContext, stream *grpc.BidiStream[*board.FeedEvent, *board.FeedEvent]) error {
	for {
		event, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return gst.NewErrorWithCause(http.StatusBadRequest, "failed to read the event", err)
		}
		if err := stream.Send(&board.FeedEvent{Seq: event.Seq, Body: "echo: " + event.Body}); err != nil {
			return gst.NewErrorWithCause(http.StatusInternalServerError, "failed to send the event", err)
		}
	}
}
