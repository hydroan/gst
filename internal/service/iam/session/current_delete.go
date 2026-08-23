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
	sessionID, err := CookieSessionID(ctx)
	if err != nil {
		return nil, err
	}

	if _, err = Store.DeleteSession(ctx, sessionID); err != nil {
		return nil, service.NewErrorWithCause(http.StatusUnauthorized, "session not exists", err)
	}

	ClearCookie(ctx)

	return &modeliamsession.CurrentDeleteRsp{}, nil
}
