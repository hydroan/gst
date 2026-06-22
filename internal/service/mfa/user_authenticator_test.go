package servicemfa

import (
	"net/http"
	"testing"

	"github.com/cockroachdb/errors"
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	loggerzap "github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

type stubUserAuthenticator struct {
	authenticateByUsername func(*types.ServiceContext, string, string) (*AuthenticatedUser, error)
	authenticateByUserID   func(*types.ServiceContext, string, string) (*AuthenticatedUser, error)
}

func (s stubUserAuthenticator) AuthenticateByUsername(ctx *types.ServiceContext, username, password string) (*AuthenticatedUser, error) {
	if s.authenticateByUsername == nil {
		return nil, ErrUserAuthenticationFailed
	}
	return s.authenticateByUsername(ctx, username, password)
}

func (s stubUserAuthenticator) AuthenticateByUserID(ctx *types.ServiceContext, userID, password string) (*AuthenticatedUser, error) {
	if s.authenticateByUserID == nil {
		return nil, ErrUserAuthenticationFailed
	}
	return s.authenticateByUserID(ctx, userID, password)
}

func resetUserAuthenticatorAfterTest(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		SetUserAuthenticator(nil)
	})
}

func requireServiceError(t *testing.T, err error, status int, message string) {
	t.Helper()

	var serviceErr *types.ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, status, serviceErr.StatusCode)
	require.Equal(t, message, serviceErr.Message)
}

func TestTOTPCheckCreateReturnsConfigurationErrorWhenUserAuthenticatorMissing(t *testing.T) {
	resetUserAuthenticatorAfterTest(t)
	SetUserAuthenticator(nil)

	svc := &TOTPCheckService{}
	svc.Logger = loggerzap.New("")

	_, err := svc.Create(&types.ServiceContext{}, &modelmfa.TOTPCheckReq{
		Username: "user01",
		Password: "password",
	})

	requireServiceError(t, err, http.StatusInternalServerError, "MFA user authenticator is not configured")
	require.ErrorIs(t, err, ErrUserAuthenticatorNotConfigured)
}

func TestTOTPCheckCreateHidesUserAuthenticationFailure(t *testing.T) {
	resetUserAuthenticatorAfterTest(t)
	SetUserAuthenticator(stubUserAuthenticator{
		authenticateByUsername: func(*types.ServiceContext, string, string) (*AuthenticatedUser, error) {
			return nil, ErrUserAuthenticationFailed
		},
	})

	svc := &TOTPCheckService{}
	svc.Logger = loggerzap.New("")

	_, err := svc.Create(&types.ServiceContext{}, &modelmfa.TOTPCheckReq{
		Username: "user01",
		Password: "wrong-password",
	})

	require.EqualError(t, err, "authentication failed")
}

func TestTOTPCheckCreateReturnsConfigurationErrorForInvalidAuthenticatedUser(t *testing.T) {
	resetUserAuthenticatorAfterTest(t)
	SetUserAuthenticator(stubUserAuthenticator{
		authenticateByUsername: func(*types.ServiceContext, string, string) (*AuthenticatedUser, error) {
			return &AuthenticatedUser{}, nil
		},
	})

	svc := &TOTPCheckService{}
	svc.Logger = loggerzap.New("")

	_, err := svc.Create(&types.ServiceContext{}, &modelmfa.TOTPCheckReq{
		Username: "user01",
		Password: "password",
	})

	requireServiceError(t, err, http.StatusInternalServerError, "MFA user authenticator returned invalid user")
	require.ErrorIs(t, err, ErrUserAuthenticatorInvalidUser)
}

func TestVerifyTOTPUnbindPasswordMapsUserAuthenticatorErrors(t *testing.T) {
	tests := []struct {
		name       string
		auth       UserAuthenticator
		wantErr    error
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "missing authenticator",
			auth:       nil,
			wantErr:    ErrUserAuthenticatorNotConfigured,
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "MFA user authenticator is not configured",
		},
		{
			name: "authentication failed",
			auth: stubUserAuthenticator{
				authenticateByUserID: func(*types.ServiceContext, string, string) (*AuthenticatedUser, error) {
					return nil, ErrUserAuthenticationFailed
				},
			},
			wantErr: errTOTPUnbindVerificationInvalid,
		},
		{
			name: "nil authenticated user",
			auth: stubUserAuthenticator{
				authenticateByUserID: func(*types.ServiceContext, string, string) (*AuthenticatedUser, error) {
					return nilAuthenticatedUser(), nil
				},
			},
			wantErr:    ErrUserAuthenticatorInvalidUser,
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "MFA user authenticator returned invalid user",
		},
		{
			name: "empty authenticated user id",
			auth: stubUserAuthenticator{
				authenticateByUserID: func(*types.ServiceContext, string, string) (*AuthenticatedUser, error) {
					return &AuthenticatedUser{}, nil
				},
			},
			wantErr:    ErrUserAuthenticatorInvalidUser,
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "MFA user authenticator returned invalid user",
		},
		{
			name: "mismatched authenticated user id",
			auth: stubUserAuthenticator{
				authenticateByUserID: func(*types.ServiceContext, string, string) (*AuthenticatedUser, error) {
					return &AuthenticatedUser{ID: "other-user"}, nil
				},
			},
			wantErr:    ErrUserAuthenticatorInvalidUser,
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "MFA user authenticator returned invalid user",
		},
		{
			name: "unknown authenticator error",
			auth: stubUserAuthenticator{
				authenticateByUserID: func(*types.ServiceContext, string, string) (*AuthenticatedUser, error) {
					return nil, errors.New("database unavailable")
				},
			},
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "failed to verify password",
		},
		{
			name: "success",
			auth: stubUserAuthenticator{
				authenticateByUserID: func(*types.ServiceContext, string, string) (*AuthenticatedUser, error) {
					return &AuthenticatedUser{ID: "user-1", Username: "user01"}, nil
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetUserAuthenticatorAfterTest(t)
			SetUserAuthenticator(tt.auth)

			err := verifyTOTPUnbindPassword(&types.ServiceContext{}, "user-1", "password")

			if tt.wantErr == nil && tt.wantStatus == 0 {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			}
			if tt.wantStatus != 0 {
				requireServiceError(t, err, tt.wantStatus, tt.wantMsg)
			}
		})
	}
}

func nilAuthenticatedUser() *AuthenticatedUser {
	return nil
}
