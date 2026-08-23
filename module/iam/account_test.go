package iam_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/authn"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamaccount "github.com/hydroan/gst/internal/service/iam/account"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/internal/testutil"
	loggerzap "github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/redis"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// Column references for the credential fixture writes in the account tests;
// module test code carries no generated Cols vars.
var (
	colCredentialUserID   = types.NewColumn[string]("user_id")
	colMustChangePassword = types.NewColumn[bool]("must_change_password")
)

func TestAccountSignup(t *testing.T) {
	user := accountSignupUserWithEmail(t, "acct_signup", "12345678", "Acct.Signup@Example.COM")

	require.NotEmpty(t, user.UserID)
	require.NotEmpty(t, user.Username)

	credential := accountRequirePasswordCredential(t, user.UserID)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(credential.PasswordHash), []byte(user.Password)))
	require.False(t, credential.MustChangePassword)

	identity := accountRequireEmailIdentity(t, user.UserID)
	require.Equal(t, "Acct.Signup@Example.COM", identity.Email)
	require.Equal(t, "acct.signup@example.com", identity.NormalizedEmail)
	require.Nil(t, identity.VerifiedAt)
}

func TestAccountLogin(t *testing.T) {
	t.Run("login", func(t *testing.T) {
		user := accountSignupUser(t, "acct_login", "12345678")
		user.SessionID = accountLoginUser(t, &user, user.Password)

		require.NotEmpty(t, user.SessionID)
		accountRequireUserSessionContains(t, user.UserID, user.SessionID)
	})

	t.Run("returns_authenticated_session", func(t *testing.T) {
		user := accountSignupUserWithEmail(t, "acct_login_response", "12345678", "Acct.Login@Example.COM")

		cli := accountNewClient(t)

		rsp, err := cli.Post[iam.LoginRsp](loginPath, iam.LoginReq{
			Username: user.Username,
			Password: user.Password,
		})
		require.NoError(t, err)

		require.False(t, rsp.ServerTime.IsZero())
		require.False(t, rsp.Session.IssuedAt.IsZero())
		require.False(t, rsp.Session.LastSeenAt.IsZero())
		require.False(t, rsp.Session.ExpiresAt.IsZero())
		require.Positive(t, rsp.Session.ExpiresInSeconds)
		require.True(t, rsp.Session.ExpiresAt.After(rsp.ServerTime))
		require.Equal(t, user.UserID, rsp.Principal.UserID)
		require.Equal(t, user.Username, rsp.Principal.Username)
		require.Equal(t, "Acct.Login@Example.COM", rsp.Principal.Email)
		require.False(t, rsp.Principal.MustChangePassword)
	})

	t.Run("sets_session_cookie", func(t *testing.T) {
		user := accountSignupUser(t, "acct_login_cookie", "12345678")
		cookie := accountLoginSessionCookieOverHTTPS(t, user.Username, user.Password)

		require.Equal(t, "session_id", cookie.Name)
		require.NotEmpty(t, cookie.Value)
		require.Equal(t, "/", cookie.Path)
		require.True(t, cookie.HttpOnly)
		require.True(t, cookie.Secure)
		require.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
		require.Positive(t, cookie.MaxAge)
	})

	t.Run("counts_failed_attempts_and_clears_them_on_success", func(t *testing.T) {
		user := accountSignupUser(t, "acct_login_stats", "12345678")
		t.Cleanup(func() {
			serviceiamsession.Store.ClearLoginFailures(context.Background(), user.Username)
		})

		for i := 1; i <= 3; i++ {
			accountRequireLoginRejected(t, user.Username, "wrong-password")
			require.EqualValues(t, i, serviceiamsession.Store.LoginFailures(t.Context(), user.Username))
		}

		sessionID := accountLoginUser(t, &user, user.Password)
		accountRequireUserSessionContains(t, user.UserID, sessionID)

		// Proving the password forgets the attempts that missed it, so a user
		// who eventually gets it right does not stay one attempt from a lockout.
		require.Zero(t, serviceiamsession.Store.LoginFailures(t.Context(), user.Username))
	})

	// The lockout is what makes a password worth guessing at expensive. Without
	// it the only cost of an attempt is the bcrypt comparison, which an attacker
	// is happy to pay a few million times.
	t.Run("locks_out_after_the_configured_number_of_failures", func(t *testing.T) {
		t.Setenv("IAM_LOGIN_FAILURE_LIMIT", "3")
		t.Setenv("IAM_LOGIN_FAILURE_WINDOW", "1m")

		user := accountSignupUser(t, "acct_login_lockout", "12345678")
		t.Cleanup(func() {
			serviceiamsession.Store.ClearLoginFailures(context.Background(), user.Username)
		})

		for range 3 {
			accountRequireLoginRejected(t, user.Username, "wrong-password")
		}

		// The correct password is refused too: the lockout is about the account,
		// not about the credential offered this time.
		accountRequireLoginRejected(t, user.Username, user.Password)

		// Clearing the counter is what a window expiring would do, and the
		// account works again immediately after.
		serviceiamsession.Store.ClearLoginFailures(t.Context(), user.Username)
		require.NotEmpty(t, accountLoginUser(t, &user, user.Password))
	})
}

// accountRequireLoginRejected asserts a login attempt is refused, and refused
// with the one reply every failed attempt gets.
//
// The reply is asserted rather than assumed: a lockout that announced itself
// would let anyone tell an account that exists from one that does not by
// failing five times against a guess.
func accountRequireLoginRejected(t *testing.T, username, password string) {
	t.Helper()

	_, err := accountNewClient(t).Post[iam.LoginRsp](loginPath, iam.LoginReq{
		Username: username,
		Password: password,
	})
	testutil.RequireError(t, err, http.StatusUnauthorized)
}

func TestAccountLogout(t *testing.T) {
	user := accountSignupUser(t, "acct_logout", "12345678")
	user.SessionID = accountLoginUser(t, &user, user.Password)
	accountRequireUserSessionContains(t, user.UserID, user.SessionID)

	t.Run("logout", func(t *testing.T) {
		cli := accountSessionClient(t, user.SessionID)

		rsp, err := cli.Post[iam.LogoutRsp](logoutPath, nil)
		require.NoError(t, err)
		require.NotEmpty(t, rsp.Msg)

		accountRequireSessionNotFound(t, user.SessionID)
		accountRequireUserSessionNotContains(t, user.UserID, user.SessionID)
	})

	t.Run("unauthorized_after_logout", func(t *testing.T) {
		cli := accountSessionClient(t, user.SessionID)

		_, err := cli.Get[iam.CurrentGetRsp](currentPath)
		testutil.RequireError(t, err, http.StatusUnauthorized)
	})

	t.Run("login_again", func(t *testing.T) {
		user.SessionID = accountLoginUser(t, &user, user.Password)
		require.NotEmpty(t, user.SessionID)
	})

	t.Run("returns_error_when_session_index_delete_fails", func(t *testing.T) {
		brokenIndexUser := accountSignupUser(t, "acct_logout_broken_index", "12345678")
		brokenIndexUser.SessionID = accountLoginUser(t, &brokenIndexUser, brokenIndexUser.Password)

		userSessionKey := accountUserSessionIndexKey(t, brokenIndexUser.UserID)
		// This case deliberately corrupts the user session index below. Repair
		// it afterwards: a string left where a zset belongs makes every later
		// read of that key fail, and nothing else clears it.
		t.Cleanup(func() {
			require.NoError(t, redis.Del(context.Background(), userSessionKey))
			require.NoError(t, serviceiamsession.Store.DropSessionIndexes(context.Background(), "", brokenIndexUser.SessionID))
			_, _ = serviceiamsession.Store.DeleteSession(context.Background(), brokenIndexUser.SessionID)
			_ = serviceiamsession.Store.DeleteUserSessions(context.Background(), brokenIndexUser.UserID)
		})

		require.NoError(t, redis.Del(t.Context(), userSessionKey))
		require.NoError(t, redis.Set(t.Context(), userSessionKey, "not-a-zset", time.Hour))

		cli := accountSessionClient(t, brokenIndexUser.SessionID)

		_, err := cli.Post[iam.LogoutRsp](logoutPath, nil)
		testutil.RequireError(t, err, http.StatusInternalServerError, "failed to logout")
	})
}

func TestAccountLoginSecondFactorVerifier(t *testing.T) {
	t.Cleanup(func() { authn.SetLoginSecondFactorVerifier(nil) })

	user := accountSignupUser(t, "acct_login_verifier", "12345678")

	var mu sync.Mutex
	var allow bool
	var gotUserID string
	var gotFactor authn.LoginSecondFactor
	authn.SetLoginSecondFactorVerifier(func(_ *types.ServiceContext, userID string, factor authn.LoginSecondFactor) error {
		mu.Lock()
		defer mu.Unlock()
		gotUserID = userID
		gotFactor = factor
		if allow {
			return nil
		}
		return service.NewError(http.StatusUnauthorized, "second factor required")
	})

	t.Run("verifier_rejection_blocks_login", func(t *testing.T) {
		cli := accountNewClient(t)

		_, err := cli.Post[iam.LoginRsp](loginPath, iam.LoginReq{
			Username: user.Username,
			Password: user.Password,
			TOTPCode: "654321",
		})
		testutil.RequireError(t, err, http.StatusUnauthorized, "second factor required")

		mu.Lock()
		defer mu.Unlock()
		require.Equal(t, user.UserID, gotUserID)
		require.Equal(t, authn.LoginSecondFactor{TOTPCode: "654321"}, gotFactor)
	})

	t.Run("verifier_runs_only_after_first_factor_passed", func(t *testing.T) {
		mu.Lock()
		gotUserID = ""
		mu.Unlock()

		cli := accountNewClient(t)
		_, err := cli.Post[iam.LoginRsp](loginPath, iam.LoginReq{
			Username: user.Username,
			Password: "wrong-password",
		})
		testutil.RequireError(t, err, http.StatusUnauthorized)

		mu.Lock()
		defer mu.Unlock()
		require.Empty(t, gotUserID)
	})

	t.Run("verifier_pass_allows_login", func(t *testing.T) {
		mu.Lock()
		allow = true
		mu.Unlock()

		user.SessionID = accountLoginUser(t, &user, user.Password)
		require.NotEmpty(t, user.SessionID)
	})
}

func TestAccountLoginObservers(t *testing.T) {
	var mu sync.Mutex
	var events []authn.LoginEvent
	remove := authn.AddLoginObserver(func(_ *types.ServiceContext, event authn.LoginEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	})
	t.Cleanup(remove)

	takeEvents := func() []authn.LoginEvent {
		mu.Lock()
		defer mu.Unlock()
		taken := events
		events = nil
		return taken
	}

	user := accountSignupUser(t, "acct_login_observer", "12345678")

	t.Run("successful_login_notifies_succeeded", func(t *testing.T) {
		takeEvents()

		user.SessionID = accountLoginUser(t, &user, user.Password)

		got := takeEvents()
		require.Len(t, got, 1)
		event := got[0]
		require.Equal(t, authn.LoginEventSucceeded, event.Kind)
		require.Equal(t, user.UserID, event.UserID)
		require.Equal(t, user.Username, event.Username)
		require.NotEmpty(t, event.ClientIP)
		require.False(t, event.At.IsZero())
		require.Equal(t, time.UTC, event.At.Location())
	})

	t.Run("failed_password_notifies_failed_with_resolved_user", func(t *testing.T) {
		takeEvents()

		cli := accountNewClient(t)
		_, err := cli.Post[iam.LoginRsp](loginPath, iam.LoginReq{
			Username: user.Username,
			Password: "wrong-password",
		})
		testutil.RequireError(t, err, http.StatusUnauthorized)

		got := takeEvents()
		require.Len(t, got, 1)
		event := got[0]
		require.Equal(t, authn.LoginEventFailed, event.Kind)
		require.Equal(t, user.UserID, event.UserID)
		require.Equal(t, user.Username, event.Username)
	})

	t.Run("unknown_username_notifies_failed_without_user_id", func(t *testing.T) {
		takeEvents()

		cli := accountNewClient(t)
		_, err := cli.Post[iam.LoginRsp](loginPath, iam.LoginReq{
			Username: "acct_login_observer_missing",
			Password: "12345678",
		})
		testutil.RequireError(t, err, http.StatusUnauthorized)

		got := takeEvents()
		require.Len(t, got, 1)
		require.Equal(t, authn.LoginEventFailed, got[0].Kind)
		require.Empty(t, got[0].UserID)
		require.Equal(t, "acct_login_observer_missing", got[0].Username)
	})

	t.Run("logout_notifies_logged_out", func(t *testing.T) {
		user.SessionID = accountLoginUser(t, &user, user.Password)
		takeEvents()

		cli := accountSessionClient(t, user.SessionID)
		_, err := cli.Post[iam.LogoutRsp](logoutPath, nil)
		require.NoError(t, err)

		got := takeEvents()
		require.Len(t, got, 1)
		event := got[0]
		require.Equal(t, authn.LoginEventLoggedOut, event.Kind)
		require.Equal(t, user.UserID, event.UserID)
		require.Equal(t, user.Username, event.Username)
	})
}

func TestAccountChangePassword(t *testing.T) {
	user := accountSignupUser(t, "acct_changepwd", "12345678")
	newPassword := "123456789"
	user.SessionID = accountLoginUser(t, &user, user.Password)
	otherSessionID := accountLoginUser(t, &user, user.Password)
	accountRequireUserSessionContains(t, user.UserID, user.SessionID)
	accountRequireUserSessionContains(t, user.UserID, otherSessionID)

	t.Run("rejects_empty_old_password", func(t *testing.T) {
		invalidUser := accountSignupUser(t, "acct_changepwd_empty_old", "12345678")
		invalidUser.SessionID = accountLoginUser(t, &invalidUser, invalidUser.Password)

		cli := accountSessionClient(t, invalidUser.SessionID)

		_, err := cli.Post[iam.ChangePasswordRsp](changepasswordPath, iam.ChangePasswordReq{
			OldPassword: "",
			NewPassword: newPassword,
		})
		testutil.RequireError(t, err, http.StatusBadRequest, "old password is required")
	})

	t.Run("rejects_empty_new_password", func(t *testing.T) {
		invalidUser := accountSignupUser(t, "acct_changepwd_empty_new", "12345678")
		invalidUser.SessionID = accountLoginUser(t, &invalidUser, invalidUser.Password)

		cli := accountSessionClient(t, invalidUser.SessionID)

		_, err := cli.Post[iam.ChangePasswordRsp](changepasswordPath, iam.ChangePasswordReq{
			OldPassword: invalidUser.Password,
			NewPassword: "",
		})
		testutil.RequireError(t, err, http.StatusBadRequest, "new password is required")
	})

	t.Run("rejects_short_new_password", func(t *testing.T) {
		invalidUser := accountSignupUser(t, "acct_changepwd_short_new", "12345678")
		invalidUser.SessionID = accountLoginUser(t, &invalidUser, invalidUser.Password)

		cli := accountSessionClient(t, invalidUser.SessionID)

		_, err := cli.Post[iam.ChangePasswordRsp](changepasswordPath, iam.ChangePasswordReq{
			OldPassword: invalidUser.Password,
			NewPassword: "12345",
		})
		testutil.RequireError(t, err, http.StatusBadRequest, "password must be at least 6 characters long")
	})

	t.Run("succeeds_when_current_session_snapshot_is_unreadable", func(t *testing.T) {
		syncFailUser := accountSignupUser(t, "acct_changepwd_sync_fail", "12345678")
		syncFailUser.SessionID = accountLoginUser(t, &syncFailUser, syncFailUser.Password)
		syncFailPassword := "syncpass9"

		session, err := serviceiamsession.Store.LoadSession(t.Context(), syncFailUser.SessionID)
		require.NoError(t, err)

		credential := accountRequirePasswordCredential(t, syncFailUser.UserID)
		credential.MustChangePassword = true
		require.NoError(t, database.Database[*modeliamaccount.PasswordCredential](context.Background()).
			WithoutHook().
			WithSelect(colCredentialUserID, colMustChangePassword).
			Update(credential))

		serviceCtx := accountNewServiceContext(
			serviceiamsession.WithCurrentSession(t.Context(), syncFailUser.SessionID, session),
			http.MethodPost,
			"/api/iam/change-password",
			syncFailUser.SessionID,
			consts.PHASE_CREATE,
		)
		require.NoError(t, redis.Set(t.Context(), accountSessionDataKey(t, syncFailUser.SessionID), "not-a-session", time.Hour))

		svc := &serviceiamaccount.ChangePasswordService{}
		svc.Logger = loggerzap.New("")

		resp, err := svc.Create(serviceCtx, &iam.ChangePasswordReq{
			OldPassword: syncFailUser.Password,
			NewPassword: syncFailPassword,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.Msg)

		changed := accountRequirePasswordCredential(t, syncFailUser.UserID)
		require.NoError(t, bcrypt.CompareHashAndPassword([]byte(changed.PasswordHash), []byte(syncFailPassword)))
		require.False(t, changed.MustChangePassword)
	})

	// The password is written before any session is revoked, so a revoke that
	// fails reports a password that did change. The reverse order would report
	// the same failure for a password that had not changed at all, having
	// already logged the user out of the devices it was changing it for.
	t.Run("keeps_new_password_when_revoking_other_sessions_fails", func(t *testing.T) {
		revokeFailUser := accountSignupUser(t, "acct_changepwd_revoke_fail", "12345678")
		revokeFailUser.SessionID = accountLoginUser(t, &revokeFailUser, revokeFailUser.Password)
		revokeFailOtherSessionID := accountLoginUser(t, &revokeFailUser, revokeFailUser.Password)
		revokeFailPassword := "revokepass9"

		session, err := serviceiamsession.Store.LoadSession(t.Context(), revokeFailUser.SessionID)
		require.NoError(t, err)

		// The other session's snapshot is left unreadable, which is what the
		// revoke below trips over. The current session is skipped by the revoke,
		// so it has to be a different one for this to reach the failure at all.
		otherSessionKey := accountSessionDataKey(t, revokeFailOtherSessionID)
		require.NoError(t, redis.Set(t.Context(), otherSessionKey, "not-a-session", time.Hour))
		// Only this user's wreckage is cleared. The subtests around this one
		// share a session of their own, so purging the store would take theirs
		// with it.
		t.Cleanup(func() {
			require.NoError(t, redis.Del(context.Background(), otherSessionKey))
			require.NoError(t, serviceiamsession.Store.DropSessionIndexes(context.Background(), revokeFailUser.UserID, revokeFailOtherSessionID))
			require.NoError(t, serviceiamsession.Store.DeleteUserSessions(context.Background(), revokeFailUser.UserID))
		})

		serviceCtx := accountNewServiceContext(
			serviceiamsession.WithCurrentSession(t.Context(), revokeFailUser.SessionID, session),
			http.MethodPost,
			"/api/iam/change-password",
			revokeFailUser.SessionID,
			consts.PHASE_CREATE,
		)
		svc := &serviceiamaccount.ChangePasswordService{}
		svc.Logger = loggerzap.New("")

		_, err = svc.Create(serviceCtx, &iam.ChangePasswordReq{
			OldPassword: revokeFailUser.Password,
			NewPassword: revokeFailPassword,
		})
		require.Error(t, err)

		var serviceErr *service.Error
		require.True(t, errors.As(err, &serviceErr))
		require.Equal(t, http.StatusInternalServerError, serviceErr.Status())

		changed := accountRequirePasswordCredential(t, revokeFailUser.UserID)
		require.NoError(t, bcrypt.CompareHashAndPassword([]byte(changed.PasswordHash), []byte(revokeFailPassword)))
	})

	t.Run("change_password", func(t *testing.T) {
		cli := accountSessionClient(t, user.SessionID)

		rsp, err := cli.Post[iam.ChangePasswordRsp](changepasswordPath, iam.ChangePasswordReq{
			OldPassword: user.Password,
			NewPassword: newPassword,
		})
		require.NoError(t, err)
		require.NotEmpty(t, rsp.Msg)
	})

	t.Run("keeps_current_session_and_revokes_other_sessions", func(t *testing.T) {
		accountRequireUserSessionContains(t, user.UserID, user.SessionID)
		accountRequireSessionNotFound(t, otherSessionID)
		accountRequireUserSessionNotContains(t, user.UserID, otherSessionID)
	})

	t.Run("login_with_new_password", func(t *testing.T) {
		user.SessionID = accountLoginUser(t, &user, newPassword)
		require.NotEmpty(t, user.SessionID)
	})

	t.Run("user_status_forbidden_without_admin_permission", func(t *testing.T) {
		cli := accountSessionClient(t, user.SessionID)

		_, err := cli.Patch[iam.AdminUserPatchRsp](adminUserPath(user.UserID), iam.AdminUserPatchReq{Status: new(modeliamuser.UserStatusActive)})
		testutil.RequireError(t, err, http.StatusForbidden, "permission denied")
	})
}

func TestAccountResetPassword(t *testing.T) {
	actor := accountSignupUser(t, "acct_reset_actor", "12345678")
	actor.SessionID = accountLoginUser(t, &actor, actor.Password)
	rootSessionID := accountLoginRoot(t)

	victim := accountSignupUser(t, "acct_reset_victim", "87654321")
	resetPass := "resetpass9"
	finalPass := "finalpass9"
	victimSessionBeforeReset := ""
	victimSessionAfterReset := ""

	t.Run("forbidden_without_admin_permission", func(t *testing.T) {
		cli := accountSessionClient(t, actor.SessionID)

		_, err := cli.Post[iam.ResetPasswordRsp](resetpasswordPath, iam.ResetPasswordReq{
			UserID:      victim.UserID,
			NewPassword: resetPass,
		})
		testutil.RequireError(t, err, http.StatusForbidden, "permission denied")
	})

	t.Run("victim_login_before_reset", func(t *testing.T) {
		victimSessionBeforeReset = accountLoginUser(t, &victim, victim.Password)
		require.NotEmpty(t, victimSessionBeforeReset)
		accountRequireUserSessionContains(t, victim.UserID, victimSessionBeforeReset)
	})

	t.Run("rejects_empty_target_user_id", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		_, err := cli.Post[iam.ResetPasswordRsp](resetpasswordPath, iam.ResetPasswordReq{
			UserID:      "",
			NewPassword: resetPass,
		})
		testutil.RequireError(t, err, http.StatusBadRequest, "user_id is required")
	})

	t.Run("rejects_empty_new_password", func(t *testing.T) {
		invalidVictim := accountSignupUser(t, "acct_reset_empty_new", "87654321")

		cli := accountSessionClient(t, rootSessionID)

		_, err := cli.Post[iam.ResetPasswordRsp](resetpasswordPath, iam.ResetPasswordReq{
			UserID:      invalidVictim.UserID,
			NewPassword: "",
		})
		testutil.RequireError(t, err, http.StatusBadRequest, "new password is required")
	})

	t.Run("rejects_short_new_password", func(t *testing.T) {
		invalidVictim := accountSignupUser(t, "acct_reset_short_new", "87654321")

		cli := accountSessionClient(t, rootSessionID)

		_, err := cli.Post[iam.ResetPasswordRsp](resetpasswordPath, iam.ResetPasswordReq{
			UserID:      invalidVictim.UserID,
			NewPassword: "12345",
		})
		testutil.RequireError(t, err, http.StatusBadRequest, "password must be at least 6 characters long")
	})

	t.Run("missing_target_returns_not_found", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		_, err := cli.Post[iam.ResetPasswordRsp](resetpasswordPath, iam.ResetPasswordReq{
			UserID:      "missing-reset-password-target",
			NewPassword: resetPass,
		})
		testutil.RequireError(t, err, http.StatusNotFound, "user not found")
	})

	t.Run("returns_error_when_session_revoke_fails", func(t *testing.T) {
		brokenIndexVictim := accountSignupUser(t, "acct_reset_broken_index", "87654321")
		brokenSessionID := accountLoginUser(t, &brokenIndexVictim, brokenIndexVictim.Password)

		userSessionKey := accountUserSessionIndexKey(t, brokenIndexVictim.UserID)
		// See the note in TestAccountLogout: the corrupted index has to be
		// repaired here, nothing else clears it.
		t.Cleanup(func() {
			require.NoError(t, redis.Del(context.Background(), userSessionKey))
			require.NoError(t, serviceiamsession.Store.DropSessionIndexes(context.Background(), "", brokenSessionID))
			_, _ = serviceiamsession.Store.DeleteSession(context.Background(), brokenSessionID)
			_ = serviceiamsession.Store.DeleteUserSessions(context.Background(), brokenIndexVictim.UserID)
		})

		require.NoError(t, redis.Del(t.Context(), userSessionKey))
		require.NoError(t, redis.Set(t.Context(), userSessionKey, "not-a-zset", time.Hour))

		cli := accountSessionClient(t, rootSessionID)

		_, err := cli.Post[iam.ResetPasswordRsp](resetpasswordPath, iam.ResetPasswordReq{
			UserID:      brokenIndexVictim.UserID,
			NewPassword: resetPass,
		})
		testutil.RequireError(t, err, http.StatusInternalServerError, "failed to revoke user sessions")
	})

	t.Run("reset_success", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		rsp, err := cli.Post[iam.ResetPasswordRsp](resetpasswordPath, iam.ResetPasswordReq{
			UserID:      victim.UserID,
			NewPassword: resetPass,
		})
		require.NoError(t, err)
		require.NotEmpty(t, rsp.Msg)
	})

	t.Run("victim_session_invalid_after_reset", func(t *testing.T) {
		accountRequireSessionNotFound(t, victimSessionBeforeReset)
		accountRequireUserSessionNotContains(t, victim.UserID, victimSessionBeforeReset)

		cli := accountSessionClient(t, victimSessionBeforeReset)

		_, err := cli.Get[iam.CurrentGetRsp](currentPath)
		testutil.RequireError(t, err, http.StatusUnauthorized)
	})

	t.Run("victim_login_after_reset", func(t *testing.T) {
		victimSessionAfterReset = accountLoginUser(t, &victim, resetPass)
		require.NotEmpty(t, victimSessionAfterReset)
	})

	t.Run("must_change_password_blocks_list", func(t *testing.T) {
		cli := accountSessionClient(t, victimSessionAfterReset)

		_, err := cli.Patch[iam.AdminUserPatchRsp](adminUserPath(victim.UserID), iam.AdminUserPatchReq{Status: new(modeliamuser.UserStatusActive)})
		testutil.RequireError(t, err, http.StatusForbidden, "password change required")
	})

	t.Run("victim_change_password", func(t *testing.T) {
		cli := accountSessionClient(t, victimSessionAfterReset)

		rsp, err := cli.Post[iam.ChangePasswordRsp](changepasswordPath, iam.ChangePasswordReq{
			OldPassword: resetPass,
			NewPassword: finalPass,
		})
		require.NoError(t, err)
		require.NotEmpty(t, rsp.Msg)
	})

	t.Run("victim_account_status_forbidden_without_admin_permission_after_change_password", func(t *testing.T) {
		cli := accountSessionClient(t, victimSessionAfterReset)

		_, err := cli.Patch[iam.AdminUserPatchRsp](adminUserPath(victim.UserID), iam.AdminUserPatchReq{Status: new(modeliamuser.UserStatusActive)})
		testutil.RequireError(t, err, http.StatusForbidden, "permission denied")
	})
}

type accountTestUser struct {
	UserID    string
	Username  string
	Password  string
	Email     string
	SessionID string
}

const accountTestUserAgent = "gst-account-test"

func accountSignupUser(t *testing.T, prefix, password string) accountTestUser {
	t.Helper()

	return accountSignupUserWithEmail(t, prefix, password, "")
}

func accountSignupUserWithEmail(t *testing.T, prefix, password, email string) accountTestUser {
	t.Helper()

	user := accountTestUser{
		Username: fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano()),
		Password: password,
		Email:    email,
	}

	cli := accountNewClient(t)

	rsp, err := cli.Post[iam.SignupRsp](signupPath, iam.SignupReq{
		Username:   user.Username,
		Password:   user.Password,
		RePassword: user.Password,
		Email:      user.Email,
	})
	require.NoError(t, err)
	require.Equal(t, user.Username, rsp.Username)
	require.NotEmpty(t, rsp.UserID)
	require.NotEmpty(t, rsp.Message)
	user.UserID = rsp.UserID

	return user
}

func accountRequirePasswordCredential(t *testing.T, userID string) *modeliamaccount.PasswordCredential {
	t.Helper()

	credentials := make([]*modeliamaccount.PasswordCredential, 0, 1)
	require.NoError(t, database.Database[*modeliamaccount.PasswordCredential](context.Background()).
		WithQuery(&modeliamaccount.PasswordCredential{UserID: userID}).
		List(&credentials))
	require.Len(t, credentials, 1)
	return credentials[0]
}

func accountRequireEmailIdentity(t *testing.T, userID string) *modeliamaccount.EmailIdentity {
	t.Helper()

	identities := make([]*modeliamaccount.EmailIdentity, 0, 1)
	require.NoError(t, database.Database[*modeliamaccount.EmailIdentity](context.Background()).
		WithQuery(&modeliamaccount.EmailIdentity{UserID: userID}).
		List(&identities))
	require.Len(t, identities, 1)
	return identities[0]
}

func accountLoginUser(t *testing.T, user *accountTestUser, password string) string {
	t.Helper()

	_, sessionID := accountLoginClient(t, user.Username, password)
	return sessionID
}

func accountLoginRoot(t *testing.T) string {
	t.Helper()

	_, sessionID := accountLoginClient(t, consts.AUTHZ_USER_ROOT, rootPassword)
	t.Cleanup(func() {
		_ = serviceiamsession.Store.DeleteUserSessions(context.Background(), consts.AUTHZ_USER_ROOT)
	})
	return sessionID
}

// accountNewClient returns a service-level client with the test user agent.
func accountNewClient(t *testing.T) *client.Client {
	t.Helper()

	cli, err := client.New(baseURL, client.WithUserAgent(accountTestUserAgent))
	require.NoError(t, err)
	return cli
}

// accountLoginClient logs in and returns the authenticated client (the session
// cookie lives in its jar) together with the session id for storage asserts.
func accountLoginClient(t *testing.T, username, password string) (*client.Client, string) {
	t.Helper()

	cli := accountNewClient(t)
	resp, err := cli.Do(http.MethodPost, loginPath, iam.LoginReq{
		Username: username,
		Password: password,
	})
	require.NoError(t, err)
	cookie := resp.Cookie("session_id")
	require.NotNil(t, cookie, "session cookie not found")
	return cli, cookie.Value
}

// accountLoginSessionCookieOverHTTPS logs in behind a simulated HTTPS proxy
// and returns the raw session cookie for attribute asserts.
func accountLoginSessionCookieOverHTTPS(t *testing.T, username, password string) *http.Cookie {
	t.Helper()

	header := http.Header{}
	header.Set("X-Forwarded-Proto", "https")
	cli, err := client.New(baseURL,
		client.WithUserAgent(accountTestUserAgent), client.WithHeader(header))
	require.NoError(t, err)

	resp, err := cli.Do(http.MethodPost, loginPath, iam.LoginReq{
		Username: username,
		Password: password,
	})
	require.NoError(t, err)

	cookie := resp.Cookie("session_id")
	require.NotNil(t, cookie, "session cookie not found")
	return cookie
}

// accountSessionClient returns a client presenting the given session id, for
// cases that hold a raw session id (revoked sessions, cross-user checks).
func accountSessionClient(t *testing.T, sessionID string) *client.Client {
	t.Helper()

	cli, err := client.New(baseURL,
		client.WithUserAgent(accountTestUserAgent),
		client.WithCookie(&http.Cookie{Name: "session_id", Value: sessionID}))
	require.NoError(t, err)
	return cli
}

func accountNewServiceContext(baseCtx context.Context, method, path, sessionID string, phase consts.Phase) *types.ServiceContext {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(method, path, nil).WithContext(baseCtx)
	req.Header.Set("User-Agent", accountTestUserAgent)
	req.AddCookie(&http.Cookie{
		Name:  serviceiamsession.SessionCookieName,
		Value: sessionID,
	})
	ginCtx.Request = req
	return types.NewServiceContext(ginCtx, nil, phase)
}

func accountRequireSessionNotFound(t *testing.T, sessionID string) {
	t.Helper()

	_, err := serviceiamsession.Store.LoadSession(t.Context(), sessionID)
	require.ErrorIs(t, err, types.ErrEntryNotFound)
}

func accountRequireUserSessionContains(t *testing.T, userID, sessionID string) {
	t.Helper()

	userSessionIDs, err := serviceiamsession.Store.ListUserSessionIDs(t.Context(), userID)
	require.NoError(t, err)
	require.Contains(t, userSessionIDs, sessionID)
}

func accountRequireUserSessionNotContains(t *testing.T, userID, sessionID string) {
	t.Helper()

	userSessionIDs, err := serviceiamsession.Store.ListUserSessionIDs(t.Context(), userID)
	require.NoError(t, err)
	require.NotContains(t, userSessionIDs, sessionID)
}

// The two helpers below spell out Redis keys the store keeps private.
//
// They exist for the cases that plant broken storage — an index that is not a
// sorted set, a snapshot that is not a session — which no method of the store
// can express, because no correct caller would ever ask for it.
//
// The spelling is guarded rather than trusted. The caller has just logged in,
// so the key is known to exist; if the store's layout moves out from under
// these literals the guard fails here, instead of letting a test quietly
// corrupt a key nothing reads and pass for the wrong reason.
func accountUserSessionIndexKey(t *testing.T, userID string) string {
	t.Helper()

	return accountRequireExistingKey(t, "iam:session:index:user:"+userID)
}

func accountSessionDataKey(t *testing.T, sessionID string) string {
	t.Helper()

	return accountRequireExistingKey(t, "iam:session:data:"+sessionID)
}

func accountRequireExistingKey(t *testing.T, key string) string {
	t.Helper()

	ttl, err := redis.TTL(t.Context(), key)
	require.NoError(t, err)
	require.NotEqual(t, redis.TTLKeyNotExists, ttl, "key %q does not exist: the store's key layout changed", key)
	return key
}
