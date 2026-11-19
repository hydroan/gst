package ticket_test

import (
	"net/http"
	"testing"

	"demo/internal/testsupport"
	"demo/model"

	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestGet covers GET /api/tickets/:ticket, served by the framework with the
// hook of Getter in get.go: the owner reads the ticket, and someone else's
// ticket is not there to read.
func TestGet(t *testing.T) {
	account := testsupport.Login(t)
	ticket := createTicket(t, account, "draft", model.TicketStatusOpen)

	read, err := account.Client.Get[model.Ticket](t.Context(), "/api/tickets/"+ticket.ID)
	require.NoError(t, err)
	require.Equal(t, ticket.ID, read.ID)
	require.Equal(t, "draft", read.Title)

	_, err = testsupport.Login(t).Client.Get[model.Ticket](t.Context(), "/api/tickets/"+ticket.ID)
	testutil.RequireError(t, err, http.StatusNotFound)
}
