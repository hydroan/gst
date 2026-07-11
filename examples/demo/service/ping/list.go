package ping

import (
	"demo/model"

	"github.com/hydroan/gst/database"
	gstmodel "github.com/hydroan/gst/model"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Lister struct {
	service.Base[*model.Ping, *gstmodel.Empty, *model.PingRsp]
}

func (p *Lister) List(ctx *types.ServiceContext, req *gstmodel.Empty) (rsp *model.PingRsp, err error) {
	users := make([]*iam.User, 0)
	n := new(int64)
	// _ = database.Database[*iam.User](ctx).WithDryRun().List(&users)
	// _ = database.Database[*iam.User](ctx).WithDryRun().Count(n)

	_ = database.Database[*iam.User](ctx).List(&users)
	_ = database.Database[*iam.User](ctx).Count(n)

	// sqls := make([]types.SQLStatement, 0)
	//
	// _ = database.Database[*iam.User](ctx).WithBuildSQL(&sqls).WithQuery(&iam.User{Username: "test"}).List(&users)
	// pretty.Println(sqls)
	// _ = database.Database[*iam.User](ctx).WithBuildSQL(&sqls).Count(n)
	// pretty.Println(sqls)

	return &model.PingRsp{
		Msg: "pong",
	}, nil
}
