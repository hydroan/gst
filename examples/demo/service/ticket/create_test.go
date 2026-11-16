package ticket_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model"

	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestCreate covers POST /api/tickets, served by Creator in create.go: the
// owner is the session's user, the status defaults to open, and the creation
// leaves an audit row.
func TestCreate(t *testing.T) {
	account := testsupport.Login(t)

	ticket, err := account.Client.Post[model.Ticket](t.Context(), "/api/tickets", &model.Ticket{Title: "first"})
	require.NoError(t, err)
	require.Equal(t, account.UserID, ticket.UserID)
	require.Equal(t, model.TicketStatusOpen, ticket.Status)

	stored := testutil.RequireGet[model.Ticket](t, ticket.ID)
	require.Equal(t, "first", stored.Title)
	require.Equal(t, 1, testutil.RequireCount[model.Audit](t, &model.Audit{TicketID: ticket.ID, Action: "create", Actor: account.Username}))
}
