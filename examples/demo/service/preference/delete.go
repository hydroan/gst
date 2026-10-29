package preference

import (
	"net/http"

	"demo/model"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/service"
)

// Deleter hooks the framework's own Delete: the hook keeps the delete to
// the caller's own preferences.
type Deleter struct {
	service.Base[*model.Preference, *model.Preference, *model.Preference]
}

// DeleteBefore reads the stored preference the id names and refuses one of
// someone else's as not there, 404, the way a missing one is answered.
func (p *Deleter) DeleteBefore(ctx *gst.ServiceContext, preference *model.Preference) error {
	stored := new(model.Preference)
	if err := database.Database[*model.Preference](ctx).Get(stored, preference.ID); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return gst.NewErrorWithCause(http.StatusNotFound, "preference not found", err)
		}
		return gst.NewErrorWithCause(http.StatusInternalServerError, "failed to load the preference", err)
	}
	if stored.UserID != ctx.UserID() {
		return gst.NewError(http.StatusNotFound, "preference not found")
	}
	return nil
}
