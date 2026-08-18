package bench

import (
	"net/http"

	"bench/model/bench"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Delete struct {
	service.Base[*bench.Bench, *model.Empty, *bench.DeleteRsp]
}

// Delete benchmarks the framework's standard Delete write path: one real
// DELETE statement against a primary key that does not exist (Bench.Purge()
// is true, so the model's delete policy is a hard delete), exercising the
// full chain of hook detection, transaction routing, and the DB round trip
// while matching zero rows and changing nothing. Delete does not error on
// zero matched rows; the benchmark therefore needs no seeding and no cleanup,
// and table contents do not affect the numbers.
func (d *Delete) Delete(ctx *types.ServiceContext, req *model.Empty) (rsp *bench.DeleteRsp, err error) {
	isDryRun := isDryRun(ctx)

	data := &bench.Bench{Base: model.Base{ID: "not exists"}}

	if isDryRun {
		err = database.Database[*bench.Bench](ctx).WithDryRun().Delete(data)
	} else {
		err = database.Database[*bench.Bench](ctx).Delete(data)
	}
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "delete bench data failed", err)
	}

	return &bench.DeleteRsp{Msg: "hi delete"}, nil
}
