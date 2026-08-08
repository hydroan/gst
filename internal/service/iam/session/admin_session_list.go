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
	"github.com/hydroan/gst/redis"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// AdminSessionListService handles retrieval of all sessions grouped by user for privileged administrators.
type AdminSessionListService struct {
	service.Base[*modeliamsession.AdminSession, *model.Empty, *modeliamsession.AdminSessionListRsp]
}

type adminSessionOwnerItem struct {
	view       modeliamsession.AdminSessionOwnerView
	lastActive time.Time
}

// List returns all indexed sessions grouped by user for a privileged administrator.
func (a *AdminSessionListService) List(ctx *types.ServiceContext, req *model.Empty) (rsp *modeliamsession.AdminSessionListRsp, err error) {
	if err = ensureAdminSessionActor(ctx); err != nil {
		return nil, err
	}

	onlineSince, onlineOnly, err := parseAdminSessionOnlineSince(ctx)
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
		return nil, err
	}

	cache := redis.Cache[modeliamsession.Session]().WithContext(ctx)
	owners := make(map[string]*adminSessionOwnerItem, len(sessionIDs))
	var sessionTotal int
	for i := range sessionIDs {
		sessionID := sessionIDs[i]
		if sessionID == "" {
			continue
		}

		sessionData, getErr := cache.Get(modeliamsession.SessionIDKey(sessionID))
		if getErr != nil {
			if errors.Is(getErr, types.ErrEntryNotFound) {
				_ = modeliamsession.RemoveSessionIndexes(ctx, "", sessionID)
				continue
			}
			return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load session", getErr)
		}
		if validateErr := SessionManager.Validate(sessionID, sessionData); validateErr != nil {
			_, _ = SessionManager.Delete(ctx, sessionID)
			continue
		}
		if onlineOnly && !sessionSeenSince(sessionData, onlineSince) {
			continue
		}

		item, exists := owners[sessionData.UserID]
		if !exists {
			var ok bool
			item, ok, err = a.buildItem(ctx, sessionData)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			owners[sessionData.UserID] = item
		}

		view := buildSessionView(sessionData, "")
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

	return &modeliamsession.AdminSessionListRsp{
		Items:        rspItems,
		Total:        len(rspItems),
		SessionTotal: sessionTotal,
	}, nil
}

func (a *AdminSessionListService) buildItem(ctx *types.ServiceContext, sourceSession modeliamsession.Session) (*adminSessionOwnerItem, bool, error) {
	targetUser := new(modeliamuser.User)
	if err := database.Database[*modeliamuser.User](ctx).Get(targetUser, sourceSession.UserID); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			_, _ = SessionManager.Delete(ctx, sourceSession.ID)
			return nil, false, nil
		}
		return nil, false, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load session owner", err)
	}
	credential, err := loadSessionPasswordCredential(ctx, targetUser.ID)
	if err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			_, _ = SessionManager.Delete(ctx, sourceSession.ID)
			return nil, false, nil
		}
		return nil, false, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load session owner", err)
	}
	email, err := loadSessionEmail(ctx, targetUser.ID)
	if err != nil {
		return nil, false, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load session owner", err)
	}

	return &adminSessionOwnerItem{
		view: modeliamsession.AdminSessionOwnerView{
			UserID:             targetUser.ID,
			Username:           targetUser.Username,
			Email:              email,
			Status:             string(targetUser.Status),
			MustChangePassword: credential.MustChangePassword,
			Sessions:           make([]modeliamsession.SessionView, 0, 1),
		},
	}, true, nil
}
