package cacheentry

import (
	"io"
	"net/http"

	"cluster/dao"
	"cluster/helper"
	"cluster/model"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Exchange serves the exchange stream of the cache, the ExchangeCacheEntry rpc
// over gRPC and a WebSocket connection over HTTP: keys in, and for each what
// this replica's own store holds, for as long as the client keeps asking.
type Exchange struct {
	service.Base[*model.CacheEntry, *model.CacheEntryKeyReq, *model.CacheEntryExchangeRsp]
}

// Stream answers every key until the client closes its side, which Recv
// reports as io.EOF; returning then ends the stream.
func (e *Exchange) Stream(ctx *gst.ServiceContext, stream *gst.BidiStream[*model.CacheEntryKeyReq, *model.CacheEntryExchangeRsp]) error {
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
		if err := stream.Send(&model.CacheEntryExchangeRsp{Replica: helper.Replica(), Key: req.Key, Value: value, Found: found}); err != nil {
			return gst.NewErrorWithCause(http.StatusInternalServerError, "failed to send the entry", err)
		}
	}
}
