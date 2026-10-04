package flag

import (
	"net/http"

	"cluster/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Creator hooks the framework's own Create of a flag, which serves POST
// /api/flags and the CreateFlag rpc alike: the framework stores the flag, the
// hook checks it and fills in what the request left out.
type Creator struct {
	service.Base[*model.Flag, *model.Flag, *model.Flag]
}

// CreateBefore refuses a flag without a name, 400 over HTTP and
// InvalidArgument over gRPC, and makes a flag turned on without a percent
// apply to everyone.
func (f *Creator) CreateBefore(_ *gst.ServiceContext, flag *model.Flag) error {
	if flag.Name == "" {
		return gst.NewError(http.StatusBadRequest, "a flag needs a name")
	}
	if flag.On && flag.Percent == 0 {
		flag.Percent = 100
	}
	return nil
}
