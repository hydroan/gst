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

type Getter struct {
	service.Base[*model.Cached, *gstmodel.Empty, *model.CachedRsp]
}

// Get answers with what this replica's own store holds for the key. It never
// asks another replica, so an entry that did not propagate here reads as
// missing rather than being fetched on the spot.
func (g *Getter) Get(ctx *gst.ServiceContext, _ *gstmodel.Empty) (*model.CachedRsp, error) {
	key := ctx.Param("id")
	if key == "" {
		return nil, service.NewError(http.StatusBadRequest, "a key is required")
	}
	value, found, err := dao.CacheGet(ctx, key)
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "the entry was not read", err)
	}
	return &model.CachedRsp{Replica: helper.Replica(), Key: key, Value: value, Found: found}, nil
}
