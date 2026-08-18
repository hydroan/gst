package bench

import (
	"net/http"

	"bench/model/bench"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Update struct {
	service.Base[*bench.Bench, *bench.UpdateReq, *bench.UpdateRsp]
}

// Update benchmarks the framework's standard Update write path: one real
// UPDATE against a primary key that does not exist, exercising the full chain
// of binding, hook detection, transaction routing, and the DB round trip
// while matching zero rows and writing nothing. The framework reports zero
// matched rows as ErrRecordNotFound — the expected outcome of this setup —
// which is swallowed as success; the benchmark therefore needs no seeding and
// no cleanup, and table contents do not affect the numbers.
func (u *Update) Update(ctx *types.ServiceContext, req *bench.UpdateReq) (rsp *bench.UpdateRsp, err error) {
	isDryRun := isDryRun(ctx)

	data := &bench.Bench{
		Field1: req.Field1,
		Field2: req.Field2,
		Field3: req.Field3,
		Field4: req.Field4,
		Base:   model.Base{ID: "not exists"},
	}

	if isDryRun {
		err = database.Database[*bench.Bench](ctx).WithDryRun().Update(data)
	} else {
		err = database.Database[*bench.Bench](ctx).Update(data)
	}
	if err != nil && !errors.Is(err, database.ErrRecordNotFound) {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "update bench data failed", err)
	}

	return &bench.UpdateRsp{Msg: "hi update"}, nil
}
