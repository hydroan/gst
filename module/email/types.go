package email

import modelemail "github.com/hydroan/gst/internal/model/email"

type (
	EmailVerificationRequestReq = modelemail.VerificationRequestReq
	EmailVerificationRequestRsp = modelemail.VerificationRequestRsp
	EmailVerificationResendReq  = modelemail.VerificationResendReq
	EmailVerificationResendRsp  = modelemail.VerificationResendRsp
	EmailVerificationConfirmReq = modelemail.VerificationConfirmReq
	EmailVerificationConfirmRsp = modelemail.VerificationConfirmRsp

	EmailPasswordResetRequestReq = modelemail.PasswordResetRequestReq
	EmailPasswordResetRequestRsp = modelemail.PasswordResetRequestRsp
	EmailPasswordResetConfirmReq = modelemail.PasswordResetConfirmReq
	EmailPasswordResetConfirmRsp = modelemail.PasswordResetConfirmRsp

	EmailChangeRequestReq = modelemail.ChangeRequestReq
	EmailChangeRequestRsp = modelemail.ChangeRequestRsp
	EmailChangeResendReq  = modelemail.ChangeResendReq
	EmailChangeResendRsp  = modelemail.ChangeResendRsp
	EmailChangeCancelReq  = modelemail.ChangeCancelReq
	EmailChangeCancelRsp  = modelemail.ChangeCancelRsp
	EmailChangeConfirmReq = modelemail.ChangeConfirmReq
	EmailChangeConfirmRsp = modelemail.ChangeConfirmRsp
)
