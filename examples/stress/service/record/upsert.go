package record

import (
	"net/http"

	"stress/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/service"
)

// Upsert serves POST /api/records/upsert: the record is inserted, or
// overwritten where its code is taken. The row is named by its code,
// recordID, so the merge meets the primary key, which every dialect merges
// on.
type Upsert struct {
	service.Base[*model.Record, *model.RecordUpsertReq, *model.Record]
}

func (u *Upsert) Create(ctx *gst.ServiceContext, req *model.RecordUpsertReq) (*model.Record, error) {
	rec := &model.Record{Code: req.Code, Title: req.Title, Category: req.Category, Status: req.Status, Score: req.Score, Body: req.Body}
	rec.ID = recordID(req.Code)
	if err := database.Database[*model.Record](ctx).Upsert(rec); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "upsert record failed", err)
	}
	return rec, nil
}

// recordID is the id of the record carrying code: "rec-" and the code. The
// rows the scenarios seed carry the same id, so an upsert meets them.
func recordID(code string) string { return "rec-" + code }
