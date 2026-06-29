package serviceiamsession

import (
	"net/http"
	"sort"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// AdminUserSessionListService handles retrieval of all sessions owned by a specified user for privileged administrators.
type AdminUserSessionListService struct {
	service.Base[*modeliamsession.AdminUserSession, *modeliamsession.AdminUserSessionListReq, *modeliamsession.AdminUserSessionListRsp]
}

// AdminUserSessionDeleteService handles invalidation of all sessions owned by a specified user for privileged administrators.
type AdminUserSessionDeleteService struct {
	service.Base[*modeliamsession.AdminUserSession, *modeliamsession.AdminUserSessionDeleteReq, *modeliamsession.AdminUserSessionDeleteRsp]
}

// List returns all indexed sessions of a specified user for a privileged administrator.
func (s *AdminUserSessionListService) List(ctx *types.ServiceContext, req *modeliamsession.AdminUserSessionListReq) (rsp *modeliamsession.AdminUserSessionListRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())

	currentSessionID, _, err := SessionManager.Current(ctx)
	if err != nil {
		log.Error("failed to get current session", err)
		return nil, err
	}
	onlineSince, onlineOnly, err := parseAdminSessionOnlineSince(ctx)
	if err != nil {
		return nil, err
	}

	targetUserID := ctx.Param("id")
	if targetUserID == "" {
		return nil, service.NewError(http.StatusBadRequest, "user id is required")
	}

	user := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx).Get(user, targetUserID); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return nil, service.NewError(http.StatusNotFound, "user not found")
		}
		log.Error("failed to load target user", err)
		return nil, err
	}
	if err = ensureAdminSessionTarget(ctx, user); err != nil {
		log.Error("failed to verify admin session target", err)
		return nil, err
	}

	view, err := s.buildView(ctx, user, currentSessionID, onlineSince, onlineOnly)
	if err != nil {
		log.Error("failed to build target user sessions view", err)
		return nil, err
	}

	return &modeliamsession.AdminUserSessionListRsp{
		User: view,
	}, nil
}

// Delete invalidates all indexed sessions of a specified user for a privileged administrator.
func (s *AdminUserSessionDeleteService) Delete(ctx *types.ServiceContext, req *modeliamsession.AdminUserSessionDeleteReq) (rsp *modeliamsession.AdminUserSessionDeleteRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())

	_, currentSession, err := SessionManager.Current(ctx)
	if err != nil {
		log.Error("failed to get current session", err)
		return nil, err
	}

	targetUserID := ctx.Param("id")
	if targetUserID == "" {
		return nil, service.NewError(http.StatusBadRequest, "user id is required")
	}

	targetUser := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx).Get(targetUser, targetUserID); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return nil, service.NewError(http.StatusNotFound, "user not found")
		}
		log.Error("failed to load target user", err)
		return nil, err
	}
	if err = ensureAdminSessionTarget(ctx, targetUser); err != nil {
		log.Error("failed to verify admin session target", err)
		return nil, err
	}

	if err = DeleteUserSessions(ctx, targetUserID); err != nil {
		log.Error("failed to delete target user sessions", err)
		return nil, err
	}
	if currentSession.UserID == targetUserID {
		SessionManager.ClearCookie(ctx)
	}

	return &modeliamsession.AdminUserSessionDeleteRsp{}, nil
}

// buildView builds a target user's session view for admin APIs.
//
// Without online filtering it reads the user's own session index. With
// online_within it reads the global last-seen candidate index first, then
// filters by user after loading each session snapshot. That keeps the online
// path bounded by recently active sessions instead of scanning every session
// owned by the target user.
func (s *AdminUserSessionListService) buildView(ctx *types.ServiceContext, user *modeliamuser.User, currentSessionID string, onlineSince time.Time, onlineOnly bool) (modeliamsession.AdminSessionOwnerView, error) {
	credential, err := loadSessionPasswordCredential(ctx, user.ID)
	if err != nil {
		return modeliamsession.AdminSessionOwnerView{}, err
	}
	email, err := loadSessionEmail(ctx, user.ID)
	if err != nil {
		return modeliamsession.AdminSessionOwnerView{}, err
	}

	view := modeliamsession.AdminSessionOwnerView{
		UserID:             user.ID,
		Username:           user.Username,
		Email:              email,
		Status:             string(user.Status),
		MustChangePassword: credential.MustChangePassword,
		Sessions:           make([]modeliamsession.SessionView, 0),
	}

	var indexUserID string
	var sessionIDs []string
	if onlineOnly {
		sessionIDs, err = listOnlineSessionIDs(ctx, onlineSince)
	} else {
		indexUserID = user.ID
		sessionIDs, err = listUserSessionIDs(ctx, user.ID)
	}
	if err != nil {
		return modeliamsession.AdminSessionOwnerView{}, err
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
				removeStaleSessionIndexes(ctx, indexUserID, sessionID)
				continue
			}
			return modeliamsession.AdminSessionOwnerView{}, getErr
		}
		if validateErr := SessionManager.Validate(sessionID, session); validateErr != nil {
			_, _ = SessionManager.Delete(ctx, sessionID)
			continue
		}
		if session.UserID != user.ID {
			if indexUserID != "" {
				_ = redis.ZRem(ctx, modeliamsession.SessionUserKey(indexUserID), sessionID)
			}
			continue
		}
		if onlineOnly && !sessionSeenSince(session, onlineSince) {
			continue
		}

		view.Sessions = append(view.Sessions, buildSessionView(session, currentSessionID))
	}

	sort.Slice(view.Sessions, func(i, j int) bool {
		left := sessionViewActiveAt(view.Sessions[i])
		right := sessionViewActiveAt(view.Sessions[j])
		if left.Equal(right) {
			return view.Sessions[i].ID > view.Sessions[j].ID
		}
		return left.After(right)
	})

	return view, nil
}
