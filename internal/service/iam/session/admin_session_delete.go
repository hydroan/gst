package serviceiamsession

import (
	"net/http"

	"github.com/cockroachdb/errors"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// AdminSessionDeleteService handles invalidation of a specified session for privileged administrators.
type AdminSessionDeleteService struct {
	service.Base[*modeliamsession.AdminSession, *modeliamsession.AdminSessionDeleteReq, *modeliamsession.AdminSessionDeleteRsp]
}

// Delete invalidates a specified session for a privileged administrator.
func (a *AdminSessionDeleteService) Delete(ctx *types.ServiceContext, req *modeliamsession.AdminSessionDeleteReq) (rsp *modeliamsession.AdminSessionDeleteRsp, err error) {
	currentSessionID, _, err := CurrentSession(ctx)
	if err != nil {
		return nil, err
	}
	if err = ensureAdminSessionActor(ctx); err != nil {
		return nil, err
	}

	targetSessionID := ctx.Param("id")
	if targetSessionID == "" {
		return nil, service.NewError(http.StatusBadRequest, "session id is required")
	}

	targetSession, err := Store.LoadSession(ctx, targetSessionID)
	if err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			return nil, service.NewError(http.StatusNotFound, "session not found")
		}
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load target session", err)
	}
	if err = ValidateSession(targetSessionID, targetSession); err != nil {
		_, _ = Store.DeleteSession(ctx, targetSessionID)
		return nil, service.NewError(http.StatusNotFound, "session not found")
	}

	if _, err = Store.DeleteSession(ctx, targetSessionID); err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			return nil, service.NewError(http.StatusNotFound, "session not found")
		}
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to delete session", err)
	}
	if targetSessionID == currentSessionID {
		ClearCookie(ctx)
	}

	return &modeliamsession.AdminSessionDeleteRsp{}, nil
}
