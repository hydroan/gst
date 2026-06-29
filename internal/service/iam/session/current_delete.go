package serviceiamsession

import (
	"net/http"

	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// CurrentDeleteService handles invalidation of the current authenticated session.
type CurrentDeleteService struct {
	service.Base[*modeliamsession.Current, *modeliamsession.CurrentDeleteReq, *modeliamsession.CurrentDeleteRsp]
}

// Delete invalidates the current authenticated session and clears the session cookie.
func (c *CurrentDeleteService) Delete(ctx *types.ServiceContext, req *modeliamsession.CurrentDeleteReq) (rsp *modeliamsession.CurrentDeleteRsp, err error) {
	log := c.WithContext(ctx, ctx.Phase())

	sessionID, err := SessionManager.SessionID(ctx)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	if _, err = SessionManager.Delete(ctx, sessionID); err != nil {
		log.Error("failed to delete current session", err)
		return nil, service.NewErrorWithCause(http.StatusUnauthorized, "session not exists", err)
	}

	SessionManager.ClearCookie(ctx)

	return &modeliamsession.CurrentDeleteRsp{}, nil
}
