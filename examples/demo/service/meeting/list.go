package meeting

import (
	"time"

	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/datatypes"
	"github.com/hydroan/gst/service"
)

// upcomingDays is how many days upcoming=true covers, today among them.
const upcomingDays = 7

// Lister hooks the framework's own List.
type Lister struct {
	service.Base[*model.Meeting, *model.Meeting, *model.Meeting]
}

// Filter serves the two parameters of a list of meetings that are no
// columns, with the column references generated for the fields: tag keeps
// the meetings carrying it, GET /api/meetings?tag=weekly, a membership test
// the reference of a JSON array of strings offers, and upcoming=true keeps
// the ones of today and the six days after it, counted by the day itself.
func (m *Lister) Filter(ctx *gst.ServiceContext, meeting *model.Meeting, opts gst.QueryOptions) (*model.Meeting, gst.QueryOptions, error) {
	if meeting.Tag != "" {
		opts.Filters = append(opts.Filters, model.MeetingCols.Tags.JSONContains(meeting.Tag))
	}
	if meeting.Upcoming {
		today := datatypes.Date(time.Now().UTC())
		opts.Filters = append(
			opts.Filters,
			model.MeetingCols.Day.Gte(today),
			model.MeetingCols.Day.Lte(today.AddDays(upcomingDays-1)),
		)
	}
	return meeting, opts, nil
}
