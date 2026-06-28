package serviceiamaccount

import (
	"testing"

	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	"github.com/stretchr/testify/require"
)

func TestValidateChangePasswordInput(t *testing.T) {
	tests := []struct {
		name    string
		req     *modeliamaccount.ChangePasswordReq
		wantErr string
	}{
		{
			name: "rejects_empty_old_password",
			req: &modeliamaccount.ChangePasswordReq{
				NewPassword: "12345678",
			},
			wantErr: "old password is required",
		},
		{
			name: "rejects_empty_new_password",
			req: &modeliamaccount.ChangePasswordReq{
				OldPassword: "12345678",
			},
			wantErr: "new password is required",
		},
		{
			name: "rejects_short_new_password",
			req: &modeliamaccount.ChangePasswordReq{
				OldPassword: "12345678",
				NewPassword: "12345",
			},
			wantErr: "password must be at least 6 characters long",
		},
		{
			name: "accepts_valid_passwords",
			req: &modeliamaccount.ChangePasswordReq{
				OldPassword: "12345678",
				NewPassword: "87654321",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateChangePasswordInput(tt.req)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateResetPasswordInput(t *testing.T) {
	tests := []struct {
		name    string
		req     *modeliamaccount.ResetPasswordReq
		wantErr string
	}{
		{
			name: "rejects_empty_user_id",
			req: &modeliamaccount.ResetPasswordReq{
				NewPassword: "12345678",
			},
			wantErr: "user_id is required",
		},
		{
			name: "rejects_empty_new_password",
			req: &modeliamaccount.ResetPasswordReq{
				UserID: "target-user-id",
			},
			wantErr: "new password is required",
		},
		{
			name: "rejects_short_new_password",
			req: &modeliamaccount.ResetPasswordReq{
				UserID:      "target-user-id",
				NewPassword: "12345",
			},
			wantErr: "password must be at least 6 characters long",
		},
		{
			name: "accepts_valid_password",
			req: &modeliamaccount.ResetPasswordReq{
				UserID:      "target-user-id",
				NewPassword: "87654321",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResetPasswordInput(tt.req)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
