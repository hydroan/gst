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

type adminSessionOwnerItem struct {
	view       modeliamsession.AdminSessionOwnerView
	lastActive time.Time
}

// List returns all indexed sessions grouped by user for a privileged administrator.
func (s *AdminSessionsListService) List(ctx *types.ServiceContext, req *modeliamsession.AdminSessionsListReq) (rsp *modeliamsession.AdminSessionsListRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())

	if err = ensureAdminSessionActor(ctx); err != nil {
		log.Error("failed to verify admin session actor", err)
		return nil, err
	}

	onlineSince, onlineOnly, err := parseAdminSessionsOnlineSince(ctx)
	if err != nil {
		return nil, err
	}

	var sessionIDs []string
	if onlineOnly {
		sessionIDs, err = listOnlineSessionIDs(ctx, onlineSince)
	} else {
		sessionIDs, err = listAllSessionIDs(ctx)
	}
	if err != nil {
		log.Error("failed to list all sessions", err)
		return nil, err
	}

	cache := redis.Cache[modeliamsession.Session]().WithContext(ctx)
	owners := make(map[string]*adminSessionOwnerItem, len(sessionIDs))
	var sessionTotal int64
	for i := range sessionIDs {
		sessionID := sessionIDs[i]
		if sessionID == "" {
			continue
		}

		session, getErr := cache.Get(modeliamsession.SessionIDKey(sessionID))
		if getErr != nil {
			if errors.Is(getErr, types.ErrEntryNotFound) {
				removeStaleSessionIndexes(ctx, "", sessionID)
				continue
			}
			log.Error("failed to load session from redis", getErr)
			return nil, getErr
		}
		if validateErr := SessionManager.Validate(sessionID, session); validateErr != nil {
			_, _ = SessionManager.Delete(ctx, sessionID)
			continue
		}
		if onlineOnly && !sessionSeenSince(session, onlineSince) {
			continue
		}

		item, exists := owners[session.UserID]
		if !exists {
			var ok bool
			item, ok, err = s.buildItem(ctx, session)
			if err != nil {
				log.Error("failed to build admin session owner view", err)
				return nil, err
			}
			if !ok {
				continue
			}
			owners[session.UserID] = item
		}

		view := buildSessionView(session, "")
		item.view.Sessions = append(item.view.Sessions, view)
		sessionTotal++

		activeAt := sessionViewActiveAt(view)
		if item.lastActive.IsZero() || activeAt.After(item.lastActive) {
			item.lastActive = activeAt
		}
	}

	items := make([]adminSessionOwnerItem, 0, len(owners))
	for _, item := range owners {
		sort.Slice(item.view.Sessions, func(i, j int) bool {
			left := sessionViewActiveAt(item.view.Sessions[i])
			right := sessionViewActiveAt(item.view.Sessions[j])
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

	rspItems := make([]modeliamsession.AdminSessionOwnerView, 0, len(items))
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

	return &modeliamsession.AdminSessionsGetRsp{
		Session: buildSessionView(targetSession, currentSessionID),
	}, nil
}

// Delete invalidates a specified session for a privileged administrator.
func (s *AdminSessionsDeleteService) Delete(ctx *types.ServiceContext, req *modeliamsession.AdminSessionsDeleteReq) (rsp *modeliamsession.AdminSessionsDeleteRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())

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

	return &modeliamsession.AdminSessionsDeleteRsp{}, nil
}

func (s *AdminSessionsListService) buildItem(ctx *types.ServiceContext, session modeliamsession.Session) (*adminSessionOwnerItem, bool, error) {
	user := new(modeliamuser.User)
	if err := database.Database[*modeliamuser.User](ctx).Get(user, session.UserID); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			_, _ = SessionManager.Delete(ctx, session.ID)
			return nil, false, nil
		}
		return nil, false, err
	}
	credential, err := loadSessionPasswordCredential(ctx, user.ID)
	if err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			_, _ = SessionManager.Delete(ctx, session.ID)
			return nil, false, nil
		}
		return nil, false, err
	}

	return &adminSessionOwnerItem{
		view: modeliamsession.AdminSessionOwnerView{
			UserID:             user.ID,
			Username:           user.Username,
			Email:              util.Deref(user.Email),
			FirstName:          user.FirstName,
			LastName:           user.LastName,
			Status:             string(user.Status),
			MustChangePassword: credential.MustChangePassword,
			Sessions:           make([]modeliamsession.SessionView, 0, 1),
		},
	}, true, nil
}
