package record

import (
	"net/http"

	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	gstmodel "github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
)

// Trash answers GET /api/records/trash, the records the caller deleted: a
// custom List reading past the soft deletion, which every other read of the
// table leaves out.
type Trash struct {
	service.Base[*model.Record, *gstmodel.Empty, *model.RecordTrashRsp]
}

// List lists the records the caller deleted, latest first, and counts them.
// WithDeleted lifts the soft-delete condition alone; the owner condition and
// the filter on the deletion time are the service's, which is what keeps
// the live rows out.
func (t *Trash) List(ctx *gst.ServiceContext, _ *gstmodel.Empty) (*model.RecordTrashRsp, error) {
	opts := gst.QueryOptions{Filters: []gst.Filter{model.RecordCols.DeletedAt.IsNotNull()}}
	rsp := &model.RecordTrashRsp{Items: make([]*model.Record, 0)}
	if err := database.Database[*model.Record](ctx).
		WithDeleted().
		WithQuery(&model.Record{UserID: ctx.UserID()}, opts).
		WithOrder(model.RecordCols.DeletedAt.Desc()).
		List(&rsp.Items); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to list the deleted records", err)
	}
	if err := database.Database[*model.Record](ctx).
		WithDeleted().
		WithQuery(&model.Record{UserID: ctx.UserID()}, opts).
		Count(&rsp.Total); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to count the deleted records", err)
	}
	return rsp, nil
}
