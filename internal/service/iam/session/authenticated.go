package serviceiamsession

import (
	"time"

	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
)

// BuildAuthenticatedSessionRsp builds the shared login/current response contract.
func BuildAuthenticatedSessionRsp(sessionData modeliamsession.Session, user *modeliamuser.User, email string, now time.Time) *modeliamsession.AuthenticatedSessionRsp {
	if now.IsZero() {
		now = time.Now()
	}
	return &modeliamsession.AuthenticatedSessionRsp{
		ServerTime: now,
		Session:    buildAuthenticatedSessionView(sessionData, now),
		Principal:  buildPrincipalView(user, email, sessionData.MustChangePassword),
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
		Status:           sessionData.Status,
		IssuedAt:         sessionData.IssuedAt,
		LastSeenAt:       sessionData.LastSeenAt,
		ExpiresAt:        sessionData.ExpiresAt,
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
		MustChangePassword: mustChangePassword,
	}
}
