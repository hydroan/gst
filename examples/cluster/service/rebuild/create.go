package rebuild

import (
	"net/http"

	"cluster/dao"
	"cluster/helper"
	"cluster/model"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/lock"
	"github.com/hydroan/gst/service"
)

type Creator struct {
	service.Base[*model.Rebuild, *model.RebuildReq, *model.RebuildRsp]
}

// Create runs the rebuild under the lock. A second request while one runs —
// on this replica or on another — is answered 409 at once, never queued; a
// rebuild cut short because the lease behind the lock was lost is reported
// as a failure even though the work itself may have finished.
func (c *Creator) Create(ctx *gst.ServiceContext, req *model.RebuildReq) (*model.RebuildRsp, error) {
	seconds := max(req.Seconds, 1)
	err := dao.Rebuild(ctx, seconds, req.InTransaction)
	switch {
	case errors.Is(err, lock.ErrInTransaction):
		return nil, service.NewError(http.StatusBadRequest, "a lock cannot be taken inside a transaction")
	case errors.Is(err, lock.ErrHeld):
		return nil, service.NewError(http.StatusConflict, "a rebuild is already running")
	case err != nil:
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "the rebuild did not finish", err)
	}
	return &model.RebuildRsp{Replica: helper.Replica(), Seconds: seconds}, nil
}
