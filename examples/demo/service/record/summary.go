package record

import (
	"net/http"

	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	gstmodel "github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
)

// Summary answers GET /api/records/summary, a custom List: a GET action
// declares no Payload, so its request type is model.Empty, and what it
// answers is its own.
type Summary struct {
	service.Base[*model.Record, *gstmodel.Empty, *model.RecordSummaryRsp]
}

// List counts the caller's records, in all and by type, through the
// database chain: WithQuery takes a model whose set fields are the
// conditions.
func (s *Summary) List(ctx *gst.ServiceContext, _ *gstmodel.Empty) (*model.RecordSummaryRsp, error) {
	rsp := &model.RecordSummaryRsp{ByType: map[model.RecordType]int{}}
	if err := database.Database[*model.Record](ctx).WithQuery(&model.Record{UserID: ctx.UserID()}).Count(&rsp.Total); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to count the records", err)
	}
	for _, kind := range []model.RecordType{model.RecordTypeText, model.RecordTypeImage} {
		var total int
		if err := database.Database[*model.Record](ctx).WithQuery(&model.Record{UserID: ctx.UserID(), Type: kind}).Count(&total); err != nil {
			return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to count the records", err)
		}
		rsp.ByType[kind] = total
	}
	return rsp, nil
}
