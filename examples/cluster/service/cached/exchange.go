package cached

import (
	"io"
	"net/http"

	"cluster/dao"
	"cluster/helper"
	"cluster/model"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/service"
)

// Exchange serves the ExchangeCached rpc, a stream both ways: keys in, and
// for each what this replica's own store holds, for as long as the client
// keeps asking.
type Exchange struct {
	service.Base[*model.Cached, *model.CachedKeyReq, *model.CachedExchangeRsp]
}

// Stream answers every key until the client closes its side, which Recv
// reports as io.EOF; returning then ends the stream.
func (e *Exchange) Stream(ctx *gst.ServiceContext, stream *grpc.BidiStream[*model.CachedKeyReq, *model.CachedExchangeRsp]) (err error) {
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return gst.NewErrorWithCause(http.StatusBadRequest, "failed to read the key", err)
		}
		value, found, err := dao.CacheGet(ctx, req.Key)
		if err != nil {
			return gst.NewErrorWithCause(http.StatusInternalServerError, "the entry was not read", err)
		}
		if err := stream.Send(&model.CachedExchangeRsp{Replica: helper.Replica(), Key: req.Key, Value: value, Found: found}); err != nil {
			return gst.NewErrorWithCause(http.StatusInternalServerError, "failed to send the entry", err)
		}
	}
}
