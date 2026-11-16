package cronjob

import (
	"context"

	"demo/dao"
	"demo/model"

	"github.com/hydroan/gst/database"
)

// The count_tickets job runs every half minute on every replica: a
// per-instance job refreshes what this process keeps for itself, so each
// replica counts for its own figure, and a missed instant is not caught up.
const (
	countTicketsName = "count_tickets"
	countTicketsSpec = "@every 30s"
)

// countTickets refreshes the ticket count the ping answers from.
func countTickets(ctx context.Context) error {
	var total int
	if err := database.Database[*model.Ticket](ctx).Count(&total); err != nil {
		return err
	}
	dao.SetTicketCount(int64(total))
	return nil
}
