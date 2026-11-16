package tally

import (
	"io"
	"net/http"

	"demo/model"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Sum serves the tally connection, a bidirectional stream: amounts in,
// totals out.
type Sum struct {
	service.Base[*model.Tally, *model.TallySumReq, *model.TallySumRsp]
}

// Stream answers every amount with the total the connection has carried so
// far. The total lives in the call, and the call lasts as long as the
// connection does: another connection starts at zero. The client says it has
// no more amounts by closing its side, which Recv reports as io.EOF;
// returning then closes the connection.
func (s *Sum) Stream(_ *gst.ServiceContext, stream *gst.BidiStream[*model.TallySumReq, *model.TallySumRsp]) error {
	var total int64
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return gst.NewErrorWithCause(http.StatusBadRequest, "failed to read the amount", err)
		}
		total += req.Amount
		if err := stream.Send(&model.TallySumRsp{Total: total}); err != nil {
			return gst.NewErrorWithCause(http.StatusInternalServerError, "failed to send the total", err)
		}
	}
}
