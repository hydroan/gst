package bench

import (
	"net/http"

	"bench/model/bench"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/service"
)

type Updatebyid struct {
	service.Base[*bench.Bench, *bench.UpdateByIDReq, *bench.UpdateByIDRsp]
}

// Patch benchmarks the database.UpdateByID write path — no hooks and no
// transaction wrapper, the lightest write the framework offers. It issues one
// real UPDATE against a primary key that does not exist. The framework reports
// zero matched rows as ErrRecordNotFound — the expected outcome of this setup
// — which is swallowed as success; the benchmark therefore needs no seeding
// and no cleanup, and table contents do not affect the numbers.
func (u *Updatebyid) Patch(ctx *gst.ServiceContext, req *bench.UpdateByIDReq) (rsp *bench.UpdateByIDRsp, err error) {
	isDryRun := isDryRun(ctx)

	if isDryRun {
		err = database.Database[*bench.Bench](ctx).WithDryRun().UpdateByID("not exists", bench.BenchCols.Field1.Set(req.Field1))
	} else {
		err = database.Database[*bench.Bench](ctx).UpdateByID("not exists", bench.BenchCols.Field1.Set(req.Field1))
	}
	if err != nil && !errors.Is(err, database.ErrRecordNotFound) {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "update bench data by id failed", err)
	}

	return &bench.UpdateByIDRsp{Msg: "hi updatebyid"}, nil
}
