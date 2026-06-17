package serviceiamemail

import (
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamemail "github.com/hydroan/gst/internal/model/iam/email"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// VerificationConfirmService handles the token confirmation step that finalizes
// the email verification flow.
type VerificationConfirmService struct {
	service.Base[*model.Empty, *modeliamemail.VerificationConfirmReq, *modeliamemail.VerificationConfirmRsp]
}

var (
	// verificationLoadUserByID loads the account referenced by the verification token.
	verificationLoadUserByID = func(ctx *types.ServiceContext, userID string) (*modeliamuser.User, error) {
		user := new(modeliamuser.User)
		if err := database.Database[*modeliamuser.User](ctx.DatabaseContext()).Get(user, userID); err != nil {
			return nil, err
		}
		return user, nil
	}
	// verificationUpdateUser persists the verified email state for the target user.
	verificationUpdateUser = func(ctx *types.ServiceContext, user *modeliamuser.User) error {
		return database.Database[*modeliamuser.User](ctx.DatabaseContext()).
			WithoutHook().
			WithSelect("email_verified", "email_verified_at").
			Update(user)
	}
)

// Create consumes the one-time verification token and marks the corresponding
// email address as verified when the current account state still matches.
func (s *VerificationConfirmService) Create(ctx *types.ServiceContext, req *modeliamemail.VerificationConfirmReq) (rsp *modeliamemail.VerificationConfirmRsp, err error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	flow, err := consumeEmailFlow(passwordResetContext(ctx), iamEmailFlowKindVerification, req.Token)
	if err != nil {
		if errors.Is(err, errEmailFlowNotFound) || errors.Is(err, errEmailFlowExpired) {
			return &modeliamemail.VerificationConfirmRsp{
				Verified: false,
				Msg:      "invalid or expired verification token",
			}, nil
		}
		log.Error("failed to consume verification flow", err)
		return nil, errors.Wrap(err, "failed to consume verification flow")
	}
	if strings.TrimSpace(flow.UserID) == "" {
		return nil, errors.New("verification user id is required")
	}

	user, err := verificationLoadUserByID(ctx, flow.UserID)
	if err != nil {
		log.Error("failed to load verification user", err)
		return nil, errors.Wrap(err, "failed to load verification user")
	}
	if normalizePasswordResetEmail(user.Email) != normalizeEmailScope(flow.Email) {
		return &modeliamemail.VerificationConfirmRsp{
			Verified: false,
			Msg:      "invalid or expired verification token",
		}, nil
	}
	if userEmailVerified(user) {
		return &modeliamemail.VerificationConfirmRsp{
			Verified: true,
			Msg:      "email already verified",
		}, nil
	}

	if err = applyEmailVerification(user, emailNow()); err != nil {
		log.Error("failed to apply email verification", err)
		return nil, err
	}
	if err = verificationUpdateUser(ctx, user); err != nil {
		log.Error("failed to update verification user", err)
		return nil, errors.Wrap(err, "failed to update email verification state")
	}

	return &modeliamemail.VerificationConfirmRsp{
		Verified: true,
		Msg:      "email verified successfully",
	}, nil
}

// applyEmailVerification updates the in-memory user model with the verified
// email flags before persistence.
func applyEmailVerification(user *modeliamuser.User, verifiedAt time.Time) error {
	if user == nil {
		return errors.New("verification user is required")
	}
	verified := true
	verifiedAt = verifiedAt.UTC()
	user.EmailVerified = &verified
	user.EmailVerifiedAt = &verifiedAt
	return nil
}
