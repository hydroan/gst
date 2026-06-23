package email

import modelemail "github.com/hydroan/gst/internal/model/email"

type (
	EmailChangeConfirmReq = modelemail.ChangeConfirmReq
	EmailChangeConfirmRsp = modelemail.ChangeConfirmRsp
	EmailChangeCancelReq  = modelemail.ChangeCancelReq
	EmailChangeCancelRsp  = modelemail.ChangeCancelRsp
	EmailChangeRequestReq = modelemail.ChangeRequestReq
	EmailChangeRequestRsp = modelemail.ChangeRequestRsp
	EmailChangeResendReq  = modelemail.ChangeResendReq
	EmailChangeResendRsp  = modelemail.ChangeResendRsp

	EmailPasswordResetConfirmReq = modelemail.PasswordResetConfirmReq
	EmailPasswordResetConfirmRsp = modelemail.PasswordResetConfirmRsp
	EmailPasswordResetRequestReq = modelemail.PasswordResetRequestReq
	EmailPasswordResetRequestRsp = modelemail.PasswordResetRequestRsp

	EmailVerificationConfirmReq = modelemail.VerificationConfirmReq
	EmailVerificationConfirmRsp = modelemail.VerificationConfirmRsp
	EmailVerificationResendReq  = modelemail.VerificationResendReq
	EmailVerificationResendRsp  = modelemail.VerificationResendRsp
	EmailVerificationRequestReq = modelemail.VerificationRequestReq
	EmailVerificationRequestRsp = modelemail.VerificationRequestRsp
)
