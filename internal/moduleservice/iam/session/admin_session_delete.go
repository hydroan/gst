package serviceiamsession

import (
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	modeliamsession "github.com/hydroan/gst/internal/modulemodel/iam/session"
	"github.com/hydroan/gst/service"
)

// AdminSessionDeleteService handles invalidation of a specified session for privileged administrators.
type AdminSessionDeleteService struct {
	service.Base[*modeliamsession.AdminSession, *modeliamsession.AdminSessionDeleteReq, *modeliamsession.AdminSessionDeleteRsp]
}

// Delete invalidates a specified session for a privileged administrator.
func (a *AdminSessionDeleteService) Delete(ctx *gst.ServiceContext, req *modeliamsession.AdminSessionDeleteReq) (rsp *modeliamsession.AdminSessionDeleteRsp, err error) {
	currentSessionID, _, err := CurrentSession(ctx)
	if err != nil {
		return nil, err
	}
	if err = ensureAdminSessionActor(ctx); err != nil {
		return nil, err
	}

	targetSessionID := ctx.Param("id")
	if targetSessionID == "" {
		return nil, gst.NewError(http.StatusBadRequest, sessionIDRequiredMsg)
	}

	targetSession, err := Store.LoadSession(ctx, targetSessionID)
	if err != nil {
		if errors.Is(err, gst.ErrEntryNotFound) {
			return nil, gst.NewError(http.StatusNotFound, sessionNotFoundMsg)
		}
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, failedToLoadTargetSessionMsg, err)
	}
	if err = ValidateSession(targetSessionID, targetSession); err != nil {
		_, _ = Store.DeleteSession(ctx, targetSessionID)
		return nil, gst.NewError(http.StatusNotFound, sessionNotFoundMsg)
	}

	if _, err = Store.DeleteSession(ctx, targetSessionID); err != nil {
		if errors.Is(err, gst.ErrEntryNotFound) {
			return nil, gst.NewError(http.StatusNotFound, sessionNotFoundMsg)
		}
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, failedToDeleteSessionMsg, err)
	}
	if targetSessionID == currentSessionID {
		ClearCookie(ctx)
	}

	return &modeliamsession.AdminSessionDeleteRsp{}, nil
}
