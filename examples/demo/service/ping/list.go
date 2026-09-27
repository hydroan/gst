package ping

import (
	"demo/component"
	"demo/model"

	"github.com/hydroan/gst"
	gstmodel "github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
)

// Lister answers the ping: a custom action, since the model's request and
// response types differ, so the framework calls List and does nothing of
// its own.
type Lister struct {
	service.Base[*model.Ping, *gstmodel.Empty, *model.PingRsp]
}

func (p *Lister) List(*gst.ServiceContext, *gstmodel.Empty) (*model.PingRsp, error) {
	return &model.PingRsp{Msg: "pong", Records: component.RecordCount()}, nil
}
