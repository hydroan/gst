package serviceiamsession

import (
	"net/http"
	"time"

	"github.com/hydroan/gst/database"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// CurrentGetService handles retrieval of the current authenticated session.
type CurrentGetService struct {
	service.Base[*modeliamsession.Current, *modeliamsession.CurrentGetReq, *modeliamsession.CurrentGetRsp]
}

// Get returns the current authenticated session together with the latest user snapshot.
func (c *CurrentGetService) Get(ctx *types.ServiceContext, req *modeliamsession.CurrentGetReq) (rsp *modeliamsession.CurrentGetRsp, err error) {
	log := c.WithContext(ctx, ctx.Phase())

	_, currentSession, err := SessionManager.Current(ctx)
	if err != nil {
		log.Error("failed to get current session", err)
		return nil, err
	}

	currentUser := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx).Get(currentUser, currentSession.UserID); err != nil {
		log.Error("failed to load user for current session")
		return nil, service.NewError(http.StatusUnauthorized, "session invalid")
	}
	if err = ensureSessionUserActive(currentUser); err != nil {
		return nil, err
	}
	email, err := loadSessionEmail(ctx, currentUser.ID)
	if err != nil {
		log.Error("failed to load email identity for current session")
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load email identity", err)
	}

	return BuildAuthenticatedSessionRsp(currentSession, currentUser, email, time.Now()), nil
}
