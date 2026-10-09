package feed

import (
	"net/http"
	"strconv"

	"demo/model/board"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/service"
)

// Watch serves the WatchFeed rpc, a server stream: one request in, a stream
// of events out.
type Watch struct {
	service.Base[*board.Feed, *board.FeedWatchReq, *board.FeedWatchRsp]
}

// Stream sends three events of the topic and returns, which ends the
// stream. A live feed would block on its source instead and send until ctx
// is done, which is when the client has gone.
func (w *Watch) Stream(_ *gst.ServiceContext, req *board.FeedWatchReq, stream *grpc.ServerStream[*board.FeedWatchRsp]) error {
	for seq := int64(1); seq <= 3; seq++ {
		if err := stream.Send(&board.FeedWatchRsp{Seq: seq, Body: req.Topic + " #" + strconv.FormatInt(seq, 10)}); err != nil {
			return gst.NewErrorWithCause(http.StatusInternalServerError, "failed to send the event", err)
		}
	}
	return nil
}
