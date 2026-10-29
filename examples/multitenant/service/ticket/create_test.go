package ticket_test

import (
	"net/http"
	"testing"

	"multitenant/internal/testsupport"
	"multitenant/model"
	"multitenant/pb"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestCreate covers POST /api/tickets and the CreateTicket rpc, served by
// the framework with the hook of Creator in create.go, and the row scoping
// around them: a ticket is stamped with the tenant the caller acts in and
// keyed by it whatever the body names, another tenant sees nothing of it,
// a code is free for another tenant and taken within one, a role without
// the manage menu may read but not write, an update cannot move a ticket to
// another tenant, and the system root, acting in no one tenant, names it.
func TestCreate(t *testing.T) {
	alice := testsupport.Login(t, "alice", "alpha")
	carol := testsupport.Login(t, "carol", "beta")

	ticket, err := alice.Client.Post[model.Ticket](t.Context(), "/api/tickets", map[string]any{"code": "T-1", "title": "printer", "tenant_id": "beta"})
	require.NoError(t, err)
	require.Equal(t, "alpha", ticket.GetTenantID(), "the stamp writes the caller's tenant over what the body named")
	require.Equal(t, "alpha/T-1", ticket.Key)
	require.Equal(t, model.TicketStatusOpen, ticket.Status)

	t.Run("another tenant sees nothing of it", func(t *testing.T) {
		_, err := carol.Client.Get[model.Ticket](t.Context(), "/api/tickets/"+ticket.ID)
		testutil.RequireError(t, err, http.StatusNotFound)
		list, err := carol.Client.Get[client.ListResult[*model.Ticket]](t.Context(), "/api/tickets", client.WithQuery("code", "T-1"))
		require.NoError(t, err)
		require.Equal(t, 0, list.Total)
		_, err = carol.Client.Delete[struct{}](t.Context(), "/api/tickets/"+ticket.ID, nil)
		testutil.RequireError(t, err, http.StatusNotFound)
	})

	t.Run("the code is free for another tenant and taken within one", func(t *testing.T) {
		theirs, err := carol.Client.Post[model.Ticket](t.Context(), "/api/tickets", &model.Ticket{Code: "T-1", Title: "theirs"})
		require.NoError(t, err)
		require.Equal(t, "beta/T-1", theirs.Key)
		_, err = alice.Client.Post[model.Ticket](t.Context(), "/api/tickets", &model.Ticket{Code: "T-1", Title: "again"})
		testutil.RequireError(t, err, http.StatusConflict)
	})

	t.Run("a role without the manage menu may read but not write", func(t *testing.T) {
		bob := testsupport.Login(t, "bob", "alpha")
		got, err := bob.Client.Get[model.Ticket](t.Context(), "/api/tickets/"+ticket.ID)
		require.NoError(t, err)
		require.Equal(t, ticket.ID, got.ID)
		_, err = bob.Client.Post[model.Ticket](t.Context(), "/api/tickets", &model.Ticket{Code: "T-3"})
		testutil.RequireError(t, err, http.StatusForbidden, "permission denied")
	})

	t.Run("an update cannot move it to another tenant", func(t *testing.T) {
		_, err := alice.Client.Put[model.Ticket](t.Context(), "/api/tickets/"+ticket.ID, map[string]any{"code": "T-1", "title": "moved", "status": "open", "tenant_id": "beta"})
		testutil.RequireError(t, err, http.StatusBadRequest)
	})

	t.Run("the system root names the tenant", func(t *testing.T) {
		// The system root may log into any tenant, member or not, and acts
		// across all of them: its body names the tenant a row is filed
		// under, and a row naming none goes to the tenant the request acts
		// in.
		root := testsupport.Login(t, "root", "alpha")
		filed, err := root.Client.Post[model.Ticket](t.Context(), "/api/tickets", map[string]any{"code": "T-9", "title": "filed for beta", "tenant_id": "beta"})
		require.NoError(t, err)
		require.Equal(t, "beta", filed.GetTenantID())
		require.Equal(t, "beta/T-9", filed.Key)
		seen, err := carol.Client.Get[model.Ticket](t.Context(), "/api/tickets/"+filed.ID)
		require.NoError(t, err)
		require.Equal(t, filed.ID, seen.ID)

		unnamed, err := root.Client.Post[model.Ticket](t.Context(), "/api/tickets", &model.Ticket{Code: "T-10"})
		require.NoError(t, err)
		require.Equal(t, "alpha", unnamed.GetTenantID())
		require.Equal(t, "alpha/T-10", unnamed.Key)
	})

	t.Run("over gRPC the stamp holds as well", func(t *testing.T) {
		tickets := pb.NewTicketServiceClient(testsupport.Dial(t))
		ctx := testsupport.Authorized(t.Context(), testsupport.Session(t, "alice", "alpha"))
		created, err := tickets.CreateTicket(ctx, &pb.CreateTicketRequest{Ticket: &pb.Ticket{Code: "T-2", Title: "over grpc", TenantId: "beta"}})
		require.NoError(t, err)
		require.Equal(t, "alpha", created.GetTicket().GetTenantId())
		require.Equal(t, "alpha/T-2", created.GetTicket().GetKey())
	})
}
