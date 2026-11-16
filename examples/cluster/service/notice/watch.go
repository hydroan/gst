package notice

import (
	"encoding/json"
	"net/http"

	"cluster/helper"
	"cluster/model"

	"github.com/hydroan/gst"
	gstmodel "github.com/hydroan/gst/model"
	gstredis "github.com/hydroan/gst/redis"
	"github.com/hydroan/gst/service"
)

// Watch serves the notice stream, a server stream over a WebSocket
// connection: every notice published while the connection is open, whichever
// replica it was published on.
type Watch struct {
	service.Base[*model.Notice, *gstmodel.Empty, *model.NoticeWatchRsp]
}

// Stream subscribes the connection to the notice channel and passes on what
// arrives until ctx is done, which is when the client has gone or the
// process is stopping. The subscription is the connection's own, one redis
// connection each, and closes with it. Each notice sent names the replica
// that published it and this one, the replica the connection is on.
func (w *Watch) Stream(ctx *gst.ServiceContext, _ *gstmodel.Empty, stream *gst.ServerStream[*model.NoticeWatchRsp]) error {
	client, err := gstredis.Client()
	if err != nil {
		return gst.NewErrorWithCause(http.StatusInternalServerError, "the notices cannot be watched", err)
	}
	subscription := client.Subscribe(ctx, channel)
	defer subscription.Close()
	arrivals := subscription.Channel()
	for {
		select {
		case <-ctx.Done():
			return nil
		case arrival, ok := <-arrivals:
			if !ok {
				return gst.NewError(http.StatusInternalServerError, "the notice subscription closed")
			}
			var published notice
			if err := json.Unmarshal([]byte(arrival.Payload), &published); err != nil {
				return gst.NewErrorWithCause(http.StatusInternalServerError, "the notice was not read", err)
			}
			if err := stream.Send(&model.NoticeWatchRsp{Text: published.Text, From: published.From, Replica: helper.Replica()}); err != nil {
				return gst.NewErrorWithCause(http.StatusInternalServerError, "failed to send the notice", err)
			}
		}
	}
}
