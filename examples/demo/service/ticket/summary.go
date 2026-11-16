package ticket

import (
	"net/http"

	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	gstmodel "github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
)

// Summary answers GET /api/tickets/summary, a custom List: a GET action
// declares no Payload, so its request type is model.Empty, and what it
// answers is its own.
type Summary struct {
	service.Base[*model.Ticket, *gstmodel.Empty, *model.TicketSummaryRsp]
}

// List counts the caller's tickets by status in one query: database.Select
// projects the group key, the status, and a count under the alias the result
// row names, and the framework derives the GROUP BY from the key. The
// owner condition is the service's to write: a projection runs no service
// hook, so nothing narrows it to the caller but this Where.
func (s *Summary) List(ctx *gst.ServiceContext, _ *gstmodel.Empty) (*model.TicketSummaryRsp, error) {
	type ticketsOfStatus struct {
		Status model.TicketStatus
		Total  int
	}
	rows := make([]ticketsOfStatus, 0)
	if err := database.Select[*model.Ticket, ticketsOfStatus](ctx, model.TicketCols.Status.Group(), gst.Count().As("total")).
		Where(model.TicketCols.UserID.Eq(ctx.UserID())).
		Scan(&rows); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to count the tickets", err)
	}
	rsp := &model.TicketSummaryRsp{ByStatus: make(map[model.TicketStatus]int, len(rows))}
	for _, row := range rows {
		rsp.ByStatus[row.Status] = row.Total
		rsp.Total += row.Total
	}
	return rsp, nil
}
