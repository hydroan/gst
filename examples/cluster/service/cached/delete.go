package cached

import (
	"net/http"

	"cluster/dao"
	"cluster/helper"
	"cluster/model"

	"github.com/hydroan/gst"
	gstmodel "github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
)

type Deleter struct {
	service.Base[*model.Cached, *gstmodel.Empty, *model.CachedRsp]
}

// Delete removes the entry here and publishes the removal to every other
// replica, which is the operation a replica that is not listening misses:
// its own store keeps serving the entry until the ttl runs out.
func (d *Deleter) Delete(ctx *gst.ServiceContext, _ *gstmodel.Empty) (*model.CachedRsp, error) {
	key := ctx.Param("id")
	if key == "" {
		return nil, service.NewError(http.StatusBadRequest, "a key is required")
	}
	if err := dao.CacheDelete(ctx, key); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "the entry was not removed", err)
	}
	return &model.CachedRsp{Replica: helper.Replica(), Key: key}, nil
}
