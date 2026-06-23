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

type stubAccountAuthenticator struct {
	authenticateByUsername  func(*types.ServiceContext, string, string) (*AuthenticatedAccount, error)
	authenticateByAccountID func(*types.ServiceContext, string, string) (*AuthenticatedAccount, error)
}

func (s stubAccountAuthenticator) AuthenticateByUsername(ctx *types.ServiceContext, username, password string) (*AuthenticatedAccount, error) {
	if s.authenticateByUsername == nil {
		return nil, ErrAccountAuthenticationFailed
	}
	return s.authenticateByUsername(ctx, username, password)
}

func (s stubAccountAuthenticator) AuthenticateByAccountID(ctx *types.ServiceContext, accountID, password string) (*AuthenticatedAccount, error) {
	if s.authenticateByAccountID == nil {
		return nil, ErrAccountAuthenticationFailed
	}
	return s.authenticateByAccountID(ctx, accountID, password)
}

func resetAccountAuthenticatorAfterTest(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		SetAccountAuthenticator(nil)
	})
}

func requireServiceError(t *testing.T, err error, status int, message string) {
	t.Helper()

	var serviceErr *types.ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, status, serviceErr.StatusCode)
	require.Equal(t, message, serviceErr.Message)
}

func TestTOTPCheckCreateReturnsConfigurationErrorWhenAccountAuthenticatorMissing(t *testing.T) {
	resetAccountAuthenticatorAfterTest(t)
	SetAccountAuthenticator(nil)

	svc := &TOTPCheckService{}
	svc.Logger = loggerzap.New("")

	_, err := svc.Create(&types.ServiceContext{}, &modelmfa.TOTPCheckReq{
		Username: "user01",
		Password: "password",
	})

	requireServiceError(t, err, http.StatusInternalServerError, "MFA account authenticator is not configured")
	require.ErrorIs(t, err, ErrAccountAuthenticatorNotConfigured)
}

func TestTOTPCheckCreateHidesAccountAuthenticationFailure(t *testing.T) {
	resetAccountAuthenticatorAfterTest(t)
	SetAccountAuthenticator(stubAccountAuthenticator{
		authenticateByUsername: func(*types.ServiceContext, string, string) (*AuthenticatedAccount, error) {
			return nil, ErrAccountAuthenticationFailed
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

func TestTOTPCheckCreateReturnsConfigurationErrorForInvalidAuthenticatedAccount(t *testing.T) {
	resetAccountAuthenticatorAfterTest(t)
	SetAccountAuthenticator(stubAccountAuthenticator{
		authenticateByUsername: func(*types.ServiceContext, string, string) (*AuthenticatedAccount, error) {
			return &AuthenticatedAccount{}, nil
		},
	})

	svc := &TOTPCheckService{}
	svc.Logger = loggerzap.New("")

	_, err := svc.Create(&types.ServiceContext{}, &modelmfa.TOTPCheckReq{
		Username: "user01",
		Password: "password",
	})

	requireServiceError(t, err, http.StatusInternalServerError, "MFA account authenticator returned invalid account")
	require.ErrorIs(t, err, ErrAccountAuthenticatorInvalidAccount)
}

func TestVerifyTOTPUnbindPasswordMapsAccountAuthenticatorErrors(t *testing.T) {
	tests := []struct {
		name       string
		auth       AccountAuthenticator
		wantErr    error
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "missing authenticator",
			auth:       nil,
			wantErr:    ErrAccountAuthenticatorNotConfigured,
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "MFA account authenticator is not configured",
		},
		{
			name: "authentication failed",
			auth: stubAccountAuthenticator{
				authenticateByAccountID: func(*types.ServiceContext, string, string) (*AuthenticatedAccount, error) {
					return nil, ErrAccountAuthenticationFailed
				},
			},
			wantErr: errTOTPUnbindVerificationInvalid,
		},
		{
			name: "nil authenticated account",
			auth: stubAccountAuthenticator{
				authenticateByAccountID: func(*types.ServiceContext, string, string) (*AuthenticatedAccount, error) {
					return nilAuthenticatedAccount(), nil
				},
			},
			wantErr:    ErrAccountAuthenticatorInvalidAccount,
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "MFA account authenticator returned invalid account",
		},
		{
			name: "empty authenticated account id",
			auth: stubAccountAuthenticator{
				authenticateByAccountID: func(*types.ServiceContext, string, string) (*AuthenticatedAccount, error) {
					return &AuthenticatedAccount{}, nil
				},
			},
			wantErr:    ErrAccountAuthenticatorInvalidAccount,
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "MFA account authenticator returned invalid account",
		},
		{
			name: "mismatched authenticated account id",
			auth: stubAccountAuthenticator{
				authenticateByAccountID: func(*types.ServiceContext, string, string) (*AuthenticatedAccount, error) {
					return &AuthenticatedAccount{ID: "other-user"}, nil
				},
			},
			wantErr:    ErrAccountAuthenticatorInvalidAccount,
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "MFA account authenticator returned invalid account",
		},
		{
			name: "unknown authenticator error",
			auth: stubAccountAuthenticator{
				authenticateByAccountID: func(*types.ServiceContext, string, string) (*AuthenticatedAccount, error) {
					return nil, errors.New("database unavailable")
				},
			},
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "failed to verify password",
		},
		{
			name: "success",
			auth: stubAccountAuthenticator{
				authenticateByAccountID: func(*types.ServiceContext, string, string) (*AuthenticatedAccount, error) {
					return &AuthenticatedAccount{ID: "user-1", Username: "user01"}, nil
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetAccountAuthenticatorAfterTest(t)
			SetAccountAuthenticator(tt.auth)

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

func nilAuthenticatedAccount() *AuthenticatedAccount {
	return nil
}
