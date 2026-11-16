package ticket_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model"

	"github.com/hydroan/gst/client"
	"github.com/stretchr/testify/require"
)

// TestTrash covers GET /api/tickets/trash, served by Trash in trash.go: the
// tickets the caller deleted, which the ordinary list no longer shows, and
// nobody else's.
func TestTrash(t *testing.T) {
	account := testsupport.Login(t)
	kept := createTicket(t, account, "kept", model.TicketStatusOpen)
	deleted := createTicket(t, account, "deleted", model.TicketStatusOpen)
	_, err := account.Client.Delete[struct{}](t.Context(), "/api/tickets/"+deleted.ID, nil)
	require.NoError(t, err)

	trash, err := account.Client.Get[model.TicketTrashRsp](t.Context(), "/api/tickets/trash")
	require.NoError(t, err)
	require.Equal(t, 1, trash.Total)
	require.Len(t, trash.Items, 1)
	require.Equal(t, deleted.ID, trash.Items[0].ID)

	list, err := account.Client.Get[client.ListResult[*model.Ticket]](t.Context(), "/api/tickets")
	require.NoError(t, err)
	require.Equal(t, 1, list.Total)
	require.Equal(t, kept.ID, list.Items[0].ID)

	trash, err = testsupport.Login(t).Client.Get[model.TicketTrashRsp](t.Context(), "/api/tickets/trash")
	require.NoError(t, err)
	require.Equal(t, 0, trash.Total)
}
