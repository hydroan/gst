package ticket

import (
	"multitenant/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/tenant"
)

// Creator hooks the framework's own Create. Nothing here names the tenant:
// the stamp writes the tenant the request acts in onto the row after the
// hooks, over whatever the body carried; for a cross-tenant caller, the
// system root, the row carries the tenant the body named, or the one the
// request acts in when it named none.
type Creator struct {
	service.Base[*model.Ticket, *model.Ticket, *model.Ticket]
}

// CreateBefore derives the key, the tenant and the code joined, from the
// tenant the row will carry, the way the stamp decides it: tenant.From
// reports it for a request acting in one tenant; for a cross-tenant caller
// it reports none, and the row then carries the tenant its body named, or
// the one the request acts in when it named none. A ticket opens unless
// the body says otherwise.
func (c *Creator) CreateBefore(ctx *gst.ServiceContext, ticket *model.Ticket) error {
	tenantID, ok := tenant.From(ctx)
	if !ok {
		tenantID = ticket.GetTenantID()
		if tenantID == "" {
			tenantID = ctx.TenantID()
		}
	}
	ticket.Key = tenantID + "/" + ticket.Code
	if ticket.Status == "" {
		ticket.Status = model.TicketStatusOpen
	}
	return nil
}
