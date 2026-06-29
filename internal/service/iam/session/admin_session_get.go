package serviceiamsession

import (
	"net/http"

	"github.com/cockroachdb/errors"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// AdminSessionGetService handles retrieval of a specified session for privileged administrators.
type AdminSessionGetService struct {
	service.Base[*modeliamsession.AdminSessionGet, *modeliamsession.AdminSessionGetReq, *modeliamsession.AdminSessionGetRsp]
}

// Get returns the detail of a specified session for a privileged administrator.
func (a *AdminSessionGetService) Get(ctx *types.ServiceContext, req *modeliamsession.AdminSessionGetReq) (rsp *modeliamsession.AdminSessionGetRsp, err error) {
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

	return &modeliamsession.AdminSessionGetRsp{
		Session: buildSessionView(targetSession, currentSessionID),
	}, nil
}
