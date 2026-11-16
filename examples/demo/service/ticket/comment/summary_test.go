package comment_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model"
	"demo/model/ticket"

	"github.com/stretchr/testify/require"
)

// TestSummary covers GET /api/comments/summary, served by Summary in
// summary.go: the caller's comments counted by the status of their ticket,
// and nobody else's.
func TestSummary(t *testing.T) {
	account := testsupport.Login(t)
	for status, contents := range map[model.TicketStatus][]string{
		model.TicketStatusOpen:   {"one", "two"},
		model.TicketStatusClosed: {"three"},
	} {
		parent, err := account.Client.Post[model.Ticket](t.Context(), "/api/tickets", &model.Ticket{Title: string(status), Status: status})
		require.NoError(t, err)
		for _, content := range contents {
			_, err := account.Client.Post[ticket.Comment](t.Context(), "/api/tickets/"+parent.ID+"/comments", &ticket.Comment{Kind: ticket.CommentKindQuestion, Content: content})
			require.NoError(t, err)
		}
	}
	other := testsupport.Login(t)
	parent, err := other.Client.Post[model.Ticket](t.Context(), "/api/tickets", &model.Ticket{Title: "someone else's"})
	require.NoError(t, err)
	_, err = other.Client.Post[ticket.Comment](t.Context(), "/api/tickets/"+parent.ID+"/comments", &ticket.Comment{Kind: ticket.CommentKindAnswer, Content: "theirs"})
	require.NoError(t, err)

	summary, err := account.Client.Get[ticket.CommentSummaryRsp](t.Context(), "/api/comments/summary")
	require.NoError(t, err)
	require.Equal(t, map[string]int{"open": 2, "closed": 1}, summary.ByTicketStatus)
}
