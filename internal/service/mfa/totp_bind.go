package servicemfa

import (
	"bytes"
	"encoding/base64"
	"net/http"

	"github.com/cockroachdb/errors"
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/pquerna/otp/totp"
	"github.com/skip2/go-qrcode"
	"go.uber.org/zap"
)

var Enabled bool

// TOTPBindService starts the TOTP binding flow for an authenticated user.
//
// The service validates the current user and session, generates a new TOTP
// secret, renders the provisioning URL as a QR code, and stores the secret in a
// short-lived binding challenge tied to the current user and session. The
// response returns only the challenge ID and authenticator setup data, never the
// raw secret as a standalone field.
type TOTPBindService struct {
	service.Base[*modelmfa.TOTPBind, *modelmfa.TOTPBind, *modelmfa.TOTPBindRsp]
}

// Create creates a pending TOTP binding challenge and returns setup metadata.
//
// The method requires an authenticated user and session, generates a new
// server-held secret, stores it in the binding challenge, and returns the
// challenge ID with the provisioning URL and QR image for authenticator setup.
func (t *TOTPBindService) Create(ctx *types.ServiceContext, req *modelmfa.TOTPBind) (rsp *modelmfa.TOTPBindRsp, err error) {
	log := t.WithServiceContext(ctx, ctx.GetPhase())

	if len(ctx.UserID) == 0 {
		log.Errorz("user_id not found in context")
		return nil, types.NewServiceError(http.StatusUnauthorized, "authentication required")
	}
	if len(ctx.Username) == 0 {
		log.Errorz("username not found in context")
		return nil, types.NewServiceError(http.StatusUnauthorized, "authentication required")
	}
	sessionID, err := currentTOTPBindSessionID(ctx)
	if err != nil {
		log.Errorz("session_id not found in context")
		return nil, err
	}

	log.Infoz("generating TOTP for user", zap.String("user_id", ctx.UserID), zap.String("username", ctx.Username))

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      consts.FrameworkName,
		AccountName: ctx.Username,
		SecretSize:  32, // 32 bytes = 256 bits
	})
	if err != nil {
		log.Errorz("failed to generate TOTP key", zap.Error(err))
		return nil, errors.New("failed to generate TOTP key")
	}

	qrCodeURL := key.URL()

	qrCodeImage, err := generateQRCode(qrCodeURL)
	if err != nil {
		log.Errorz("failed to generate QR code image", zap.Error(err))
		return nil, errors.New("failed to generate QR code image")
	}

	challengeID, _, err := issueTOTPBindChallenge(ctx.Context(), totpBindChallenge{
		UserID:    ctx.UserID,
		SessionID: sessionID,
		Username:  ctx.Username,
		Secret:    key.Secret(),
	})
	if err != nil {
		log.Errorz("failed to issue TOTP bind challenge", zap.Error(err))
		return nil, err
	}

	rsp = &modelmfa.TOTPBindRsp{
		ChallengeID: challengeID,
		OtpauthURL:  qrCodeURL,
		QRCodeImage: qrCodeImage,
		Issuer:      consts.FrameworkName,
		AccountName: ctx.Username,
	}

	log.Infoz("generated TOTP bind challenge",
		zap.String("user_id", ctx.UserID),
		zap.String("challenge_id", challengeID))

	return rsp, nil
}

// generateQRCode generates a QR code data URL.
func generateQRCode(url string) (string, error) {
	qrBytes, err := qrcode.Encode(url, qrcode.Medium, 256)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	buf.WriteString("data:image/png;base64,")
	buf.WriteString(base64.StdEncoding.EncodeToString(qrBytes))

	return buf.String(), nil
}
