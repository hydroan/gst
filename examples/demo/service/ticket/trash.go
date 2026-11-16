package ticket

import (
	"net/http"

	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	gstmodel "github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
)

// Trash answers GET /api/tickets/trash, the tickets the caller deleted: a
// custom List reading past the soft deletion, which every other read of the
// table leaves out.
type Trash struct {
	service.Base[*model.Ticket, *gstmodel.Empty, *model.TicketTrashRsp]
}

// List lists the tickets the caller deleted, latest first, and counts them.
// WithDeleted lifts the soft-delete condition alone; the owner condition and
// the filter on the deletion time are the service's, which is what keeps
// the live rows out.
func (t *Trash) List(ctx *gst.ServiceContext, _ *gstmodel.Empty) (*model.TicketTrashRsp, error) {
	opts := gst.QueryOptions{Filters: []gst.Filter{model.TicketCols.DeletedAt.IsNotNull()}}
	rsp := &model.TicketTrashRsp{Items: make([]*model.Ticket, 0)}
	if err := database.Database[*model.Ticket](ctx).
		WithDeleted().
		WithQuery(&model.Ticket{UserID: ctx.UserID()}, opts).
		WithOrder(model.TicketCols.DeletedAt.Desc()).
		List(&rsp.Items); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to list the deleted tickets", err)
	}
	if err := database.Database[*model.Ticket](ctx).
		WithDeleted().
		WithQuery(&model.Ticket{UserID: ctx.UserID()}, opts).
		Count(&rsp.Total); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to count the deleted tickets", err)
	}
	return rsp, nil
}
