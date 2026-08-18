package bench

import (
	"bench/model/bench"

	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Ping struct {
	service.Base[*bench.Bench, *model.Empty, *bench.PingRsp]
}

func (p *Ping) List(ctx *types.ServiceContext, req *model.Empty) (rsp *bench.PingRsp, err error) {
	return &bench.PingRsp{Msg: "pong"}, nil
}
