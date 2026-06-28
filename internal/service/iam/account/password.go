package serviceiamaccount

import (
	"github.com/cockroachdb/errors"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
)

const minAccountPasswordLength = 6

func validateChangePasswordInput(req *modeliamaccount.ChangePasswordReq) error {
	if req == nil {
		return errors.New("change password request is required")
	}
	if req.OldPassword == "" {
		return errors.New("old password is required")
	}
	return validateNewAccountPassword(req.NewPassword)
}

func validateResetPasswordInput(req *modeliamaccount.ResetPasswordReq) error {
	if req == nil {
		return errors.New("reset password request is required")
	}
	if req.UserID == "" {
		return errors.New("user_id is required")
	}
	return validateNewAccountPassword(req.NewPassword)
}

func validateNewAccountPassword(password string) error {
	if password == "" {
		return errors.New("new password is required")
	}
	if len(password) < minAccountPasswordLength {
		return errors.New("password must be at least 6 characters long")
	}
	return nil
}
