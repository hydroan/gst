package notice

import (
	"demo/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/sse"
	"github.com/hydroan/gst/types"
)

type Streamer struct {
	service.Base[*model.Notice, *model.Notice, *model.Notice]
}

// SSE streams three demo events and ends the stream. A real feed would block
// on an event source instead — subscribe to an sse.Hub, forward events until
// conn.Context() is done — see the gst sse package documentation for that
// shape. Heartbeat comment frames are sent automatically while the stream is
// open, so an event-quiet connection stays alive on its own.
func (n *Streamer) SSE(ctx *types.ServiceContext) (err error) {
	log := n.WithContext(ctx, ctx.Phase())
	log.Info("notice sse")

	return ctx.SSE(func(conn *sse.Conn) error {
		for i := 1; i <= 3; i++ {
			if err := conn.Send(sse.Event{Event: "notice", Data: i}); err != nil {
				return err
			}
		}
		return nil
	})
}
