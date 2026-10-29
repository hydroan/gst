package cronjob

import (
	"context"

	"multitenant/model"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/tenant"
)

// The digest_tickets job runs every minute.
const (
	digestTicketsName = "digest_tickets"
	digestTicketsSpec = "@every 1m"
)

// digestTickets writes every tenant a digest of its tickets. The tenants
// come from the registry, a global table a job reads as it is; the count
// and the row of each tenant are done on a context acting in that tenant,
// tenant.In, which is what a job has in place of a request's authorization:
// the count sees the tenant's tickets alone, and the digest is stamped with
// the tenant the job acted in.
func digestTickets(ctx context.Context) error {
	tenants := make([]*model.Tenant, 0)
	if err := database.Database[*model.Tenant](ctx).List(&tenants); err != nil {
		return err
	}
	for _, t := range tenants {
		scoped := tenant.In(ctx, t.ID)
		digest := &model.Digest{At: database.DB().NowFunc()}
		if err := database.Database[*model.Ticket](scoped).
			WithQuery(&model.Ticket{Status: model.TicketStatusOpen}).Count(&digest.Open); err != nil {
			return err
		}
		if err := database.Database[*model.Ticket](scoped).
			WithQuery(&model.Ticket{Status: model.TicketStatusClosed}).Count(&digest.Closed); err != nil {
			return err
		}
		if err := database.Database[*model.Digest](scoped).Create(digest); err != nil {
			return errors.Wrapf(err, "failed to write the digest of tenant %s", t.ID)
		}
	}
	return nil
}
