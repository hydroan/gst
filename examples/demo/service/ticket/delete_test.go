package ticket_test

import (
	"net/http"
	"testing"

	"demo/internal/testsupport"
	"demo/model"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestDelete covers DELETE /api/tickets/:ticket, served by the framework
// with the hook of Deleter in delete.go: the caller's ticket goes, and
// someone else's is not there to delete.
func TestDelete(t *testing.T) {
	account := testsupport.Login(t)
	ticket := createTicket(t, account, "mine", model.TicketStatusOpen)

	_, err := testsupport.Login(t).Client.Delete[struct{}](t.Context(), "/api/tickets/"+ticket.ID, nil)
	testutil.RequireError(t, err, http.StatusNotFound)
	list, err := account.Client.Get[client.ListResult[*model.Ticket]](t.Context(), "/api/tickets")
	require.NoError(t, err)
	require.Equal(t, 1, list.Total, "a delete refused leaves the ticket")

	_, err = account.Client.Delete[struct{}](t.Context(), "/api/tickets/"+ticket.ID, nil)
	require.NoError(t, err)
	list, err = account.Client.Get[client.ListResult[*model.Ticket]](t.Context(), "/api/tickets")
	require.NoError(t, err)
	require.Equal(t, 0, list.Total)
}
