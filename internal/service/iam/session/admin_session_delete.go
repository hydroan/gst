package serviceiamsession

import (
	"net/http"

	"github.com/cockroachdb/errors"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// AdminSessionDeleteService handles invalidation of a specified session for privileged administrators.
type AdminSessionDeleteService struct {
	service.Base[*modeliamsession.AdminSession, *modeliamsession.AdminSessionDeleteReq, *modeliamsession.AdminSessionDeleteRsp]
}

// Delete invalidates a specified session for a privileged administrator.
func (a *AdminSessionDeleteService) Delete(ctx *types.ServiceContext, req *modeliamsession.AdminSessionDeleteReq) (rsp *modeliamsession.AdminSessionDeleteRsp, err error) {
	log := a.WithContext(ctx, ctx.Phase())

	currentSessionID, _, err := SessionManager.Current(ctx)
	if err != nil {
		log.Error("failed to get current session", err)
		return nil, err
	}
	if err = ensureAdminSessionActor(ctx); err != nil {
		log.Error("failed to verify admin session actor", err)
		return nil, err
	}

	targetSessionID := ctx.Param("id")
	if targetSessionID == "" {
		return nil, service.NewError(http.StatusBadRequest, "session id is required")
	}

	targetSession, err := redis.Cache[modeliamsession.Session]().WithContext(ctx).Get(modeliamsession.SessionIDKey(targetSessionID))
	if err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			return nil, service.NewError(http.StatusNotFound, "session not found")
		}
		log.Error("failed to load target session", err)
		return nil, err
	}
	if err = SessionManager.Validate(targetSessionID, targetSession); err != nil {
		_, _ = SessionManager.Delete(ctx, targetSessionID)
		return nil, service.NewError(http.StatusNotFound, "session not found")
	}

	if _, err = SessionManager.Delete(ctx, targetSessionID); err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			return nil, service.NewError(http.StatusNotFound, "session not found")
		}
		log.Error("failed to delete target session", err)
		return nil, err
	}
	if targetSessionID == currentSessionID {
		SessionManager.ClearCookie(ctx)
	}

	return &modeliamsession.AdminSessionDeleteRsp{}, nil
}
