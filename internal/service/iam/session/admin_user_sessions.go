package serviceiamsession

import (
	"net/http"
	"sort"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/util"
)

// AdminUserSessionsListService handles retrieval of all sessions owned by a specified user for privileged administrators.
type AdminUserSessionsListService struct {
	service.Base[*model.Empty, *modeliamsession.AdminUserSessionsListReq, *modeliamsession.AdminUserSessionsListRsp]
}

// AdminUserSessionsDeleteService handles invalidation of all sessions owned by a specified user for privileged administrators.
type AdminUserSessionsDeleteService struct {
	service.Base[*model.Empty, *modeliamsession.AdminUserSessionsDeleteReq, *modeliamsession.AdminUserSessionsDeleteRsp]
}

// List returns all indexed sessions of a specified user for a privileged administrator.
func (s *AdminUserSessionsListService) List(ctx *types.ServiceContext, req *modeliamsession.AdminUserSessionsListReq) (rsp *modeliamsession.AdminUserSessionsListRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())

	currentSessionID, _, err := GetCurrentSession(ctx)
	if err != nil {
		log.Error("failed to get current session", err)
		return nil, err
	}
	if err = ensureAdminSessionActor(ctx); err != nil {
		log.Error("failed to verify admin session actor", err)
		return nil, err
	}

	targetUserID := ctx.Param("id")
	if targetUserID == "" {
		return nil, service.NewError(http.StatusBadRequest, "user id is required")
	}

	user := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx).Get(user, targetUserID); err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			return nil, service.NewError(http.StatusNotFound, "user not found")
		}
		log.Error("failed to load target user", err)
		return nil, err
	}
	if user.GetID() == "" {
		return nil, service.NewError(http.StatusNotFound, "user not found")
	}

	view, err := buildAdminUserSessionsView(ctx, user, currentSessionID)
	if err != nil {
		log.Error("failed to build target user sessions view", err)
		return nil, err
	}

	return &modeliamsession.AdminUserSessionsListRsp{
		User: view,
	}, nil
}

// Delete invalidates all indexed sessions of a specified user for a privileged administrator.
func (s *AdminUserSessionsDeleteService) Delete(ctx *types.ServiceContext, req *modeliamsession.AdminUserSessionsDeleteReq) (rsp *modeliamsession.AdminUserSessionsDeleteRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())

	_, currentSession, err := GetCurrentSession(ctx)
	if err != nil {
		log.Error("failed to get current session", err)
		return nil, err
	}
	if err = ensureAdminSessionActor(ctx); err != nil {
		log.Error("failed to verify admin session actor", err)
		return nil, err
	}

	targetUserID := ctx.Param("id")
	if targetUserID == "" {
		return nil, service.NewError(http.StatusBadRequest, "user id is required")
	}

	targetUser := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx).Get(targetUser, targetUserID); err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			return nil, service.NewError(http.StatusNotFound, "user not found")
		}
		log.Error("failed to load target user", err)
		return nil, err
	}
	if targetUser.GetID() == "" {
		return nil, service.NewError(http.StatusNotFound, "user not found")
	}

	if err = DeleteAllSessions(ctx, targetUserID); err != nil {
		log.Error("failed to delete target user sessions", err)
		return nil, err
	}
	if currentSession.UserID == targetUserID {
		ClearSessionCookie(ctx)
	}

	return &modeliamsession.AdminUserSessionsDeleteRsp{}, nil
}

func buildAdminUserSessionsView(ctx *types.ServiceContext, user *modeliamuser.User, currentSessionID string) (modeliamsession.AdminSessionUserView, error) {
	view := modeliamsession.AdminSessionUserView{
		UserID:             user.ID,
		Username:           user.Username,
		Email:              util.Deref(user.Email),
		FirstName:          user.FirstName,
		LastName:           user.LastName,
		Status:             string(user.Status),
		MustChangePassword: user.MustChangePassword,
		Sessions:           make([]modeliamsession.SessionView, 0),
	}

	sessionIDs, err := listUserSessionIDs(ctx, user.ID)
	if err != nil {
		return modeliamsession.AdminSessionUserView{}, err
	}

	cache := redis.Cache[modeliamsession.Session]().WithContext(ctx)
	for i := range sessionIDs {
		sessionID := sessionIDs[i]
		if sessionID == "" {
			continue
		}

		session, getErr := cache.Get(modeliamsession.SessionIDKey(sessionID))
		if getErr != nil {
			if errors.Is(getErr, types.ErrEntryNotFound) {
				_ = redis.ZRem(ctx, modeliamsession.SessionUserKey(user.ID), sessionID)
				_ = redis.ZRem(ctx, modeliamsession.SessionAllKey(), sessionID)
				continue
			}
			return modeliamsession.AdminSessionUserView{}, getErr
		}
		if validateErr := ValidateActiveSession(sessionID, session); validateErr != nil {
			_, _ = DeleteSession(ctx, sessionID)
			continue
		}
		if session.UserID != user.ID {
			_ = redis.ZRem(ctx, modeliamsession.SessionUserKey(user.ID), sessionID)
			continue
		}

		view.Sessions = append(view.Sessions, buildCurrentSessionView(session, currentSessionID))
	}

	sort.Slice(view.Sessions, func(i, j int) bool {
		left := view.Sessions[i].LastSeenAt
		if left.IsZero() {
			left = view.Sessions[i].IssuedAt
		}
		right := view.Sessions[j].LastSeenAt
		if right.IsZero() {
			right = view.Sessions[j].IssuedAt
		}
		if left.Equal(right) {
			return view.Sessions[i].ID > view.Sessions[j].ID
		}
		return left.After(right)
	})

	view.SessionTotal = int64(len(view.Sessions))

	return view, nil
}
