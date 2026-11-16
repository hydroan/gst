package comment

import (
	"net/http"

	"demo/model"
	"demo/model/ticket"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	gstmodel "github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
)

// Summary answers GET /api/comments/summary: the caller's comments counted
// by the status of their ticket, one query joining the parent, with
// database.Select taking its group key from the joined model.
type Summary struct {
	service.Base[*ticket.Comment, *gstmodel.Empty, *ticket.CommentSummaryRsp]
}

// List counts the comments by ticket status. The join pins the ticket by
// its primary key, so no comment is counted twice, and the owner condition
// on the ticket keeps the figures the caller's own; a status the caller has
// no comments under is absent rather than zero.
func (s *Summary) List(ctx *gst.ServiceContext, _ *gstmodel.Empty) (*ticket.CommentSummaryRsp, error) {
	type commentsOfStatus struct {
		Status   model.TicketStatus
		Comments int
	}
	rows := make([]commentsOfStatus, 0)
	if err := database.Select[*ticket.Comment, commentsOfStatus](ctx, model.TicketCols.Status.Group(), gst.Count().As("comments")).
		Join(gst.Join[*model.Ticket](model.TicketCols.ID.EqCol(ticket.CommentCols.TicketID))).
		Where(model.TicketCols.UserID.Eq(ctx.UserID())).
		Scan(&rows); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to count the comments", err)
	}
	rsp := &ticket.CommentSummaryRsp{ByTicketStatus: make(map[string]int, len(rows))}
	for _, row := range rows {
		rsp.ByTicketStatus[string(row.Status)] = row.Comments
	}
	return rsp, nil
}
