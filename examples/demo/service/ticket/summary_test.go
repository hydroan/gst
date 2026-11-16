package ticket_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model"

	"github.com/stretchr/testify/require"
)

// TestSummary covers GET /api/tickets/summary, served by Summary in
// summary.go: the caller's tickets counted in all and by status.
func TestSummary(t *testing.T) {
	account := testsupport.Login(t)
	createTicket(t, account, "open one", model.TicketStatusOpen)
	createTicket(t, account, "open two", model.TicketStatusOpen)
	createTicket(t, account, "closed one", model.TicketStatusClosed)
	createTicket(t, testsupport.Login(t), "someone else's", model.TicketStatusOpen)

	summary, err := account.Client.Get[model.TicketSummaryRsp](t.Context(), "/api/tickets/summary")
	require.NoError(t, err)
	require.Equal(t, 3, summary.Total)
	require.Equal(t, map[model.TicketStatus]int{model.TicketStatusOpen: 2, model.TicketStatusClosed: 1}, summary.ByStatus)
}
