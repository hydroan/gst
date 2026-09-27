package notice

import (
	"demo/configx"
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/sse"
)

// Streamer serves the notice stream.
type Streamer struct {
	service.Base[*model.Notice, *model.Notice, *model.Notice]
}

// SSE sends as many events as the [notice] section configures, under the
// event name it configures, then returns, which ends the stream. A feed
// that never ends blocks on its source instead and sends until
// conn.Context() is done, which is when the client has gone; heartbeats
// keep an idle connection alive on their own.
func (n *Streamer) SSE(ctx *gst.ServiceContext) error {
	cfg := config.Get[configx.Notice]()
	return ctx.SSE(func(conn *sse.Conn) error {
		for i := 1; i <= cfg.Count; i++ {
			if err := conn.Send(sse.Event{Event: cfg.Event, Data: i}); err != nil {
				return err
			}
		}
		return nil
	})
}
