package comment_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model"
	"demo/model/ticket"

	"github.com/hydroan/gst/client"
	"github.com/stretchr/testify/require"
)

// TestList covers GET /api/tickets/:ticket/comments, served by Lister in
// list.go: the comments of the ticket of the route, and no other's.
func TestList(t *testing.T) {
	account := testsupport.Login(t)
	parents := make([]*model.Ticket, 0, 2)
	for _, title := range []string{"one", "two"} {
		parent, err := account.Client.Post[model.Ticket](t.Context(), "/api/tickets", &model.TicketCreateReq{Title: title})
		require.NoError(t, err)
		parents = append(parents, parent)
		for _, content := range []string{title + " a", title + " b"} {
			_, err := account.Client.Post[ticket.Comment](t.Context(), "/api/tickets/"+parent.ID+"/comments", &ticket.Comment{Kind: ticket.CommentKindQuestion, Content: content})
			require.NoError(t, err)
		}
	}

	list, err := account.Client.Get[client.ListResult[ticket.Comment]](t.Context(), "/api/tickets/"+parents[0].ID+"/comments")
	require.NoError(t, err)
	require.Equal(t, 2, list.Total)
	for _, comment := range list.Items {
		require.Equal(t, parents[0].ID, comment.TicketID)
	}
}
