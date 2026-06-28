package serviceiamsession

import (
	"time"

	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
)

// BuildAuthenticatedSessionRsp builds the shared login/current response contract.
func BuildAuthenticatedSessionRsp(session modeliamsession.Session, user *modeliamuser.User, email string, now time.Time) *modeliamsession.AuthenticatedSessionRsp {
	if now.IsZero() {
		now = time.Now()
	}
	return &modeliamsession.AuthenticatedSessionRsp{
		ServerTime: now,
		Session:    buildAuthenticatedSessionView(session, now),
		Principal:  buildPrincipalView(user, email, session.MustChangePassword),
	}
}

// buildAuthenticatedSessionView builds a session timing view without exposing the bearer session id.
func buildAuthenticatedSessionView(session modeliamsession.Session, now time.Time) modeliamsession.AuthenticatedSessionView {
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

// buildPrincipalView builds the principal snapshot returned by authentication state APIs.
func buildPrincipalView(user *modeliamuser.User, email string, mustChangePassword bool) modeliamsession.PrincipalView {
	if user == nil {
		return modeliamsession.PrincipalView{}
	}
	return modeliamsession.PrincipalView{
		UserID:             user.ID,
		Username:           user.Username,
		Email:              email,
		FirstName:          user.FirstName,
		LastName:           user.LastName,
		MustChangePassword: mustChangePassword,
	}
}
