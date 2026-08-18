package bench

import (
	"bench/model/bench"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Get struct {
	service.Base[*bench.Bench, *model.Empty, *bench.GetRsp]
}

func (g *Get) Get(ctx *types.ServiceContext, req *model.Empty) (rsp *bench.GetRsp, err error) {
	isDryRun := isDryRun(ctx)

	dest := &bench.Bench{}
	if isDryRun {
		_ = database.Database[*bench.Bench](ctx).WithDryRun().Get(dest, "not exists")
	} else {
		_ = database.Database[*bench.Bench](ctx).Get(dest, "not exists")
	}

	return &bench.GetRsp{Msg: "hi get"}, nil
}
