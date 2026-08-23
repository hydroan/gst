package serviceiamsession

import (
	"time"

	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
)

// BuildAuthenticatedSessionRsp builds the shared login/current response contract.
//
// systemRoot is taken as an argument rather than resolved here. This builder
// runs once the response is already decided — for login, after the session is
// stored and the cookie is written — so an authorization lookup failing at this
// point would fail a request that has otherwise succeeded. Each caller resolves
// it while it can still refuse.
func BuildAuthenticatedSessionRsp(
	sessionData modeliamsession.Session,
	user *modeliamuser.User,
	email string,
	now time.Time,
	systemRoot bool,
) *modeliamsession.AuthenticatedSessionRsp {
	if now.IsZero() {
		now = time.Now()
	}
	return &modeliamsession.AuthenticatedSessionRsp{
		ServerTime: now,
		Session:    buildAuthenticatedSessionView(sessionData, now),
		Principal:  buildPrincipalView(user, email, sessionData.MustChangePassword, systemRoot),
	}
}

// buildAuthenticatedSessionView builds a session timing view without exposing the bearer session id.
func buildAuthenticatedSessionView(sessionData modeliamsession.Session, now time.Time) modeliamsession.AuthenticatedSessionView {
	if now.IsZero() {
		now = time.Now()
	}
	var expiresIn int64
	if !sessionData.ExpiresAt.IsZero() {
		expiresIn = int64(sessionData.ExpiresAt.Sub(now).Seconds())
	}
	if expiresIn < 0 {
		expiresIn = 0
	}
	return modeliamsession.AuthenticatedSessionView{
		TenantID:         sessionData.TenantID,
		IssuedAt:         sessionData.IssuedAt,
		LastSeenAt:       sessionData.LastSeenAt,
		ExpiresAt:        sessionData.ExpiresAt,
		ExpiresInSeconds: expiresIn,
	}
}

// buildPrincipalView builds the principal snapshot returned by authentication
// state APIs.
//
// user is required. Both callers have already read and checked the record this
// view describes, and a zero view would report the principal as holding no
// system role — an answer no client can tell apart from a resolved one.
func buildPrincipalView(user *modeliamuser.User, email string, mustChangePassword bool, systemRoot bool) modeliamsession.PrincipalView {
	return modeliamsession.PrincipalView{
		UserID:             user.ID,
		Username:           user.Username,
		Email:              email,
		MustChangePassword: mustChangePassword,
		IsSystemRoot:       systemRoot,
	}
}
