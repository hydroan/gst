package bench

import (
	"net/http"

	"bench/model/bench"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Updatebyid struct {
	service.Base[*bench.Bench, *bench.UpdateByIDReq, *bench.UpdateByIDRsp]
}

// Patch benchmarks the database.UpdateByID write path — no hooks and no
// transaction wrapper, the lightest write the framework offers. It issues one
// real UPDATE against a primary key that does not exist; UpdateByID returns
// nil for a missing record, so the benchmark needs no seeding and no cleanup,
// and table contents do not affect the numbers.
func (u *Updatebyid) Patch(ctx *types.ServiceContext, req *bench.UpdateByIDReq) (rsp *bench.UpdateByIDRsp, err error) {
	isDryRun := isDryRun(ctx)

	if isDryRun {
		err = database.Database[*bench.Bench](ctx).WithDryRun().UpdateByID("not exists", bench.BenchCols.Field1.Set(req.Field1))
	} else {
		err = database.Database[*bench.Bench](ctx).UpdateByID("not exists", bench.BenchCols.Field1.Set(req.Field1))
	}
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "update bench data by id failed", err)
	}

	return &bench.UpdateByIDRsp{Msg: "hi updatebyid"}, nil
}
