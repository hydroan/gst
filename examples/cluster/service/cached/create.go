package cached

import (
	"net/http"

	"cluster/dao"
	"cluster/helper"
	"cluster/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Creator struct {
	service.Base[*model.Cached, *model.CachedReq, *model.CachedRsp]
}

// Create writes the entry into the replica's own store and publishes it to
// the others. The reply names the replica that wrote it, which is the one
// every other replica must catch up with.
func (c *Creator) Create(ctx *gst.ServiceContext, req *model.CachedReq) (*model.CachedRsp, error) {
	if req.Key == "" {
		return nil, service.NewError(http.StatusBadRequest, "a key is required")
	}
	if err := dao.CacheSet(ctx, req.Key, req.Value); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "the entry was not cached", err)
	}
	return &model.CachedRsp{Replica: helper.Replica(), Key: req.Key, Value: req.Value, Found: true}, nil
}
