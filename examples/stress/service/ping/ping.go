package ping

import (
	"stress/model"

	"github.com/hydroan/gst"
	gstmodel "github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
)

// Ping serves GET /api/ping: no session, no database, no log line, the
// cheapest request the server answers.
type Ping struct {
	service.Base[*model.Ping, *gstmodel.Empty, *model.PingRsp]
}

func (p *Ping) List(_ *gst.ServiceContext, _ *gstmodel.Empty) (*model.PingRsp, error) {
	return &model.PingRsp{Msg: "pong"}, nil
}
