package preference

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Lister hooks the framework's own List.
type Lister struct {
	service.Base[*model.Preference, *model.Preference, *model.Preference]
}

// Filter keeps a list to the caller's own preferences, whatever the client
// asked for.
func (p *Lister) Filter(ctx *gst.ServiceContext, preference *model.Preference, opts gst.QueryOptions) (*model.Preference, gst.QueryOptions, error) {
	preference.UserID = ctx.UserID()
	return preference, opts, nil
}
