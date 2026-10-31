package item

import (
	"net/http"

	"demo/model"
	"demo/model/record"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	gstmodel "github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
)

// Summary answers GET /api/items/summary: the caller's items counted by the
// type of their record, one query joining the parent, with database.Select
// taking its group key from the joined model.
type Summary struct {
	service.Base[*record.Item, *gstmodel.Empty, *record.ItemSummaryRsp]
}

// List counts the items by record type. The join pins the record by its
// primary key, so no item is counted twice, and the owner condition on the
// record keeps the figures the caller's own; a type the caller has no items
// under is absent rather than zero.
func (s *Summary) List(ctx *gst.ServiceContext, _ *gstmodel.Empty) (*record.ItemSummaryRsp, error) {
	type itemsOfType struct {
		Type  model.RecordType
		Items int
	}
	rows := make([]itemsOfType, 0)
	if err := database.Select[*record.Item, itemsOfType](ctx, model.RecordCols.Type.Group(), gst.Count().As("items")).
		Join(gst.Join[*model.Record](model.RecordCols.ID.EqCol(record.ItemCols.RecordID))).
		Where(model.RecordCols.UserID.Eq(ctx.UserID())).
		Scan(&rows); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to count the items", err)
	}
	rsp := &record.ItemSummaryRsp{ByRecordType: make(map[string]int, len(rows))}
	for _, row := range rows {
		rsp.ByRecordType[string(row.Type)] = row.Items
	}
	return rsp, nil
}
