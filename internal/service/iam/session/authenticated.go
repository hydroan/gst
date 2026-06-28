package serviceiamsession

import (
	"time"

	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/util"
)

// BuildAuthenticatedSessionRsp builds the shared login/current response contract.
func BuildAuthenticatedSessionRsp(session modeliamsession.Session, user *modeliamuser.User, now time.Time) *modeliamsession.AuthenticatedSessionRsp {
	if now.IsZero() {
		now = time.Now()
	}
	return &modeliamsession.AuthenticatedSessionRsp{
		ServerTime: now,
		Session:    BuildAuthenticatedSessionView(session, now),
		Principal:  BuildPrincipalView(user),
	}
}

// BuildAuthenticatedSessionView builds a session timing view without exposing the bearer session id.
func BuildAuthenticatedSessionView(session modeliamsession.Session, now time.Time) modeliamsession.AuthenticatedSessionView {
	if now.IsZero() {
		now = time.Now()
	}
	var expiresIn int64
	if !session.ExpiresAt.IsZero() {
		expiresIn = int64(session.ExpiresAt.Sub(now).Seconds())
	}
	if expiresIn < 0 {
		expiresIn = 0
	}
	return modeliamsession.AuthenticatedSessionView{
		Status:           session.Status,
		IssuedAt:         session.IssuedAt,
		LastSeenAt:       session.LastSeenAt,
		ExpiresAt:        session.ExpiresAt,
		ExpiresInSeconds: expiresIn,
	}
}

// BuildPrincipalView builds the principal snapshot returned by authentication state APIs.
func BuildPrincipalView(user *modeliamuser.User) modeliamsession.PrincipalView {
	if user == nil {
		return modeliamsession.PrincipalView{}
	}
	return modeliamsession.PrincipalView{
		UserID:             user.ID,
		Username:           user.Username,
		Email:              util.Deref(user.Email),
		FirstName:          user.FirstName,
		LastName:           user.LastName,
		MustChangePassword: user.MustChangePassword,
	}
}
