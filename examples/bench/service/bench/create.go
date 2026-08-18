package bench

import (
	"net/http"

	"bench/model/bench"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Create struct {
	service.Base[*bench.Bench, *bench.CreateReq, *bench.CreateRsp]
}

func (c *Create) Create(ctx *types.ServiceContext, req *bench.CreateReq) (rsp *bench.CreateRsp, err error) {
	isDryRun := isDryRun(ctx)

	data := &bench.Bench{
		Field1: req.Field1,
		Field2: req.Field2,
		Field3: req.Field3,
		Field4: req.Field4,
	}

	if isDryRun {
		err = database.Database[*bench.Bench](ctx).WithDryRun().Create(data)
	} else {
		err = database.Database[*bench.Bench](ctx).Create(data)
	}
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "create bench data failed", err)
	}

	return data, nil
}
