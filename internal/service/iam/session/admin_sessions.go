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
	"github.com/hydroan/gst/types/consts"
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

type adminSessionUserItem struct {
	view       modeliamsession.AdminSessionUserView
	lastActive time.Time
}

// List returns all indexed sessions grouped by user for a privileged administrator.
func (s *AdminSessionsListService) List(ctx *types.ServiceContext, req *modeliamsession.AdminSessionsListReq) (rsp *modeliamsession.AdminSessionsListRsp, err error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	if err = ensureAdminSessionActor(ctx); err != nil {
		log.Error("failed to verify admin session actor", err)
		return nil, err
	}

	sessionIDs, err := listAllSessionIDs()
	if err != nil {
		log.Error("failed to list all sessions", err)
		return nil, err
	}

	users := make(map[string]*adminSessionUserItem, len(sessionIDs))
	var sessionTotal int64
	for i := range sessionIDs {
		sessionID := sessionIDs[i]
		if sessionID == "" {
			continue
		}

		session, getErr := redis.Cache[modeliamsession.Session]().Get(modeliamsession.SessionIDKey(sessionID))
		if getErr != nil {
			if errors.Is(getErr, types.ErrEntryNotFound) {
				_ = redis.ZRem(modeliamsession.SessionAllKey(), sessionID)
				continue
			}
			log.Error("failed to load session from redis", getErr)
			return nil, getErr
		}
		if session.UserID == "" {
			_ = redis.ZRem(modeliamsession.SessionAllKey(), sessionID)
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
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	currentSessionID, _, err := GetCurrentSession(ctx)
	if err != nil {
		log.Error("failed to get current session", err)
		return nil, err
	}
	if err = ensureAdminSessionActor(ctx); err != nil {
		log.Error("failed to verify admin session actor", err)
		return nil, err
	}

	targetSessionID := ctx.Params["id"]
	if targetSessionID == "" {
		return nil, types.NewServiceError(http.StatusBadRequest, "session id is required")
	}

	targetSession, err := redis.Cache[modeliamsession.Session]().Get(modeliamsession.SessionIDKey(targetSessionID))
	if err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			return nil, types.NewServiceError(http.StatusNotFound, "session not found")
		}
		log.Error("failed to load target session", err)
		return nil, err
	}

	return &modeliamsession.AdminSessionsGetRsp{
		Session: buildCurrentSessionView(targetSession, currentSessionID),
	}, nil
}

// Delete invalidates a specified session for a privileged administrator.
func (s *AdminSessionsDeleteService) Delete(ctx *types.ServiceContext, req *modeliamsession.AdminSessionsDeleteReq) (rsp *modeliamsession.AdminSessionsDeleteRsp, err error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	currentSessionID, _, err := GetCurrentSession(ctx)
	if err != nil {
		log.Error("failed to get current session", err)
		return nil, err
	}
	if err = ensureAdminSessionActor(ctx); err != nil {
		log.Error("failed to verify admin session actor", err)
		return nil, err
	}

	targetSessionID := ctx.Params["id"]
	if targetSessionID == "" {
		return nil, types.NewServiceError(http.StatusBadRequest, "session id is required")
	}

	if _, err = redis.Cache[modeliamsession.Session]().Get(modeliamsession.SessionIDKey(targetSessionID)); err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			return nil, types.NewServiceError(http.StatusNotFound, "session not found")
		}
		log.Error("failed to load target session", err)
		return nil, err
	}

	if _, err = DeleteSession(targetSessionID); err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			return nil, types.NewServiceError(http.StatusNotFound, "session not found")
		}
		log.Error("failed to delete target session", err)
		return nil, err
	}
	if targetSessionID == currentSessionID {
		ctx.SetCookie("session_id", "", -1, "/", "", false, true)
	}

	return &modeliamsession.AdminSessionsDeleteRsp{}, nil
}

func ensureAdminSessionActor(ctx *types.ServiceContext) error {
	_, session, err := GetCurrentSession(ctx)
	if err != nil {
		return err
	}

	user := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx.DatabaseContext()).Get(user, session.UserID); err != nil || user.GetID() == "" {
		return types.NewServiceError(http.StatusUnauthorized, "session invalid")
	}

	if session.Username == consts.AUTHZ_USER_ROOT || session.Username == consts.AUTHZ_USER_ADMIN {
		return nil
	}
	if user.IsSuperuser != nil && *user.IsSuperuser {
		return nil
	}

	return types.NewServiceError(http.StatusForbidden, "forbidden")
}

func buildAdminSessionUserItem(ctx *types.ServiceContext, session modeliamsession.Session) (*adminSessionUserItem, error) {
	user := new(modeliamuser.User)
	if err := database.Database[*modeliamuser.User](ctx.DatabaseContext()).Get(user, session.UserID); err == nil && user.GetID() != "" {
		return &adminSessionUserItem{
			view: modeliamsession.AdminSessionUserView{
				UserID:             user.ID,
				Username:           user.Username,
				Email:              util.Deref(user.Email),
				FirstName:          user.FirstName,
				LastName:           user.LastName,
				GroupID:            user.GroupID,
				GroupName:          session.GroupName,
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
			GroupID:            session.GroupID,
			GroupName:          session.GroupName,
			Status:             session.Status,
			MustChangePassword: session.MustChangePassword,
			Sessions:           make([]modeliamsession.SessionView, 0, 1),
		},
	}, nil
}
