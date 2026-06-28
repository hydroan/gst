package serviceiamsession

import (
	"net/http"
	"strings"
	"time"

	"github.com/hydroan/gst/database"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/internal/service/iam/adminauth"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

const adminSessionsOnlineWithinQuery = "online_within"

// ensureAdminSessionActor verifies that the current session belongs to the built-in root user.
func ensureAdminSessionActor(ctx *types.ServiceContext) error {
	user, err := loadAdminSessionActor(ctx)
	if err != nil {
		return err
	}
	if user.GetID() == consts.AUTHZ_USER_ROOT {
		return nil
	}
	return service.NewError(http.StatusForbidden, "forbidden")
}

func ensureAdminSessionTarget(ctx *types.ServiceContext, target *modeliamuser.User) error {
	actor, err := loadAdminSessionActor(ctx)
	if err != nil {
		return err
	}
	return adminauth.EnsureTenantAdmin(ctx, actor, target)
}

func loadAdminSessionActor(ctx *types.ServiceContext) (*modeliamuser.User, error) {
	_, session, err := SessionManager.Current(ctx)
	if err != nil {
		return nil, err
	}

	user := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx).Get(user, session.UserID); err != nil {
		return nil, service.NewError(http.StatusUnauthorized, "session invalid")
	}
	if err = ensureSessionUserActive(user); err != nil {
		return nil, err
	}

	return user, nil
}

// parseAdminSessionsOnlineSince parses the admin-only online session window.
//
// The public contract is a Go duration in the online_within query parameter,
// for example "5m". A missing value means the caller wants the normal full
// session list instead of an online-only view.
func parseAdminSessionsOnlineSince(ctx *types.ServiceContext) (time.Time, bool, error) {
	raw := ""
	if ctx != nil {
		raw = strings.TrimSpace(ctx.Query().Get(adminSessionsOnlineWithinQuery))
	}
	if raw == "" {
		return time.Time{}, false, nil
	}

	onlineWithin, err := time.ParseDuration(raw)
	if err != nil || onlineWithin <= 0 {
		return time.Time{}, false, service.NewError(http.StatusBadRequest, "online_within must be a positive duration")
	}
	return time.Now().Add(-onlineWithin), true, nil
}

// sessionSeenSince verifies the loaded session snapshot still matches the online window.
//
// The Redis last-seen ZSET is only a candidate index. This snapshot check keeps
// the response correct if the index score and session payload drift apart.
func sessionSeenSince(session modeliamsession.Session, since time.Time) bool {
	return !session.LastSeenAt.IsZero() && !session.LastSeenAt.Before(since)
}
