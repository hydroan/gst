package serviceiamuser

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/types"
)

// buildAdminUserView builds the public admin representation for one IAM user.
//
// It delegates to the bulk builder so Get and List expose the same fields and
// sensitive credential details stay centralized in one projection path.
func buildAdminUserView(ctx context.Context, u *modeliamuser.User) (modeliamuser.AdminUserView, error) {
	views, err := buildAdminUserViews(ctx, []*modeliamuser.User{u})
	if err != nil {
		return modeliamuser.AdminUserView{}, err
	}
	if len(views) == 0 {
		return modeliamuser.AdminUserView{}, nil
	}
	return views[0], nil
}

// buildAdminUserViews projects user rows into admin API response items.
//
// User stores only identity-neutral account state. Email and password metadata
// live in IAM account tables, so this function batch-loads them by user ID and
// attaches only response-safe fields such as email and must_change_password.
func buildAdminUserViews(ctx context.Context, users []*modeliamuser.User) ([]modeliamuser.AdminUserView, error) {
	userIDs := make([]string, 0, len(users))
	for _, user := range users {
		if user == nil || user.ID == "" {
			continue
		}
		userIDs = append(userIDs, user.ID)
	}

	emails, err := loadAdminUserEmailMap(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	credentials, err := loadAdminUserCredentialMap(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	views := make([]modeliamuser.AdminUserView, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		view := modeliamuser.AdminUserView{
			ID:        u.ID,
			Username:  u.Username,
			Status:    u.Status,
			CreatedAt: u.CreatedAt,
			UpdatedAt: u.UpdatedAt,
		}
		if email, ok := emails[u.ID]; ok {
			view.Email = email.Email
		}
		if credential, ok := credentials[u.ID]; ok {
			view.MustChangePassword = credential.MustChangePassword
		}
		views = append(views, view)
	}
	return views, nil
}

// loadAdminUserEmailMap returns email identities keyed by user ID.
//
// Missing email rows are valid for users that were created without an email, so
// an empty result is treated as an empty map rather than an error.
func loadAdminUserEmailMap(ctx context.Context, userIDs []string) (map[string]*modeliamaccount.EmailIdentity, error) {
	items := make(map[string]*modeliamaccount.EmailIdentity, len(userIDs))
	if len(userIDs) == 0 {
		return items, nil
	}

	identities := make([]*modeliamaccount.EmailIdentity, 0, len(userIDs))
	if err := database.Database[*modeliamaccount.EmailIdentity](ctx).
		WithQuery(nil, types.QueryConfig{RawQuery: "user_id IN ?", RawQueryArgs: []any{userIDs}}).
		List(&identities); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return items, nil
		}
		return nil, err
	}
	for _, identity := range identities {
		if identity != nil && identity.UserID != "" {
			items[identity.UserID] = identity
		}
	}
	return items, nil
}

// loadAdminUserCredentialMap returns password credential metadata keyed by user ID.
//
// The admin user view needs the must_change_password flag only. Password hashes
// and other credential internals remain inside the account model and are never
// copied into AdminUserView.
func loadAdminUserCredentialMap(ctx context.Context, userIDs []string) (map[string]*modeliamaccount.PasswordCredential, error) {
	items := make(map[string]*modeliamaccount.PasswordCredential, len(userIDs))
	if len(userIDs) == 0 {
		return items, nil
	}

	credentials := make([]*modeliamaccount.PasswordCredential, 0, len(userIDs))
	if err := database.Database[*modeliamaccount.PasswordCredential](ctx).
		WithQuery(nil, types.QueryConfig{RawQuery: "user_id IN ?", RawQueryArgs: []any{userIDs}}).
		List(&credentials); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return items, nil
		}
		return nil, err
	}
	for _, credential := range credentials {
		if credential != nil && credential.UserID != "" {
			items[credential.UserID] = credential
		}
	}
	return items, nil
}
