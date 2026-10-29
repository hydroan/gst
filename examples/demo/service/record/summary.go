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

// List counts the caller's records by type in one query: database.Select
// projects the group key, the type, and a count under the alias the result
// row names, and the framework derives the GROUP BY from the key. The
// owner condition is the service's to write: a projection runs no service
// hook, so nothing narrows it to the caller but this Where.
func (s *Summary) List(ctx *gst.ServiceContext, _ *gstmodel.Empty) (*model.RecordSummaryRsp, error) {
	type recordsOfType struct {
		Type  model.RecordType
		Total int
	}
	rows := make([]recordsOfType, 0)
	if err := database.Select[*model.Record, recordsOfType](ctx, model.RecordCols.Type.Group(), gst.Count().As("total")).
		Where(model.RecordCols.UserID.Eq(ctx.UserID())).
		Scan(&rows); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to count the records", err)
	}
	rsp := &model.RecordSummaryRsp{ByType: make(map[model.RecordType]int, len(rows))}
	for _, row := range rows {
		rsp.ByType[row.Type] = row.Total
		rsp.Total += row.Total
	}
	return rsp, nil
}
