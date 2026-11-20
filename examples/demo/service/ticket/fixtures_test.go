package ticket_test

import (
	"testing"

	"demo/model"

	"github.com/hydroan/gst/testutil/iamtest"
	"github.com/stretchr/testify/require"
)

// createTicket creates a ticket of the account's, in the status given, and
// returns it as the API answered.
func createTicket(t *testing.T, account *iamtest.Account, title string, status model.TicketStatus) *model.Ticket {
	t.Helper()
	ticket, err := account.Client.Post[model.Ticket](t.Context(), "/api/tickets", &model.TicketCreateReq{Title: title, Status: status})
	require.NoError(t, err)
	return ticket
}
