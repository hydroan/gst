package stepdown

import (
	"cluster/helper"
	"cluster/leader"
	"cluster/model"

	"github.com/hydroan/gst"
	gstmodel "github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
)

type Creator struct {
	service.Base[*model.StepDown, *gstmodel.Empty, *model.StepDownRsp]
}

// Create asks the leader work running on this replica to return. The reply
// says whether the request was taken; what the framework then does with the
// name — hands it back, campaigns again a few seconds later — is in the
// leader log of this replica and of whichever takes the name next.
func (c *Creator) Create(ctx *gst.ServiceContext, _ *gstmodel.Empty) (*model.StepDownRsp, error) {
	return &model.StepDownRsp{Replica: helper.Replica(), Asked: leader.StepDown()}, nil
}
