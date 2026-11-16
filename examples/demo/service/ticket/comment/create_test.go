package comment_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model"
	"demo/model/ticket"

	"github.com/stretchr/testify/require"
)

// TestCreate covers POST /api/tickets/:ticket/comments, served by Creator in
// create.go: the comment belongs to the ticket of the route.
func TestCreate(t *testing.T) {
	account := testsupport.Login(t)
	parent, err := account.Client.Post[model.Ticket](t.Context(), "/api/tickets", &model.Ticket{Title: "parent"})
	require.NoError(t, err)

	comment, err := account.Client.Post[ticket.Comment](t.Context(), "/api/tickets/"+parent.ID+"/comments", &ticket.Comment{Kind: ticket.CommentKindQuestion, Content: "one"})
	require.NoError(t, err)
	require.Equal(t, parent.ID, comment.TicketID)
	require.NotEmpty(t, comment.ID)
}
