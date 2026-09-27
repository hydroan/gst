package record

import (
	"net/http"

	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/service"
)

// Creator hooks the framework's own Create: the model, request and response
// types are the same, so the framework reads the body, writes the row and
// answers it, and calls the hooks around that.
type Creator struct {
	service.Base[*model.Record, *model.Record, *model.Record]
}

// CreateBefore stamps the owner and the default type: a record belongs to
// whoever creates it, whatever the body says, and the session names them.
func (r *Creator) CreateBefore(ctx *gst.ServiceContext, record *model.Record) error {
	record.UserID = ctx.UserID()
	if record.Type == "" {
		record.Type = model.RecordTypeText
	}
	return nil
}

// CreateAfter writes the audit row of the creation through the database
// chain, on the request's context. An error a service returns is a
// service.Error: it names the status the client gets and the message, and
// carries the cause for the log.
func (r *Creator) CreateAfter(ctx *gst.ServiceContext, record *model.Record) error {
	audit := &model.Audit{Action: "create", RecordID: record.ID, Actor: ctx.Username()}
	if err := database.Database[*model.Audit](ctx).Create(audit); err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to write the audit row", err)
	}
	return nil
}
