package ticket_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model"
	"demo/model/ticket"

	"github.com/hydroan/gst/client"
	"github.com/stretchr/testify/require"
)

// TestList covers GET /api/tickets, served by Lister in list.go: a list is
// the caller's own tickets, each carrying the caller's name, and the
// framework's query parameters narrow it further.
func TestList(t *testing.T) {
	alice := testsupport.Login(t)
	bob := testsupport.Login(t)
	createTicket(t, alice, "alice one", model.TicketStatusOpen)
	createTicket(t, alice, "alice two", model.TicketStatusClosed)
	createTicket(t, bob, "bob one", model.TicketStatusOpen)

	list, err := alice.Client.Get[client.ListResult[model.Ticket]](t.Context(), "/api/tickets")
	require.NoError(t, err)
	require.Equal(t, 2, list.Total)
	require.Len(t, list.Items, 2)
	for _, ticket := range list.Items {
		require.Equal(t, alice.UserID, ticket.UserID)
		require.Equal(t, alice.Username, ticket.Username)
	}

	list, err = bob.Client.Get[client.ListResult[model.Ticket]](t.Context(), "/api/tickets")
	require.NoError(t, err)
	require.Equal(t, 1, list.Total)
	require.Equal(t, "bob one", list.Items[0].Title)

	// The framework parses the query parameters of its own list, the
	// model embedding model.Query: a filter narrows it within the owner's.
	list, err = alice.Client.Get[client.ListResult[model.Ticket]](t.Context(), "/api/tickets", client.WithQuery("status[in]", "closed"))
	require.NoError(t, err)
	require.Equal(t, 1, list.Total)
	require.Equal(t, "alice two", list.Items[0].Title)
}

// TestListExpandsTheComments covers the Comments association Ticket declares
// through Expands, on the list Lister hooks and on a get: asked for
// ?_expand=comments, the framework answers each ticket with its comments, and
// not asked, without.
func TestListExpandsTheComments(t *testing.T) {
	account := testsupport.Login(t)
	parent := createTicket(t, account, "with comments", model.TicketStatusOpen)
	for _, content := range []string{"one", "two"} {
		_, err := account.Client.Post[ticket.Comment](t.Context(), "/api/tickets/"+parent.ID+"/comments", &ticket.Comment{Kind: ticket.CommentKindQuestion, Content: content})
		require.NoError(t, err)
	}

	bare, err := account.Client.Get[client.ListResult[model.Ticket]](t.Context(), "/api/tickets")
	require.NoError(t, err)
	require.Len(t, bare.Items, 1)
	require.Empty(t, bare.Items[0].Comments)

	expanded, err := account.Client.Get[client.ListResult[model.Ticket]](t.Context(), "/api/tickets", client.WithExpand("comments", 1))
	require.NoError(t, err)
	require.Len(t, expanded.Items, 1)
	require.Len(t, expanded.Items[0].Comments, 2)

	got, err := account.Client.Get[model.Ticket](t.Context(), "/api/tickets/"+parent.ID, client.WithExpand("comments", 1))
	require.NoError(t, err)
	require.Len(t, got.Comments, 2)
	for _, comment := range got.Comments {
		require.Equal(t, parent.ID, comment.TicketID)
	}
}
