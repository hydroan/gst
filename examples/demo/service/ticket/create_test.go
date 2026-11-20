package ticket_test

import (
	"net/http"
	"testing"

	"demo/internal/testsupport"
	"demo/model"

	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestCreate covers POST /api/tickets, served by Creator in create.go: the
// owner is the session's user whatever the body names, the status defaults
// to open, the ticket starts at version 1, the creation leaves an audit row,
// and a body without a title is refused before the service runs.
func TestCreate(t *testing.T) {
	account := testsupport.Login(t)

	ticket, err := account.Client.Post[model.Ticket](t.Context(), "/api/tickets", &model.TicketCreateReq{Title: "first"})
	require.NoError(t, err)
	require.Equal(t, account.UserID, ticket.UserID)
	require.Equal(t, model.TicketStatusOpen, ticket.Status)
	require.EqualValues(t, 1, ticket.Version)

	stored := testutil.RequireGet[model.Ticket](t, ticket.ID)
	require.Equal(t, "first", stored.Title)
	require.Equal(t, 1, testutil.RequireCount[model.Audit](t, &model.Audit{TicketID: ticket.ID, Action: "create", Actor: account.Username}))

	other := testsupport.Login(t)
	ticket, err = account.Client.Post[model.Ticket](t.Context(), "/api/tickets", map[string]any{"title": "second", "user_id": other.UserID})
	require.NoError(t, err)
	require.Equal(t, account.UserID, ticket.UserID, "the request has no field naming the owner")

	_, err = account.Client.Post[model.Ticket](t.Context(), "/api/tickets", &model.TicketCreateReq{Status: model.TicketStatusOpen})
	testutil.RequireError(t, err, http.StatusBadRequest)
}
