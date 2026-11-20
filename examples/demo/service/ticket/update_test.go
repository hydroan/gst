package ticket_test

import (
	"net/http"
	"testing"

	"demo/internal/testsupport"
	"demo/model"

	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestUpdate covers PUT /api/tickets/:ticket, served by Updater in
// update.go: the body replaces the title and the status and the owner
// stays, and the version the body carries decides whether the write goes
// through, a stale one answering 409 and none 400; someone else's ticket is
// not there to update.
func TestUpdate(t *testing.T) {
	account := testsupport.Login(t)
	ticket := createTicket(t, account, "draft", model.TicketStatusOpen)

	updated, err := account.Client.Put[model.Ticket](t.Context(), "/api/tickets/"+ticket.ID, &model.TicketUpdateReq{Title: "final", Status: model.TicketStatusClosed, Version: ticket.Version})
	require.NoError(t, err)
	require.Equal(t, "final", updated.Title)
	require.Equal(t, model.TicketStatusClosed, updated.Status)
	require.Equal(t, account.UserID, updated.UserID)
	require.EqualValues(t, 2, updated.Version)

	_, err = account.Client.Put[model.Ticket](t.Context(), "/api/tickets/"+ticket.ID, &model.TicketUpdateReq{Title: "stale", Status: model.TicketStatusOpen, Version: ticket.Version})
	testutil.RequireError(t, err, http.StatusConflict)

	_, err = account.Client.Put[model.Ticket](t.Context(), "/api/tickets/"+ticket.ID, &model.TicketUpdateReq{Title: "unversioned", Status: model.TicketStatusOpen})
	testutil.RequireError(t, err, http.StatusBadRequest)

	_, err = testsupport.Login(t).Client.Put[model.Ticket](t.Context(), "/api/tickets/"+ticket.ID, &model.TicketUpdateReq{Title: "taken", Status: model.TicketStatusOpen, Version: updated.Version})
	testutil.RequireError(t, err, http.StatusNotFound)
	require.Equal(t, "final", testutil.RequireGet[model.Ticket](t, ticket.ID).Title)
}
