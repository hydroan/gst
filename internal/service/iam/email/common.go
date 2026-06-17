package serviceiamemail

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/types"
)

type iamEmailFlowKind string

const (
	iamEmailFlowKindVerification  iamEmailFlowKind = "verification"
	iamEmailFlowKindPasswordReset iamEmailFlowKind = "password_reset"
	iamEmailFlowKindChangeConfirm iamEmailFlowKind = "change_confirm"
	iamEmailFlowKindChangeCancel  iamEmailFlowKind = "change_cancel"
)

type emailThrottleAction string

const (
	emailThrottleRequest emailThrottleAction = "request"
	emailThrottleResend  emailThrottleAction = "resend"
)

type iamEmailFlowState struct {
	Kind      iamEmailFlowKind `json:"kind"`
	UserID    string           `json:"user_id,omitempty"`
	Email     string           `json:"email,omitempty"`
	NewEmail  string           `json:"new_email,omitempty"`
	OldEmail  string           `json:"old_email,omitempty"`
	Metadata  map[string]any   `json:"metadata,omitempty"`
	IssuedAt  time.Time        `json:"issued_at"`
	ExpiresAt time.Time        `json:"expires_at"`
}

type emailThrottleRecord struct {
	Kind        iamEmailFlowKind    `json:"kind"`
	Action      emailThrottleAction `json:"action"`
	Scope       string              `json:"scope"`
	CreatedAt   time.Time           `json:"created_at"`
	AvailableAt time.Time           `json:"available_at"`
}

type emailPolicy struct {
	VerificationTokenTTL  time.Duration
	PasswordResetTokenTTL time.Duration
	ChangeTokenTTL        time.Duration
	RequestCooldown       time.Duration
	ResendCooldown        time.Duration
}

type emailDelivery struct {
	To       string         `json:"to"`
	Subject  string         `json:"subject,omitempty"`
	TextBody string         `json:"text_body,omitempty"`
	HTMLBody string         `json:"html_body,omitempty"`
	Template string         `json:"template,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

type iamEmailDeliverySender interface {
	Send(context.Context, emailDelivery) error
}

type noopEmailSender struct{}

func (noopEmailSender) Send(context.Context, emailDelivery) error { return nil }

var (
	errEmailFlowNotFound    = errors.New("email flow not found")
	errEmailFlowExpired     = errors.New("email flow expired")
	errEmailFlowThrottled   = errors.New("email flow throttled")
	errEmailFlowKindInvalid = errors.New("email flow kind invalid")
	errEmailUserNotFound    = errors.New("email user not found")

	defaultEmailPolicy = emailPolicy{
		VerificationTokenTTL:  24 * time.Hour,
		PasswordResetTokenTTL: 30 * time.Minute,
		ChangeTokenTTL:        30 * time.Minute,
		RequestCooldown:       1 * time.Minute,
		ResendCooldown:        30 * time.Second,
	}

	emailNow                                  = func() time.Time { return time.Now().UTC() }
	emailRandomReader                         = rand.Reader
	emailFlowCache                            = func() types.Cache[iamEmailFlowState] { return redis.Cache[iamEmailFlowState]() }
	emailThrottleCache                        = func() types.Cache[emailThrottleRecord] { return redis.Cache[emailThrottleRecord]() }
	activeEmailSender  iamEmailDeliverySender = noopEmailSender{}
)

func setEmailSender(sender iamEmailDeliverySender) {
	if sender == nil {
		activeEmailSender = noopEmailSender{}
		return
	}
	activeEmailSender = sender
}

func dispatchEmail(ctx context.Context, delivery emailDelivery) error {
	delivery.To = normalizeEmailScope(delivery.To)
	if delivery.To == "" {
		return errors.New("email recipient is required")
	}
	return activeEmailSender.Send(normalizeContext(ctx), delivery)
}

func issueEmailFlow(ctx context.Context, kind iamEmailFlowKind, flow iamEmailFlowState, ttl time.Duration) (string, iamEmailFlowState, error) {
	if !validEmailFlowKind(kind) {
		return "", iamEmailFlowState{}, errEmailFlowKindInvalid
	}
	if ttl <= 0 {
		ttl = defaultEmailPolicy.tokenTTL(kind)
	}

	token, err := newEmailFlowToken()
	if err != nil {
		return "", iamEmailFlowState{}, errors.Wrap(err, "generate email flow token")
	}

	now := emailNow()
	flow.Kind = kind
	flow.Email = normalizeEmailScope(flow.Email)
	flow.NewEmail = normalizeEmailScope(flow.NewEmail)
	flow.OldEmail = normalizeEmailScope(flow.OldEmail)
	flow.IssuedAt = now
	flow.ExpiresAt = now.Add(ttl)

	if err = emailFlowCache().WithContext(normalizeContext(ctx)).Set(emailFlowKey(kind, token), flow, ttl); err != nil {
		return "", iamEmailFlowState{}, errors.Wrap(err, "store email flow")
	}

	return token, flow, nil
}

func loadEmailFlow(ctx context.Context, kind iamEmailFlowKind, token string) (iamEmailFlowState, error) {
	if !validEmailFlowKind(kind) {
		return iamEmailFlowState{}, errEmailFlowKindInvalid
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return iamEmailFlowState{}, errEmailFlowNotFound
	}

	flow, err := emailFlowCache().WithContext(normalizeContext(ctx)).Get(emailFlowKey(kind, token))
	if err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			return iamEmailFlowState{}, errEmailFlowNotFound
		}
		return iamEmailFlowState{}, errors.Wrap(err, "load email flow")
	}
	if flow.Kind != kind {
		return iamEmailFlowState{}, errEmailFlowKindInvalid
	}
	if !flow.ExpiresAt.IsZero() && !flow.ExpiresAt.After(emailNow()) {
		_ = emailFlowCache().WithContext(normalizeContext(ctx)).Delete(emailFlowKey(kind, token))
		return iamEmailFlowState{}, errEmailFlowExpired
	}

	return flow, nil
}

func consumeEmailFlow(ctx context.Context, kind iamEmailFlowKind, token string) (iamEmailFlowState, error) {
	flow, err := loadEmailFlow(ctx, kind, token)
	if err != nil {
		return iamEmailFlowState{}, err
	}
	if err = emailFlowCache().WithContext(normalizeContext(ctx)).Delete(emailFlowKey(kind, strings.TrimSpace(token))); err != nil {
		return iamEmailFlowState{}, errors.Wrap(err, "consume email flow")
	}
	return flow, nil
}

func reserveEmailThrottle(ctx context.Context, kind iamEmailFlowKind, action emailThrottleAction, scope string, cooldown time.Duration) (time.Duration, error) {
	if !validEmailFlowKind(kind) {
		return 0, errEmailFlowKindInvalid
	}
	if cooldown <= 0 {
		cooldown = defaultEmailPolicy.cooldown(action)
	}
	scope = normalizeEmailScope(scope)
	if scope == "" {
		return 0, errors.New("email throttle scope is required")
	}

	key := emailThrottleKey(kind, action, scope)
	record, err := emailThrottleCache().WithContext(normalizeContext(ctx)).Get(key)
	if err == nil {
		if wait := record.AvailableAt.Sub(emailNow()); wait > 0 {
			return wait, errEmailFlowThrottled
		}
	} else if !errors.Is(err, types.ErrEntryNotFound) {
		return 0, errors.Wrap(err, "load email throttle")
	}

	now := emailNow()
	record = emailThrottleRecord{
		Kind:        kind,
		Action:      action,
		Scope:       scope,
		CreatedAt:   now,
		AvailableAt: now.Add(cooldown),
	}
	if err = emailThrottleCache().WithContext(normalizeContext(ctx)).Set(key, record, cooldown); err != nil {
		return 0, errors.Wrap(err, "store email throttle")
	}

	return 0, nil
}

func publicAcceptedMessage(kind iamEmailFlowKind) string {
	switch kind {
	case iamEmailFlowKindPasswordReset:
		return "If the email is eligible, a password reset message will be sent shortly."
	default:
		return "If the email is eligible, a verification message will be sent shortly."
	}
}

func emailFlowKey(kind iamEmailFlowKind, token string) string {
	return strings.Join([]string{"iam", "email", "flow", string(kind), strings.TrimSpace(token)}, ":")
}

func emailThrottleKey(kind iamEmailFlowKind, action emailThrottleAction, scope string) string {
	return strings.Join([]string{"iam", "email", "throttle", string(kind), string(action), normalizeEmailScope(scope)}, ":")
}

func newEmailFlowToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := io.ReadFull(emailRandomReader, buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func normalizeEmailScope(scope string) string {
	return strings.ToLower(strings.TrimSpace(scope))
}

func validEmailFlowKind(kind iamEmailFlowKind) bool {
	switch kind {
	case iamEmailFlowKindVerification, iamEmailFlowKindPasswordReset, iamEmailFlowKindChangeConfirm, iamEmailFlowKindChangeCancel:
		return true
	default:
		return false
	}
}

func (p emailPolicy) tokenTTL(kind iamEmailFlowKind) time.Duration {
	switch kind {
	case iamEmailFlowKindVerification:
		if p.VerificationTokenTTL > 0 {
			return p.VerificationTokenTTL
		}
	case iamEmailFlowKindPasswordReset:
		if p.PasswordResetTokenTTL > 0 {
			return p.PasswordResetTokenTTL
		}
	case iamEmailFlowKindChangeConfirm, iamEmailFlowKindChangeCancel:
		if p.ChangeTokenTTL > 0 {
			return p.ChangeTokenTTL
		}
	}
	return 30 * time.Minute
}

func (p emailPolicy) cooldown(action emailThrottleAction) time.Duration {
	switch action {
	case emailThrottleResend:
		if p.ResendCooldown > 0 {
			return p.ResendCooldown
		}
	default:
		if p.RequestCooldown > 0 {
			return p.RequestCooldown
		}
	}
	return 30 * time.Second
}
