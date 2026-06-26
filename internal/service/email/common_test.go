package serviceemail

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	modelemail "github.com/hydroan/gst/internal/model/email"
	loggerzap "github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

type testCacheEntry[T any] struct {
	value     T
	expiresAt time.Time
}

type testCache[T any] struct {
	items map[string]testCacheEntry[T]
}

func newTestCache[T any]() *testCache[T] {
	return &testCache[T]{items: make(map[string]testCacheEntry[T])}
}

func newEmailServiceContext(userID string) *types.ServiceContext {
	baseCtx := types.ContextWithRequestMetadata(context.Background(), types.NewRequestMetadata(types.RequestMetadataFields{
		UserID: userID,
	}))
	return types.NewServiceContext(nil, baseCtx, consts.PHASE_CREATE)
}

func (c *testCache[T]) Get(key string) (T, error) {
	var zero T
	entry, ok := c.items[key]
	if !ok {
		return zero, types.ErrEntryNotFound
	}
	if !entry.expiresAt.IsZero() && !entry.expiresAt.After(emailNow()) {
		delete(c.items, key)
		return zero, types.ErrEntryNotFound
	}
	return entry.value, nil
}

func (c *testCache[T]) Peek(key string) (T, error) {
	return c.Get(key)
}

func (c *testCache[T]) Set(key string, value T, ttl time.Duration) error {
	entry := testCacheEntry[T]{value: value}
	if ttl > 0 {
		entry.expiresAt = emailNow().Add(ttl)
	}
	c.items[key] = entry
	return nil
}

func (c *testCache[T]) Delete(key string) error {
	if _, ok := c.items[key]; !ok {
		return types.ErrEntryNotFound
	}
	delete(c.items, key)
	return nil
}

func (c *testCache[T]) Exists(key string) bool {
	_, err := c.Get(key)
	return err == nil
}

func (c *testCache[T]) Len() int {
	return len(c.items)
}

func (c *testCache[T]) Clear() {
	clear(c.items)
}

func (c *testCache[T]) WithContext(context.Context) types.Cache[T] {
	return c
}

type testEmailSender struct {
	last       emailDelivery
	deliveries []emailDelivery
}

func (s *testEmailSender) Send(_ context.Context, delivery emailDelivery) error {
	s.last = delivery
	s.deliveries = append(s.deliveries, delivery)
	return nil
}

type testAccountGateway struct {
	findByEmail        func(*types.ServiceContext, string) (*AccountSnapshot, error)
	getByID            func(*types.ServiceContext, string) (*AccountSnapshot, error)
	verifyPassword     func(*types.ServiceContext, string, string) error
	updatePassword     func(*types.ServiceContext, string, string) error
	markEmailVerified  func(*types.ServiceContext, string, time.Time) error
	applyEmailChange   func(*types.ServiceContext, string, string, time.Time) error
	invalidateSessions func(string)
}

func (p testAccountGateway) FindByEmail(ctx *types.ServiceContext, email string) (*AccountSnapshot, error) {
	if p.findByEmail == nil {
		return nil, ErrAccountNotFound
	}
	return p.findByEmail(ctx, email)
}

func (p testAccountGateway) GetByID(ctx *types.ServiceContext, userID string) (*AccountSnapshot, error) {
	if p.getByID == nil {
		return nil, ErrAccountNotFound
	}
	return p.getByID(ctx, userID)
}

func (p testAccountGateway) VerifyPassword(ctx *types.ServiceContext, userID, password string) error {
	if p.verifyPassword == nil {
		return ErrAccountAuthenticationFailed
	}
	return p.verifyPassword(ctx, userID, password)
}

func (p testAccountGateway) UpdatePassword(ctx *types.ServiceContext, userID, newPassword string) error {
	if p.updatePassword == nil {
		return nil
	}
	return p.updatePassword(ctx, userID, newPassword)
}

func (p testAccountGateway) MarkEmailVerified(ctx *types.ServiceContext, userID string, verifiedAt time.Time) error {
	if p.markEmailVerified == nil {
		return nil
	}
	return p.markEmailVerified(ctx, userID, verifiedAt)
}

func (p testAccountGateway) ApplyEmailChange(ctx *types.ServiceContext, userID, newEmail string, changedAt time.Time) error {
	if p.applyEmailChange == nil {
		return nil
	}
	return p.applyEmailChange(ctx, userID, newEmail, changedAt)
}

func (p testAccountGateway) InvalidateSessions(userID string) {
	if p.invalidateSessions != nil {
		p.invalidateSessions(userID)
	}
}

func testEmailAccount(id, email string, verified bool) *AccountSnapshot {
	return &AccountSnapshot{
		ID:            id,
		Email:         email,
		Active:        true,
		EmailVerified: verified,
	}
}

func requireServiceError(t *testing.T, err error, status int, message string) {
	t.Helper()

	var serviceErr *service.Error
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, status, serviceErr.Status())
	require.Equal(t, message, serviceErr.Msg())
}

func TestIssueLoadConsumeEmailFlow(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 8, 0, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{1}, 64)))
	t.Cleanup(restore)

	token, issued, err := issueEmailFlow(context.Background(), iamEmailFlowKindVerification, iamEmailFlowState{
		Email:    " USER@Example.COM ",
		Metadata: map[string]any{"source": "signup"},
	}, 0)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.Equal(t, iamEmailFlowKindVerification, issued.Kind)
	require.Equal(t, "user@example.com", issued.Email)
	require.Equal(t, now, issued.IssuedAt)
	require.Equal(t, now.Add(24*time.Hour), issued.ExpiresAt)

	loaded, err := loadEmailFlow(context.Background(), iamEmailFlowKindVerification, token)
	require.NoError(t, err)
	require.Equal(t, issued, loaded)

	consumed, err := consumeEmailFlow(context.Background(), iamEmailFlowKindVerification, token)
	require.NoError(t, err)
	require.Equal(t, issued, consumed)

	_, err = loadEmailFlow(context.Background(), iamEmailFlowKindVerification, token)
	require.ErrorIs(t, err, errEmailFlowNotFound)
}

func TestLoadEmailFlowExpired(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 9, 0, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{2}, 64)))
	t.Cleanup(restore)

	token, err := newEmailFlowToken()
	require.NoError(t, err)
	err = flowCache.Set(emailFlowKey(iamEmailFlowKindPasswordReset, token), iamEmailFlowState{
		Kind:      iamEmailFlowKindPasswordReset,
		Email:     "user@example.com",
		IssuedAt:  now.Add(-2 * time.Minute),
		ExpiresAt: now.Add(-1 * time.Minute),
	}, 0)
	require.NoError(t, err)

	emailNow = func() time.Time { return now.Add(2 * time.Minute) }

	_, err = loadEmailFlow(context.Background(), iamEmailFlowKindPasswordReset, token)
	require.ErrorIs(t, err, errEmailFlowExpired)
}

func TestReserveEmailThrottle(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{3}, 64)))
	t.Cleanup(restore)

	wait, err := reserveEmailThrottle(context.Background(), iamEmailFlowKindVerification, emailThrottleRequest, "USER@example.com", time.Minute)
	require.NoError(t, err)
	require.Zero(t, wait)

	wait, err = reserveEmailThrottle(context.Background(), iamEmailFlowKindVerification, emailThrottleRequest, "user@example.com", time.Minute)
	require.ErrorIs(t, err, errEmailFlowThrottled)
	require.Greater(t, wait, time.Duration(0))

	emailNow = func() time.Time { return now.Add(2 * time.Minute) }

	wait, err = reserveEmailThrottle(context.Background(), iamEmailFlowKindVerification, emailThrottleRequest, "user@example.com", time.Minute)
	require.NoError(t, err)
	require.Zero(t, wait)
}

func TestDispatchEmail(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 11, 0, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{4}, 64)))
	t.Cleanup(restore)

	sender := new(testEmailSender)
	activeEmailSender = sender

	err := dispatchEmail(context.Background(), emailDelivery{To: "  USER@Example.COM  ", Subject: "Verify"})
	require.NoError(t, err)
	require.Equal(t, "user@example.com", sender.last.To)
	require.Equal(t, "Verify", sender.last.Subject)

	err = dispatchEmail(context.Background(), emailDelivery{})
	require.EqualError(t, err, "email recipient is required")
}

func TestPublicAcceptedMessage(t *testing.T) {
	require.Equal(t, "If the email is eligible, a verification message will be sent shortly.", publicAcceptedMessage(iamEmailFlowKindVerification))
	require.Equal(t, "If the email is eligible, a password reset message will be sent shortly.", publicAcceptedMessage(iamEmailFlowKindPasswordReset))
}

func TestEmailServiceContext(t *testing.T) {
	require.NotNil(t, emailServiceContext(nil))
	require.NotNil(t, emailServiceContext(&types.ServiceContext{}))
}

func stubEmailGlobals(flowCache types.Cache[iamEmailFlowState], throttleCache types.Cache[emailThrottleRecord], now time.Time, reader *bytes.Reader) func() {
	previousFlowCache := emailFlowCache
	previousThrottleCache := emailThrottleCache
	previousNow := emailNow
	previousReader := emailRandomReader
	previousSender := activeEmailSender
	previousGateway := currentAccountGateway()

	emailFlowCache = func() types.Cache[iamEmailFlowState] { return flowCache }
	emailThrottleCache = func() types.Cache[emailThrottleRecord] { return throttleCache }
	emailNow = func() time.Time { return now }
	emailRandomReader = reader
	activeEmailSender = noopEmailSender{}
	SetAccountGateway(nil)

	return func() {
		emailFlowCache = previousFlowCache
		emailThrottleCache = previousThrottleCache
		emailNow = previousNow
		emailRandomReader = previousReader
		activeEmailSender = previousSender
		SetAccountGateway(previousGateway)
	}
}

func TestInvalidKind(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{5}, 64)))
	t.Cleanup(restore)

	_, _, err := issueEmailFlow(context.Background(), iamEmailFlowKind("unknown"), iamEmailFlowState{}, 0)
	require.ErrorIs(t, err, errEmailFlowKindInvalid)

	_, err = reserveEmailThrottle(context.Background(), iamEmailFlowKind("unknown"), emailThrottleRequest, "user@example.com", time.Minute)
	require.ErrorIs(t, err, errEmailFlowKindInvalid)
}

func TestMissingTokenReturnsNotFound(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 13, 0, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{6}, 64)))
	t.Cleanup(restore)

	_, err := loadEmailFlow(context.Background(), iamEmailFlowKindVerification, " ")
	require.True(t, errors.Is(err, errEmailFlowNotFound))
}

func TestVerificationRequestCreateReturnsProviderConfigurationError(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 13, 15, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{26}, 64)))
	t.Cleanup(restore)
	SetAccountGateway(nil)
	t.Cleanup(func() {
		SetAccountGateway(nil)
	})

	svc := &VerificationRequestService{}
	svc.Logger = loggerzap.New("")
	ctx := types.NewServiceContext(nil, nil, consts.PHASE_CREATE)

	_, err := svc.Create(ctx, &modelemail.VerificationRequestReq{Email: "user@example.com"})

	requireServiceError(t, err, 500, "Email account gateway is not configured")
	require.ErrorIs(t, err, ErrAccountGatewayNotConfigured)
}

func TestVerificationRequestCreate(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 13, 30, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{11}, 64)))
	t.Cleanup(restore)

	sender := new(testEmailSender)
	activeEmailSender = sender

	SetAccountGateway(testAccountGateway{
		findByEmail: func(_ *types.ServiceContext, email string) (*AccountSnapshot, error) {
			require.Equal(t, "user@example.com", email)
			return testEmailAccount("user-verify-1", "user@example.com", false), nil
		},
	})

	svc := &VerificationRequestService{}
	svc.Logger = loggerzap.New("")
	ctx := types.NewServiceContext(nil, nil, consts.PHASE_CREATE)

	rsp, err := svc.Create(ctx, &modelemail.VerificationRequestReq{Email: " USER@example.com "})
	require.NoError(t, err)
	require.Equal(t, publicAcceptedMessage(iamEmailFlowKindVerification), rsp.Msg)
	require.Equal(t, "user@example.com", sender.last.To)
	require.Equal(t, "Email verification", sender.last.Subject)
	require.NotEmpty(t, sender.last.Data["token"])
	require.Equal(t, "user-verify-1", sender.last.Data["user_id"])
	require.Equal(t, 1, flowCache.Len())
}

func TestVerificationRequestCreateVerifiedAccount(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 13, 45, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{12}, 64)))
	t.Cleanup(restore)

	sender := new(testEmailSender)
	activeEmailSender = sender

	SetAccountGateway(testAccountGateway{
		findByEmail: func(*types.ServiceContext, string) (*AccountSnapshot, error) {
			return testEmailAccount("user-verify-2", "user@example.com", true), nil
		},
	})

	svc := &VerificationRequestService{}
	svc.Logger = loggerzap.New("")
	ctx := types.NewServiceContext(nil, nil, consts.PHASE_CREATE)

	rsp, err := svc.Create(ctx, &modelemail.VerificationRequestReq{Email: "user@example.com"})
	require.NoError(t, err)
	require.Equal(t, publicAcceptedMessage(iamEmailFlowKindVerification), rsp.Msg)
	require.Equal(t, 0, flowCache.Len())
	require.Empty(t, sender.last.To)
}

func TestVerificationRequestCreateUnknownAccount(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 13, 47, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{15}, 64)))
	t.Cleanup(restore)

	sender := new(testEmailSender)
	activeEmailSender = sender

	SetAccountGateway(testAccountGateway{
		findByEmail: func(_ *types.ServiceContext, email string) (*AccountSnapshot, error) {
			require.Equal(t, "user@example.com", email)
			return nil, ErrAccountNotFound
		},
	})

	svc := &VerificationRequestService{}
	svc.Logger = loggerzap.New("")
	ctx := types.NewServiceContext(nil, nil, consts.PHASE_CREATE)

	rsp, err := svc.Create(ctx, &modelemail.VerificationRequestReq{Email: "user@example.com"})
	require.NoError(t, err)
	require.Equal(t, publicAcceptedMessage(iamEmailFlowKindVerification), rsp.Msg)
	require.Equal(t, 0, flowCache.Len())
	require.Empty(t, sender.deliveries)
}

func TestVerificationResendCreate(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 13, 50, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{13}, 64)))
	t.Cleanup(restore)

	sender := new(testEmailSender)
	activeEmailSender = sender

	SetAccountGateway(testAccountGateway{
		findByEmail: func(_ *types.ServiceContext, email string) (*AccountSnapshot, error) {
			require.Equal(t, "user@example.com", email)
			return testEmailAccount("user-verify-3", "user@example.com", false), nil
		},
	})

	svc := &VerificationResendService{}
	svc.Logger = loggerzap.New("")
	ctx := types.NewServiceContext(nil, nil, consts.PHASE_CREATE)

	rsp, err := svc.Create(ctx, &modelemail.VerificationResendReq{Email: "user@example.com"})
	require.NoError(t, err)
	require.Equal(t, publicAcceptedMessage(iamEmailFlowKindVerification), rsp.Msg)
	require.Equal(t, "user@example.com", sender.last.To)
	require.Equal(t, "Email verification", sender.last.Subject)
	require.Equal(t, 1, flowCache.Len())
}

func TestVerificationResendCreateUnknownAccount(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 13, 52, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{16}, 64)))
	t.Cleanup(restore)

	sender := new(testEmailSender)
	activeEmailSender = sender

	SetAccountGateway(testAccountGateway{
		findByEmail: func(_ *types.ServiceContext, email string) (*AccountSnapshot, error) {
			require.Equal(t, "user@example.com", email)
			return nil, ErrAccountNotFound
		},
	})

	svc := &VerificationResendService{}
	svc.Logger = loggerzap.New("")
	ctx := types.NewServiceContext(nil, nil, consts.PHASE_CREATE)

	rsp, err := svc.Create(ctx, &modelemail.VerificationResendReq{Email: "user@example.com"})
	require.NoError(t, err)
	require.Equal(t, publicAcceptedMessage(iamEmailFlowKindVerification), rsp.Msg)
	require.Equal(t, 0, flowCache.Len())
	require.Empty(t, sender.deliveries)
}

func TestVerificationResendCreateThrottled(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 13, 55, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{14}, 64)))
	t.Cleanup(restore)

	sender := new(testEmailSender)
	activeEmailSender = sender

	SetAccountGateway(testAccountGateway{
		findByEmail: func(*types.ServiceContext, string) (*AccountSnapshot, error) {
			return testEmailAccount("user-verify-4", "user@example.com", false), nil
		},
	})

	err := throttleCache.Set(emailThrottleKey(iamEmailFlowKindVerification, emailThrottleResend, "user@example.com"), emailThrottleRecord{
		Kind:        iamEmailFlowKindVerification,
		Action:      emailThrottleResend,
		Scope:       "user@example.com",
		CreatedAt:   now,
		AvailableAt: now.Add(30 * time.Second),
	}, time.Minute)
	require.NoError(t, err)

	svc := &VerificationResendService{}
	svc.Logger = loggerzap.New("")
	ctx := types.NewServiceContext(nil, nil, consts.PHASE_CREATE)

	rsp, err := svc.Create(ctx, &modelemail.VerificationResendReq{Email: "user@example.com"})
	require.NoError(t, err)
	require.Equal(t, publicAcceptedMessage(iamEmailFlowKindVerification), rsp.Msg)
	require.Equal(t, 0, flowCache.Len())
	require.Empty(t, sender.last.To)
}

func TestVerificationConfirmCreate(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 13, 58, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{15}, 64)))
	t.Cleanup(restore)

	token, err := newEmailFlowToken()
	require.NoError(t, err)
	err = flowCache.Set(emailFlowKey(iamEmailFlowKindVerification, token), iamEmailFlowState{
		Kind:      iamEmailFlowKindVerification,
		UserID:    "user-verify-5",
		Email:     "user@example.com",
		IssuedAt:  now,
		ExpiresAt: now.Add(24 * time.Hour),
	}, 24*time.Hour)
	require.NoError(t, err)

	var verifiedUserID string
	var verifiedAt time.Time
	SetAccountGateway(testAccountGateway{
		getByID: func(_ *types.ServiceContext, userID string) (*AccountSnapshot, error) {
			require.Equal(t, "user-verify-5", userID)
			return testEmailAccount("user-verify-5", "user@example.com", false), nil
		},
		markEmailVerified: func(_ *types.ServiceContext, userID string, at time.Time) error {
			verifiedUserID = userID
			verifiedAt = at
			return nil
		},
	})

	svc := &VerificationConfirmService{}
	svc.Logger = loggerzap.New("")
	ctx := types.NewServiceContext(nil, nil, consts.PHASE_CREATE)

	rsp, err := svc.Create(ctx, &modelemail.VerificationConfirmReq{Token: token})
	require.NoError(t, err)
	require.True(t, rsp.Verified)
	require.Equal(t, "email verified successfully", rsp.Msg)
	require.Equal(t, "user-verify-5", verifiedUserID)
	require.Equal(t, now, verifiedAt)
	_, err = loadEmailFlow(context.Background(), iamEmailFlowKindVerification, token)
	require.ErrorIs(t, err, errEmailFlowNotFound)
}

func TestVerificationConfirmCreateInvalidToken(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 13, 59, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{16}, 64)))
	t.Cleanup(restore)

	svc := &VerificationConfirmService{}
	svc.Logger = loggerzap.New("")
	ctx := types.NewServiceContext(nil, nil, consts.PHASE_CREATE)

	rsp, err := svc.Create(ctx, &modelemail.VerificationConfirmReq{Token: "missing"})
	require.NoError(t, err)
	require.False(t, rsp.Verified)
	require.Equal(t, "invalid or expired verification token", rsp.Msg)
}

func TestVerificationConfirmCreateAlreadyVerified(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 14, 1, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{17}, 64)))
	t.Cleanup(restore)

	token, err := newEmailFlowToken()
	require.NoError(t, err)
	err = flowCache.Set(emailFlowKey(iamEmailFlowKindVerification, token), iamEmailFlowState{
		Kind:      iamEmailFlowKindVerification,
		UserID:    "user-verify-6",
		Email:     "user@example.com",
		IssuedAt:  now,
		ExpiresAt: now.Add(24 * time.Hour),
	}, 24*time.Hour)
	require.NoError(t, err)

	SetAccountGateway(testAccountGateway{
		getByID: func(*types.ServiceContext, string) (*AccountSnapshot, error) {
			return testEmailAccount("user-verify-6", "user@example.com", true), nil
		},
		markEmailVerified: func(*types.ServiceContext, string, time.Time) error {
			t.Fatalf("MarkEmailVerified should not be called for already verified account")
			return nil
		},
	})

	svc := &VerificationConfirmService{}
	svc.Logger = loggerzap.New("")
	ctx := types.NewServiceContext(nil, nil, consts.PHASE_CREATE)

	rsp, err := svc.Create(ctx, &modelemail.VerificationConfirmReq{Token: token})
	require.NoError(t, err)
	require.True(t, rsp.Verified)
	require.Equal(t, "email already verified", rsp.Msg)
}

func TestChangeRequestCreate(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 14, 5, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{18}, 64)))
	t.Cleanup(restore)

	sender := new(testEmailSender)
	activeEmailSender = sender

	SetAccountGateway(testAccountGateway{
		getByID: func(_ *types.ServiceContext, userID string) (*AccountSnapshot, error) {
			require.Equal(t, "user-change-1", userID)
			return testEmailAccount("user-change-1", "old@example.com", false), nil
		},
		findByEmail: func(_ *types.ServiceContext, email string) (*AccountSnapshot, error) {
			require.Equal(t, "new@example.com", email)
			return nil, ErrAccountNotFound
		},
		verifyPassword: func(_ *types.ServiceContext, userID, password string) error {
			require.Equal(t, "user-change-1", userID)
			require.Equal(t, "current-password", password)
			return nil
		},
	})

	svc := &ChangeRequestService{}
	svc.Logger = loggerzap.New("")
	ctx := newEmailServiceContext("user-change-1")

	rsp, err := svc.Create(ctx, &modelemail.ChangeRequestReq{
		NewEmail:        " NEW@example.com ",
		CurrentPassword: "current-password",
	})
	require.NoError(t, err)
	require.Equal(t, "email change request submitted successfully", rsp.Msg)
	require.Equal(t, 2, flowCache.Len())
	require.Len(t, sender.deliveries, 2)
	require.Equal(t, "new@example.com", sender.deliveries[0].To)
	require.Equal(t, "Email change confirmation", sender.deliveries[0].Subject)
	require.Equal(t, "old@example.com", sender.deliveries[1].To)
	require.Equal(t, "Email change cancellation", sender.deliveries[1].Subject)
}

func TestChangeRequestCreateEmailAlreadyUsed(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 14, 6, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{19}, 64)))
	t.Cleanup(restore)

	sender := new(testEmailSender)
	activeEmailSender = sender

	var passwordVerified bool
	SetAccountGateway(testAccountGateway{
		getByID: func(*types.ServiceContext, string) (*AccountSnapshot, error) {
			return testEmailAccount("user-change-2", "old@example.com", false), nil
		},
		findByEmail: func(*types.ServiceContext, string) (*AccountSnapshot, error) {
			return testEmailAccount("user-change-other", "new@example.com", false), nil
		},
		verifyPassword: func(*types.ServiceContext, string, string) error {
			passwordVerified = true
			return nil
		},
	})

	svc := &ChangeRequestService{}
	svc.Logger = loggerzap.New("")
	ctx := newEmailServiceContext("user-change-2")

	_, err := svc.Create(ctx, &modelemail.ChangeRequestReq{
		NewEmail:        "new@example.com",
		CurrentPassword: "current-password",
	})
	require.EqualError(t, err, "new email is already in use")
	require.Zero(t, flowCache.Len())
	require.Empty(t, sender.deliveries)
	require.False(t, passwordVerified)
}

func TestChangeResendCreate(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 14, 7, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{20}, 64)))
	t.Cleanup(restore)

	sender := new(testEmailSender)
	activeEmailSender = sender

	SetAccountGateway(testAccountGateway{
		getByID: func(_ *types.ServiceContext, userID string) (*AccountSnapshot, error) {
			require.Equal(t, "user-change-3", userID)
			return testEmailAccount("user-change-3", "old@example.com", false), nil
		},
		findByEmail: func(_ *types.ServiceContext, email string) (*AccountSnapshot, error) {
			require.Equal(t, "new@example.com", email)
			return nil, ErrAccountNotFound
		},
	})

	svc := &ChangeResendService{}
	svc.Logger = loggerzap.New("")
	ctx := newEmailServiceContext("user-change-3")

	rsp, err := svc.Create(ctx, &modelemail.ChangeResendReq{NewEmail: "new@example.com"})
	require.NoError(t, err)
	require.Equal(t, "email change confirmation resent successfully", rsp.Msg)
	require.Equal(t, 1, flowCache.Len())
	require.Len(t, sender.deliveries, 1)
	require.Equal(t, "new@example.com", sender.deliveries[0].To)
	require.Equal(t, "Email change confirmation", sender.deliveries[0].Subject)
}

func TestChangeResendCreateThrottled(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 14, 8, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{21}, 64)))
	t.Cleanup(restore)

	sender := new(testEmailSender)
	activeEmailSender = sender

	SetAccountGateway(testAccountGateway{
		getByID: func(*types.ServiceContext, string) (*AccountSnapshot, error) {
			return testEmailAccount("user-change-4", "old@example.com", false), nil
		},
		findByEmail: func(*types.ServiceContext, string) (*AccountSnapshot, error) {
			return nil, ErrAccountNotFound
		},
	})

	err := throttleCache.Set(emailThrottleKey(iamEmailFlowKindChangeConfirm, emailThrottleResend, "new@example.com"), emailThrottleRecord{
		Kind:        iamEmailFlowKindChangeConfirm,
		Action:      emailThrottleResend,
		Scope:       "new@example.com",
		CreatedAt:   now,
		AvailableAt: now.Add(30 * time.Second),
	}, time.Minute)
	require.NoError(t, err)

	svc := &ChangeResendService{}
	svc.Logger = loggerzap.New("")
	ctx := newEmailServiceContext("user-change-4")

	rsp, err := svc.Create(ctx, &modelemail.ChangeResendReq{NewEmail: "new@example.com"})
	require.NoError(t, err)
	require.Equal(t, "email change confirmation resent successfully", rsp.Msg)
	require.Zero(t, flowCache.Len())
	require.Empty(t, sender.deliveries)
}

func TestChangeConfirmCreate(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 14, 9, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{22}, 64)))
	t.Cleanup(restore)

	token, err := newEmailFlowToken()
	require.NoError(t, err)
	err = flowCache.Set(emailFlowKey(iamEmailFlowKindChangeConfirm, token), iamEmailFlowState{
		Kind:      iamEmailFlowKindChangeConfirm,
		UserID:    "user-change-5",
		OldEmail:  "old@example.com",
		NewEmail:  "new@example.com",
		Email:     "new@example.com",
		IssuedAt:  now,
		ExpiresAt: now.Add(30 * time.Minute),
	}, 30*time.Minute)
	require.NoError(t, err)

	var changedUserID string
	var changedEmail string
	var changedAt time.Time
	SetAccountGateway(testAccountGateway{
		getByID: func(_ *types.ServiceContext, userID string) (*AccountSnapshot, error) {
			require.Equal(t, "user-change-5", userID)
			return testEmailAccount("user-change-5", "old@example.com", false), nil
		},
		findByEmail: func(_ *types.ServiceContext, email string) (*AccountSnapshot, error) {
			require.Equal(t, "new@example.com", email)
			return nil, ErrAccountNotFound
		},
		applyEmailChange: func(_ *types.ServiceContext, userID, newEmail string, at time.Time) error {
			changedUserID = userID
			changedEmail = newEmail
			changedAt = at
			return nil
		},
	})

	svc := &ChangeConfirmService{}
	svc.Logger = loggerzap.New("")
	ctx := types.NewServiceContext(nil, nil, consts.PHASE_CREATE)

	rsp, err := svc.Create(ctx, &modelemail.ChangeConfirmReq{Token: token})
	require.NoError(t, err)
	require.True(t, rsp.Changed)
	require.Equal(t, "email changed successfully", rsp.Msg)
	require.Equal(t, "user-change-5", changedUserID)
	require.Equal(t, "new@example.com", changedEmail)
	require.Equal(t, now, changedAt)
	_, err = loadEmailFlow(context.Background(), iamEmailFlowKindChangeConfirm, token)
	require.ErrorIs(t, err, errEmailFlowNotFound)
}

func TestChangeConfirmCreateCanceled(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 14, 10, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{23}, 64)))
	t.Cleanup(restore)

	token, err := newEmailFlowToken()
	require.NoError(t, err)
	flow := iamEmailFlowState{
		Kind:      iamEmailFlowKindChangeConfirm,
		UserID:    "user-change-6",
		OldEmail:  "old@example.com",
		NewEmail:  "new@example.com",
		Email:     "new@example.com",
		IssuedAt:  now,
		ExpiresAt: now.Add(30 * time.Minute),
	}
	err = flowCache.Set(emailFlowKey(iamEmailFlowKindChangeConfirm, token), flow, 30*time.Minute)
	require.NoError(t, err)
	require.NoError(t, markEmailChangeCanceled(context.Background(), flow))

	SetAccountGateway(testAccountGateway{
		getByID: func(*types.ServiceContext, string) (*AccountSnapshot, error) {
			t.Fatalf("GetByID should not be called for canceled flow")
			return nil, errors.New("unexpected GetByID call")
		},
		findByEmail: func(*types.ServiceContext, string) (*AccountSnapshot, error) {
			t.Fatalf("FindByEmail should not be called for canceled flow")
			return nil, errors.New("unexpected FindByEmail call")
		},
		applyEmailChange: func(*types.ServiceContext, string, string, time.Time) error {
			t.Fatalf("ApplyEmailChange should not be called for canceled flow")
			return nil
		},
	})

	svc := &ChangeConfirmService{}
	svc.Logger = loggerzap.New("")
	ctx := types.NewServiceContext(nil, nil, consts.PHASE_CREATE)

	rsp, err := svc.Create(ctx, &modelemail.ChangeConfirmReq{Token: token})
	require.NoError(t, err)
	require.False(t, rsp.Changed)
	require.Equal(t, "email change was canceled", rsp.Msg)
}

func TestChangeCancelCreate(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 14, 11, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{24}, 64)))
	t.Cleanup(restore)

	token, err := newEmailFlowToken()
	require.NoError(t, err)
	flow := iamEmailFlowState{
		Kind:      iamEmailFlowKindChangeCancel,
		UserID:    "user-change-7",
		OldEmail:  "old@example.com",
		NewEmail:  "new@example.com",
		Email:     "old@example.com",
		IssuedAt:  now,
		ExpiresAt: now.Add(30 * time.Minute),
	}
	err = flowCache.Set(emailFlowKey(iamEmailFlowKindChangeCancel, token), flow, 30*time.Minute)
	require.NoError(t, err)

	SetAccountGateway(testAccountGateway{
		getByID: func(_ *types.ServiceContext, userID string) (*AccountSnapshot, error) {
			require.Equal(t, "user-change-7", userID)
			return testEmailAccount("user-change-7", "old@example.com", false), nil
		},
	})

	svc := &ChangeCancelService{}
	svc.Logger = loggerzap.New("")
	ctx := types.NewServiceContext(nil, nil, consts.PHASE_CREATE)

	rsp, err := svc.Create(ctx, &modelemail.ChangeCancelReq{Token: token})
	require.NoError(t, err)
	require.True(t, rsp.Canceled)
	require.Equal(t, "email change canceled successfully", rsp.Msg)

	canceled, err := emailChangeCanceled(context.Background(), flow.UserID, flow.OldEmail, flow.NewEmail)
	require.NoError(t, err)
	require.True(t, canceled)
}

func TestChangeRequestCreateClearsCancellationMarker(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 14, 12, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{25}, 64)))
	t.Cleanup(restore)

	sender := new(testEmailSender)
	activeEmailSender = sender

	oldEmail := "old@example.com"

	require.NoError(t, markEmailChangeCanceled(context.Background(), iamEmailFlowState{
		UserID:    "user-change-8",
		OldEmail:  oldEmail,
		NewEmail:  "new@example.com",
		ExpiresAt: now.Add(30 * time.Minute),
	}))

	SetAccountGateway(testAccountGateway{
		getByID: func(*types.ServiceContext, string) (*AccountSnapshot, error) {
			return testEmailAccount("user-change-8", oldEmail, false), nil
		},
		findByEmail: func(*types.ServiceContext, string) (*AccountSnapshot, error) {
			return nil, ErrAccountNotFound
		},
		verifyPassword: func(*types.ServiceContext, string, string) error {
			return nil
		},
	})

	svc := &ChangeRequestService{}
	svc.Logger = loggerzap.New("")
	ctx := newEmailServiceContext("user-change-8")

	rsp, err := svc.Create(ctx, &modelemail.ChangeRequestReq{
		NewEmail:        "new@example.com",
		CurrentPassword: "current-password",
	})
	require.NoError(t, err)
	require.Equal(t, "email change request submitted successfully", rsp.Msg)

	canceled, err := emailChangeCanceled(context.Background(), "user-change-8", oldEmail, "new@example.com")
	require.NoError(t, err)
	require.False(t, canceled)
}

func TestPasswordResetRequestCreate(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 14, 0, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{7}, 64)))
	t.Cleanup(restore)

	sender := new(testEmailSender)
	activeEmailSender = sender

	SetAccountGateway(testAccountGateway{
		findByEmail: func(_ *types.ServiceContext, email string) (*AccountSnapshot, error) {
			require.Equal(t, "user@example.com", email)
			return testEmailAccount("user-1", "user@example.com", false), nil
		},
	})

	svc := &PasswordResetRequestService{}
	svc.Logger = loggerzap.New("")
	ctx := types.NewServiceContext(nil, nil, consts.PHASE_CREATE)

	rsp, err := svc.Create(ctx, &modelemail.PasswordResetRequestReq{Email: " USER@example.com "})
	require.NoError(t, err)
	require.Equal(t, publicAcceptedMessage(iamEmailFlowKindPasswordReset), rsp.Msg)
	require.Equal(t, "user@example.com", sender.last.To)
	require.Equal(t, "Password reset", sender.last.Subject)
	require.NotEmpty(t, sender.last.Data["token"])
	require.Equal(t, "user-1", sender.last.Data["user_id"])
	require.Equal(t, 1, flowCache.Len())
}

func TestPasswordResetRequestCreateUnknownAccount(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 15, 0, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{8}, 64)))
	t.Cleanup(restore)

	sender := new(testEmailSender)
	activeEmailSender = sender

	SetAccountGateway(testAccountGateway{
		findByEmail: func(*types.ServiceContext, string) (*AccountSnapshot, error) {
			return nil, ErrAccountNotFound
		},
	})

	svc := &PasswordResetRequestService{}
	svc.Logger = loggerzap.New("")
	ctx := types.NewServiceContext(nil, nil, consts.PHASE_CREATE)

	rsp, err := svc.Create(ctx, &modelemail.PasswordResetRequestReq{Email: "user@example.com"})
	require.NoError(t, err)
	require.Equal(t, publicAcceptedMessage(iamEmailFlowKindPasswordReset), rsp.Msg)
	require.Equal(t, 0, flowCache.Len())
	require.Empty(t, sender.last.To)
}

func TestPasswordResetConfirmCreate(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 16, 0, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{9}, 64)))
	t.Cleanup(restore)

	token, err := newEmailFlowToken()
	require.NoError(t, err)
	err = flowCache.Set(emailFlowKey(iamEmailFlowKindPasswordReset, token), iamEmailFlowState{
		Kind:      iamEmailFlowKindPasswordReset,
		UserID:    "user-2",
		Email:     "user@example.com",
		IssuedAt:  now,
		ExpiresAt: now.Add(30 * time.Minute),
	}, 30*time.Minute)
	require.NoError(t, err)

	var resetUserID string
	var updatePassword string
	var invalidated string
	SetAccountGateway(testAccountGateway{
		getByID: func(_ *types.ServiceContext, userID string) (*AccountSnapshot, error) {
			require.Equal(t, "user-2", userID)
			return testEmailAccount("user-2", "user@example.com", false), nil
		},
		updatePassword: func(_ *types.ServiceContext, userID, newPassword string) error {
			resetUserID = userID
			updatePassword = newPassword
			return nil
		},
		invalidateSessions: func(userID string) { invalidated = userID },
	})

	svc := &PasswordResetConfirmService{}
	svc.Logger = loggerzap.New("")
	ctx := types.NewServiceContext(nil, nil, consts.PHASE_CREATE)

	rsp, err := svc.Create(ctx, &modelemail.PasswordResetConfirmReq{
		Token:       token,
		NewPassword: "new-password-123",
	})
	require.NoError(t, err)
	require.True(t, rsp.Reset)
	require.Equal(t, "password reset successfully", rsp.Msg)
	require.Equal(t, "user-2", resetUserID)
	require.Equal(t, "new-password-123", updatePassword)
	require.Equal(t, "user-2", invalidated)
	_, err = loadEmailFlow(context.Background(), iamEmailFlowKindPasswordReset, token)
	require.ErrorIs(t, err, errEmailFlowNotFound)
}

func TestPasswordResetConfirmCreateInvalidToken(t *testing.T) {
	flowCache := newTestCache[iamEmailFlowState]()
	throttleCache := newTestCache[emailThrottleRecord]()
	now := time.Date(2026, 3, 31, 17, 0, 0, 0, time.UTC)
	restore := stubEmailGlobals(flowCache, throttleCache, now, bytes.NewReader(bytes.Repeat([]byte{10}, 64)))
	t.Cleanup(restore)

	svc := &PasswordResetConfirmService{}
	svc.Logger = loggerzap.New("")
	ctx := types.NewServiceContext(nil, nil, consts.PHASE_CREATE)

	rsp, err := svc.Create(ctx, &modelemail.PasswordResetConfirmReq{
		Token:       "missing",
		NewPassword: "new-password-123",
	})
	require.NoError(t, err)
	require.False(t, rsp.Reset)
	require.Equal(t, "invalid or expired password reset token", rsp.Msg)
}
