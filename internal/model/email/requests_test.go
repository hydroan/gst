package modelemail_test

import (
	"testing"

	"github.com/gin-gonic/gin/binding"
	modelemail "github.com/hydroan/gst/internal/model/email"
	"github.com/stretchr/testify/require"
)

// TestRequestsDeclareTheirRulesUnderBinding pins that the payloads of the
// email flows carry their validation rules under the binding tag, the one
// the framework validates a request by: a payload holding a value its rules
// refuse is refused by the validator the framework runs, and one meeting
// them passes.
func TestRequestsDeclareTheirRulesUnderBinding(t *testing.T) {
	for name, payload := range map[string]any{
		"change cancel without a token":                &modelemail.ChangeCancelReq{},
		"change confirm without a token":               &modelemail.ChangeConfirmReq{},
		"change request without a new email":           &modelemail.ChangeRequestReq{CurrentPassword: "current-pass9"},
		"change request with no email address":         &modelemail.ChangeRequestReq{NewEmail: "not-an-address", CurrentPassword: "current-pass9"},
		"change request without the current password":  &modelemail.ChangeRequestReq{NewEmail: "new@sample.test"},
		"change resend with no email address":          &modelemail.ChangeResendReq{NewEmail: "not-an-address"},
		"password reset confirm without a token":       &modelemail.PasswordResetConfirmReq{NewPassword: "created-pass9"},
		"password reset confirm with a short password": &modelemail.PasswordResetConfirmReq{Token: "sample-token", NewPassword: "short"},
		"password reset request with no email address": &modelemail.PasswordResetRequestReq{Email: "not-an-address"},
		"verification confirm without a token":         &modelemail.VerificationConfirmReq{},
		"verification request with no email address":   &modelemail.VerificationRequestReq{Email: "not-an-address"},
		"verification request without an email":        &modelemail.VerificationRequestReq{},
		"verification resend with no email address":    &modelemail.VerificationResendReq{Email: "not-an-address"},
	} {
		require.Error(t, binding.Validator.ValidateStruct(payload), name)
	}

	for name, payload := range map[string]any{
		"change cancel":          &modelemail.ChangeCancelReq{Token: "sample-token"},
		"change confirm":         &modelemail.ChangeConfirmReq{Token: "sample-token"},
		"change request":         &modelemail.ChangeRequestReq{NewEmail: "new@sample.test", CurrentPassword: "current-pass9"},
		"change resend":          &modelemail.ChangeResendReq{NewEmail: "new@sample.test"},
		"password reset confirm": &modelemail.PasswordResetConfirmReq{Token: "sample-token", NewPassword: "created-pass9"},
		"password reset request": &modelemail.PasswordResetRequestReq{Email: "user@sample.test"},
		"verification confirm":   &modelemail.VerificationConfirmReq{Token: "sample-token"},
		"verification request":   &modelemail.VerificationRequestReq{Email: "user@sample.test"},
		"verification resend":    &modelemail.VerificationResendReq{Email: "user@sample.test"},
	} {
		require.NoError(t, binding.Validator.ValidateStruct(payload), name)
	}
}
