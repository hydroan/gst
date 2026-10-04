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

// Load serves the LoadCached rpc, a client stream: entries in, one count
// out. Every entry is written to the store of this replica the way a single
// Create is, so its peers catch up with all of them.
type Load struct {
	service.Base[*model.Cached, *model.CachedReq, *model.CachedLoadRsp]
}

// Stream caches every entry until the client closes its side, which Recv
// reports as io.EOF, and answers the count.
func (l *Load) Stream(ctx *gst.ServiceContext, stream *grpc.ClientStream[*model.CachedReq]) (*model.CachedLoadRsp, error) {
	var count int64
	for {
		entry, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return &model.CachedLoadRsp{Replica: helper.Replica(), Count: count}, nil
		}
		if err != nil {
			return nil, gst.NewErrorWithCause(http.StatusBadRequest, "failed to read the entry", err)
		}
		if err := dao.CacheSet(ctx, entry.Key, entry.Value); err != nil {
			return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "the entry was not cached", err)
		}
		count++
	}
}
