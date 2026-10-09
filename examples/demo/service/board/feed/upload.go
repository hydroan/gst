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

// Upload serves the UploadFeed rpc, a client stream: a stream of events in,
// one answer out.
type Upload struct {
	service.Base[*board.Feed, *board.FeedUploadReq, *board.FeedUploadRsp]
}

// Stream counts the events until the client closes its side, which Recv
// reports as io.EOF, and answers the count.
func (u *Upload) Stream(_ *gst.ServiceContext, stream *grpc.ClientStream[*board.FeedUploadReq]) (*board.FeedUploadRsp, error) {
	var accepted int64
	for {
		if _, err := stream.Recv(); err != nil {
			if errors.Is(err, io.EOF) {
				return &board.FeedUploadRsp{Accepted: accepted}, nil
			}
			return nil, gst.NewErrorWithCause(http.StatusBadRequest, "failed to read the event", err)
		}
		accepted++
	}
}
