package serviceiamsession

import (
	"net/http"
	"sort"
	"time"

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

// AdminSessionsListService handles retrieval of all sessions grouped by user for privileged administrators.
type AdminSessionsListService struct {
	service.Base[*model.Empty, *modeliamsession.AdminSessionsListReq, *modeliamsession.AdminSessionsListRsp]
}

// AdminSessionsGetService handles retrieval of a specified session for privileged administrators.
type AdminSessionsGetService struct {
	service.Base[*model.Empty, *modeliamsession.AdminSessionsGetReq, *modeliamsession.AdminSessionsGetRsp]
}

// AdminSessionsDeleteService handles invalidation of a specified session for privileged administrators.
type AdminSessionsDeleteService struct {
	service.Base[*model.Empty, *modeliamsession.AdminSessionsDeleteReq, *modeliamsession.AdminSessionsDeleteRsp]
}

// AdminUserSessionsListService handles retrieval of all sessions owned by a specified user for privileged administrators.
type AdminUserSessionsListService struct {
	service.Base[*model.Empty, *modeliamsession.AdminUserSessionsListReq, *modeliamsession.AdminUserSessionsListRsp]
}

// AdminUserSessionsDeleteService handles invalidation of all sessions owned by a specified user for privileged administrators.
type AdminUserSessionsDeleteService struct {
	service.Base[*model.Empty, *modeliamsession.AdminUserSessionsDeleteReq, *modeliamsession.AdminUserSessionsDeleteRsp]
}

type adminSessionUserItem struct {
	view       modeliamsession.AdminSessionUserView
	lastActive time.Time
}

// List returns all indexed sessions grouped by user for a privileged administrator.
func (s *AdminSessionsListService) List(ctx *types.ServiceContext, req *modeliamsession.AdminSessionsListReq) (rsp *modeliamsession.AdminSessionsListRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())

	if err = ensureAdminSessionActor(ctx); err != nil {
		log.Error("failed to verify admin session actor", err)
		return nil, err
	}

	sessionIDs, err := listAllSessionIDs(ctx)
	if err != nil {
		log.Error("failed to list all sessions", err)
		return nil, err
	}

	cache := redis.Cache[modeliamsession.Session]().WithContext(ctx)
	users := make(map[string]*adminSessionUserItem, len(sessionIDs))
	var sessionTotal int64
	for i := range sessionIDs {
		sessionID := sessionIDs[i]
		if sessionID == "" {
			continue
		}

		session, getErr := cache.Get(modeliamsession.SessionIDKey(sessionID))
		if getErr != nil {
			if errors.Is(getErr, types.ErrEntryNotFound) {
				_ = redis.ZRem(ctx, modeliamsession.SessionAllKey(), sessionID)
				continue
			}
			log.Error("failed to load session from redis", getErr)
			return nil, getErr
		}
		if validateErr := ValidateActiveSession(sessionID, session); validateErr != nil {
			_, _ = DeleteSession(ctx, sessionID)
			continue
		}

		item, exists := users[session.UserID]
		if !exists {
			item, err = buildAdminSessionUserItem(ctx, session)
			if err != nil {
				log.Error("failed to build admin session user view", err)
				return nil, err
			}
			users[session.UserID] = item
		}

		view := buildCurrentSessionView(session, "")
		item.view.Sessions = append(item.view.Sessions, view)
		item.view.SessionTotal++
		sessionTotal++

		activeAt := view.LastSeenAt
		if activeAt.IsZero() {
			activeAt = view.IssuedAt
		}
		if item.lastActive.IsZero() || activeAt.After(item.lastActive) {
			item.lastActive = activeAt
		}
	}

	items := make([]adminSessionUserItem, 0, len(users))
	for _, item := range users {
		sort.Slice(item.view.Sessions, func(i, j int) bool {
			left := item.view.Sessions[i].LastSeenAt
			if left.IsZero() {
				left = item.view.Sessions[i].IssuedAt
			}
			right := item.view.Sessions[j].LastSeenAt
			if right.IsZero() {
				right = item.view.Sessions[j].IssuedAt
			}
			if left.Equal(right) {
				return item.view.Sessions[i].ID > item.view.Sessions[j].ID
			}
			return left.After(right)
		})
		items = append(items, *item)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].lastActive.Equal(items[j].lastActive) {
			return items[i].view.UserID > items[j].view.UserID
		}
		return items[i].lastActive.After(items[j].lastActive)
	})

	rspItems := make([]modeliamsession.AdminSessionUserView, 0, len(items))
	for i := range items {
		rspItems = append(rspItems, items[i].view)
	}

	return &modeliamsession.AdminSessionsListRsp{
		Items:        rspItems,
		Total:        int64(len(rspItems)),
		SessionTotal: sessionTotal,
	}, nil
}

// Get returns the detail of a specified session for a privileged administrator.
func (s *AdminSessionsGetService) Get(ctx *types.ServiceContext, req *modeliamsession.AdminSessionsGetReq) (rsp *modeliamsession.AdminSessionsGetRsp, err error) {
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
	if err = ValidateActiveSession(targetSessionID, targetSession); err != nil {
		_, _ = DeleteSession(ctx, targetSessionID)
		return nil, service.NewError(http.StatusNotFound, "session not found")
	}

	return &modeliamsession.AdminSessionsGetRsp{
		Session: buildCurrentSessionView(targetSession, currentSessionID),
	}, nil
}

// Delete invalidates a specified session for a privileged administrator.
func (s *AdminSessionsDeleteService) Delete(ctx *types.ServiceContext, req *modeliamsession.AdminSessionsDeleteReq) (rsp *modeliamsession.AdminSessionsDeleteRsp, err error) {
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
	if err = ValidateActiveSession(targetSessionID, targetSession); err != nil {
		_, _ = DeleteSession(ctx, targetSessionID)
		return nil, service.NewError(http.StatusNotFound, "session not found")
	}

	if _, err = DeleteSession(ctx, targetSessionID); err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			return nil, service.NewError(http.StatusNotFound, "session not found")
		}
		log.Error("failed to delete target session", err)
		return nil, err
	}
	if targetSessionID == currentSessionID {
		ClearSessionCookie(ctx)
	}

	return &modeliamsession.AdminSessionsDeleteRsp{}, nil
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
		if errors.Is(err, database.ErrRecordNotFound) {
			return nil, service.NewError(http.StatusNotFound, "user not found")
		}
		log.Error("failed to load target user", err)
		return nil, err
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
		if errors.Is(err, database.ErrRecordNotFound) {
			return nil, service.NewError(http.StatusNotFound, "user not found")
		}
		log.Error("failed to load target user", err)
		return nil, err
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

func buildAdminSessionUserItem(ctx *types.ServiceContext, session modeliamsession.Session) (*adminSessionUserItem, error) {
	user := new(modeliamuser.User)
	if err := database.Database[*modeliamuser.User](ctx).Get(user, session.UserID); err == nil {
		return &adminSessionUserItem{
			view: modeliamsession.AdminSessionUserView{
				UserID:             user.ID,
				Username:           user.Username,
				Email:              util.Deref(user.Email),
				FirstName:          user.FirstName,
				LastName:           user.LastName,
				Status:             string(user.Status),
				MustChangePassword: user.MustChangePassword,
				Sessions:           make([]modeliamsession.SessionView, 0, 1),
			},
		}, nil
	}

	return &adminSessionUserItem{
		view: modeliamsession.AdminSessionUserView{
			UserID:             session.UserID,
			Username:           session.Username,
			Email:              session.Email,
			FirstName:          session.FirstName,
			LastName:           session.LastName,
			Status:             session.Status,
			MustChangePassword: session.MustChangePassword,
			Sessions:           make([]modeliamsession.SessionView, 0, 1),
		},
	}, nil
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
