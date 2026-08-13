package mfa_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/module/mfa"
	"github.com/hydroan/gst/types/consts"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

var baseURL = testutil.BaseURL()

const (
	signupPath  = "/api/signup"
	loginPath   = "/api/login"
	verifyPath  = "/api/mfa/totp/verify"
	checkPath   = "/api/mfa/totp/check"
	bindPath    = "/api/mfa/totp/bind"
	confirmPath = "/api/mfa/totp/confirm"
	unbindPath  = "/api/mfa/totp/unbind"
	statusPath  = "/api/mfa/totp/status"
)

type totpTestAccount struct {
	Username  string
	Password  string
	UserID    string
	SessionID string
}

func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Database: config.DBMySQL,
		Redis:    true,
		Register: func() {
			iam.Register()
			mfa.Register()
		},
	})
}

func TestTOTPStatus(t *testing.T) {
	account := newTOTPTestAccount(t, "totp_status_user")

	t.Run("not_enabled", func(t *testing.T) {
		resp := requestTOTPStatus(t, account.SessionID)
		rsp := testutil.DecodeResp[*mfa.TOTPStatusRsp](t, resp)
		require.Equal(t, 0, rsp.DeviceCount)
		require.Empty(t, rsp.Devices)
		require.False(t, rsp.Enabled)
		testutil.RequireDataFields(t, resp, "enabled", "device_count")
		assertResponseDataArrayField(t, resp, "devices")
	})

	deviceID, _, _ := bindTOTPDeviceForTest(t, account.SessionID, "test-device-status")

	t.Run("enabled", func(t *testing.T) {
		resp := requestTOTPStatus(t, account.SessionID)
		rsp := testutil.DecodeResp[*mfa.TOTPStatusRsp](t, resp)
		require.True(t, rsp.Enabled)
		require.NotEmpty(t, rsp.DeviceCount)
		for _, d := range rsp.Devices {
			require.NotEmpty(t, d.ID)
			require.NotEmpty(t, d.DeviceName)
			require.NotEmpty(t, d.LastUsedAt)
		}
		testutil.RequireDataFields(t, resp, "enabled", "device_count")
		assertResponseDataArrayField(t, resp, "devices")
	})

	unbindTOTPDeviceWithPassword(t, account.SessionID, deviceID, account.Password)

	t.Run("disabled_after_unbind", func(t *testing.T) {
		resp := requestTOTPStatus(t, account.SessionID)
		rsp := testutil.DecodeResp[*mfa.TOTPStatusRsp](t, resp)
		require.False(t, rsp.Enabled)
		require.Equal(t, 0, rsp.DeviceCount)
		require.Empty(t, rsp.Devices)
		testutil.RequireDataFields(t, resp, "enabled", "device_count")
		assertResponseDataArrayField(t, resp, "devices")
	})
}

func TestTOTPCheck(t *testing.T) {
	account := newTOTPTestAccount(t, "totp_check_user")

	t.Run("not_enabled", func(t *testing.T) {
		resp := requestTOTPCheck(t, account)
		rsp := testutil.DecodeResp[*mfa.TOTPCheckRsp](t, resp)
		require.False(t, rsp.RequiresMFA)
		require.NotEmpty(t, rsp.Message)
		testutil.RequireDataFields(t, resp, "requires_mfa")
	})

	bindTOTPDeviceForTest(t, account.SessionID, "test-device-check")

	t.Run("enabled", func(t *testing.T) {
		resp := requestTOTPCheck(t, account)
		rsp := testutil.DecodeResp[*mfa.TOTPCheckRsp](t, resp)
		require.True(t, rsp.RequiresMFA)
		require.NotEmpty(t, rsp.Message)
		testutil.RequireDataFields(t, resp, "requires_mfa")
	})
}

func TestTOTPBind(t *testing.T) {
	account := newTOTPTestAccount(t, "totp_bind_user")
	cli := mfaSessionClient(t, account.SessionID)

	resp, err := cli.Do(http.MethodPost, bindPath, nil)
	require.NoError(t, err)
	rsp := testutil.DecodeResp[*mfa.TOTPBindRsp](t, resp)
	require.NotNil(t, rsp)
	require.NotEmpty(t, rsp.ChallengeID)
	require.NotEmpty(t, rsp.OtpauthURL)
	require.NotEmpty(t, rsp.QRCodeImageDataURL)
	require.Equal(t, consts.FrameworkName, rsp.Issuer)
	require.Equal(t, account.Username, rsp.AccountName)
	require.NotEmpty(t, extractSecretFromOtpauthURL(t, rsp.OtpauthURL))
	testutil.RequireDataFields(t, resp, "qr_code_image_data_url")
}

func TestTOTPConfirm(t *testing.T) {
	account := newTOTPTestAccount(t, "totp_confirm_user")
	challengeID, secret := createTOTPBindingChallenge(t, account.SessionID)
	cli := mfaSessionClient(t, account.SessionID)

	t.Run("invalid_challenge", func(t *testing.T) {
		code, err := totp.GenerateCode(secret, time.Now())
		require.NoError(t, err)
		resp, err := cli.Do(http.MethodPost, confirmPath, mfa.TOTPConfirmReq{
			ChallengeID: "missing-challenge",
			Code:        code,
			DeviceName:  "test-device-missing-challenge",
		})
		require.Error(t, err)
		require.Nil(t, resp)
	})

	t.Run("invalid_code_does_not_consume_challenge", func(t *testing.T) {
		code, err := totp.GenerateCode(secret, time.Now())
		require.NoError(t, err)
		invalidCode := "000000"
		if code == invalidCode {
			invalidCode = "000001"
		}
		resp, err := cli.Do(http.MethodPost, confirmPath, mfa.TOTPConfirmReq{
			ChallengeID: challengeID,
			Code:        invalidCode,
			DeviceName:  "test-device-2",
		})
		require.Error(t, err)
		require.Nil(t, resp)

		resp, err = cli.Do(http.MethodPost, confirmPath, mfa.TOTPConfirmReq{
			ChallengeID: challengeID,
			Code:        code,
			DeviceName:  "test-device",
		})
		require.NoError(t, err)
		rsp := testutil.DecodeResp[*mfa.TOTPConfirmRsp](t, resp)
		require.NotEmpty(t, rsp.DeviceID)
		require.NotEmpty(t, rsp.Message)
		require.NotEmpty(t, rsp.BackupCodes)
		require.Len(t, rsp.BackupCodes, 10)
		for _, bc := range rsp.BackupCodes {
			require.Regexp(t, `^[23456789ABCDEFGHJKLMNPQRSTUVWXYZ]{4}(-[23456789ABCDEFGHJKLMNPQRSTUVWXYZ]{4}){3}$`, bc)
		}
		assertBackupCodeHashesStored(t, rsp.DeviceID, rsp.BackupCodes)
	})

	t.Run("duplicate_challenge", func(t *testing.T) {
		code, err := totp.GenerateCode(secret, time.Now())
		require.NoError(t, err)
		resp, err := cli.Do(http.MethodPost, confirmPath, mfa.TOTPConfirmReq{
			ChallengeID: challengeID,
			Code:        code,
			DeviceName:  "test-device-dup",
		})
		require.Error(t, err)
		require.Nil(t, resp)
	})
}

func TestTOTPVerify(t *testing.T) {
	account := newTOTPTestAccount(t, "totp_verify_user")
	deviceID, secret, _ := bindTOTPDeviceForTest(t, account.SessionID, "test-device-verify")
	cli := mfaSessionClient(t, account.SessionID)

	t.Run("valid_code", func(t *testing.T) {
		// The current period's code was consumed by confirm inside
		// bindTOTPDeviceForTest, so this request needs the next period's code.
		code := nextPeriodTOTPCode(t, secret)
		verifyStart := time.Now().UTC().Truncate(time.Millisecond)
		resp, err := cli.Do(http.MethodPost, verifyPath, mfa.TOTPVerifyReq{TOTPCode: code})
		require.NoError(t, err)
		rsp := testutil.DecodeResp[*mfa.TOTPVerifyRsp](t, resp)
		require.True(t, rsp.Valid)
		require.NotEmpty(t, rsp.Message)
		testutil.RequireDataFields(t, resp, "valid")

		// The narrowed usage write must actually land: a mistyped column name
		// would leave last_used_at untouched and fail silently otherwise.
		device := getTOTPDeviceForTest(t, deviceID)
		require.NotNil(t, device.LastUsedAt)
		require.False(t, device.LastUsedAt.Before(verifyStart),
			"verify must refresh last_used_at, got %v before %v", device.LastUsedAt, verifyStart)
	})

	t.Run("invalid_code", func(t *testing.T) {
		resp, err := cli.Do(http.MethodPost, verifyPath, mfa.TOTPVerifyReq{TOTPCode: "000000"})
		require.NoError(t, err)
		rsp := testutil.DecodeResp[*mfa.TOTPVerifyRsp](t, resp)
		require.False(t, rsp.Valid)
		require.NotEmpty(t, rsp.Message)
		testutil.RequireDataFields(t, resp, "valid")
	})

	t.Run("invalid_format", func(t *testing.T) {
		resp, err := cli.Do(http.MethodPost, verifyPath, mfa.TOTPVerifyReq{TOTPCode: "abc123"})
		require.Error(t, err)
		require.Nil(t, resp)
	})
}

func TestTOTPLogin(t *testing.T) {
	t.Skip("IAM login MFA integration is temporarily disabled.")

	account := newTOTPTestAccount(t, "totp_login_user")
	deviceID, secret, backupCodes := bindTOTPDeviceForTest(t, account.SessionID, "test-device-login")

	t.Run("requires_second_factor", func(t *testing.T) {
		cli, err := client.New(baseURL)
		require.NoError(t, err)
		resp, err := cli.Do(http.MethodPost, loginPath, iam.LoginReq{
			Username: account.Username,
			Password: account.Password,
		})
		require.Error(t, err)
		require.Nil(t, resp)
	})

	t.Run("with_totp_code", func(t *testing.T) {
		code, err := totp.GenerateCode(secret, time.Now())
		require.NoError(t, err)
		_ = loginSessionIDFromCookie(t, iam.LoginReq{
			Username: account.Username,
			Password: account.Password,
			TOTPCode: code,
		})
	})

	t.Run("rejects_conflicting_second_factors", func(t *testing.T) {
		require.NotEmpty(t, backupCodes)
		cli, err := client.New(baseURL)
		require.NoError(t, err)
		code, err := totp.GenerateCode(secret, time.Now())
		require.NoError(t, err)
		resp, err := cli.Do(http.MethodPost, loginPath, iam.LoginReq{
			Username:   account.Username,
			Password:   account.Password,
			TOTPCode:   code,
			BackupCode: backupCodes[0],
		})
		require.Error(t, err)
		require.Nil(t, resp)
		assertBackupCodeHashCount(t, deviceID, 10)
	})

	t.Run("with_backup_code", func(t *testing.T) {
		require.Len(t, backupCodes, 10)
		_ = loginSessionIDFromCookie(t, iam.LoginReq{
			Username:   account.Username,
			Password:   account.Password,
			BackupCode: backupCodes[1],
		})
		cli, err := client.New(baseURL)
		require.NoError(t, err)
		resp, err := cli.Do(http.MethodPost, loginPath, iam.LoginReq{
			Username:   account.Username,
			Password:   account.Password,
			BackupCode: backupCodes[1],
		})
		require.Error(t, err)
		require.Nil(t, resp)
		assertBackupCodeHashCount(t, deviceID, 9)
	})
}

func TestTOTPUnbind(t *testing.T) {
	account := newTOTPTestAccount(t, "totp_unbind_user")
	deviceID, secret, backupCodes := bindTOTPDeviceForTest(t, account.SessionID, "test-device")
	cli := mfaSessionClient(t, account.SessionID)

	t.Run("missing_fresh_auth", func(t *testing.T) {
		resp, err := cli.Do(http.MethodPost, unbindPath, mfa.TOTPUnbindReq{DeviceID: deviceID})
		require.NoError(t, err)
		rsp := testutil.DecodeResp[*mfa.TOTPUnbindRsp](t, resp)
		require.False(t, rsp.Success)
		require.Equal(t, 1, rsp.DeviceCount)
		require.NotEmpty(t, rsp.Message)
		testutil.RequireDataFields(t, resp, "success", "device_count")
		assertTOTPDeviceActive(t, deviceID)
	})

	t.Run("multiple_verification_methods", func(t *testing.T) {
		require.NotEmpty(t, backupCodes)
		resp, err := cli.Do(http.MethodPost, unbindPath, mfa.TOTPUnbindReq{
			DeviceID:   deviceID,
			Password:   account.Password,
			BackupCode: backupCodes[0],
		})
		require.NoError(t, err)
		rsp := testutil.DecodeResp[*mfa.TOTPUnbindRsp](t, resp)
		require.False(t, rsp.Success)
		require.Equal(t, 1, rsp.DeviceCount)
		require.NotEmpty(t, rsp.Message)
		testutil.RequireDataFields(t, resp, "success", "device_count")
		assertTOTPDeviceActive(t, deviceID)
		assertBackupCodeHashCount(t, deviceID, 10)
	})

	t.Run("invalid_totp", func(t *testing.T) {
		resp, err := cli.Do(http.MethodPost, unbindPath, mfa.TOTPUnbindReq{
			DeviceID: deviceID,
			TOTPCode: "000000",
		})
		require.NoError(t, err)
		rsp := testutil.DecodeResp[*mfa.TOTPUnbindRsp](t, resp)
		require.False(t, rsp.Success)
		require.Equal(t, 1, rsp.DeviceCount)
		require.NotEmpty(t, rsp.Message)
		testutil.RequireDataFields(t, resp, "success", "device_count")
		assertTOTPDeviceActive(t, deviceID)
	})

	t.Run("valid_password", func(t *testing.T) {
		secondDeviceID, _, _ := bindTOTPDeviceForTest(t, account.SessionID, "test-device-password")
		resp, err := cli.Do(http.MethodPost, unbindPath, mfa.TOTPUnbindReq{
			DeviceID: secondDeviceID,
			Password: account.Password,
		})
		require.NoError(t, err)
		rsp := testutil.DecodeResp[*mfa.TOTPUnbindRsp](t, resp)
		require.True(t, rsp.Success)
		require.Equal(t, 1, rsp.DeviceCount)
		require.NotEmpty(t, rsp.Message)
		testutil.RequireDataFields(t, resp, "success", "device_count")
	})

	t.Run("valid_totp", func(t *testing.T) {
		// The current period's code was consumed by confirm inside
		// bindTOTPDeviceForTest, so this request needs the next period's code.
		code := nextPeriodTOTPCode(t, secret)
		resp, err := cli.Do(http.MethodPost, unbindPath, mfa.TOTPUnbindReq{
			DeviceID: deviceID,
			TOTPCode: code,
		})
		require.NoError(t, err)
		rsp := testutil.DecodeResp[*mfa.TOTPUnbindRsp](t, resp)
		require.True(t, rsp.Success)
		require.Equal(t, 0, rsp.DeviceCount)
		require.NotEmpty(t, rsp.Message)
		testutil.RequireDataFields(t, resp, "success", "device_count")
	})
}

func TestTOTPUnbindWithBackupCode(t *testing.T) {
	account := newTOTPTestAccount(t, "totp_unbind_backup_user")
	keptDeviceID, _, backupCodes := bindTOTPDeviceForTest(t, account.SessionID, "test-device-kept")
	removedDeviceID, _, _ := bindTOTPDeviceForTest(t, account.SessionID, "test-device-removed")
	cli := mfaSessionClient(t, account.SessionID)

	// Recovery codes are matched across all of the user's devices, so a code
	// issued with the kept device can authorize unbinding the other one. The
	// narrowed consumption write must actually remove the hash: a mistyped
	// column name would leave all ten hashes in place and fail silently.
	require.Len(t, backupCodes, 10)
	resp, err := cli.Do(http.MethodPost, unbindPath, mfa.TOTPUnbindReq{
		DeviceID:   removedDeviceID,
		BackupCode: backupCodes[0],
	})
	require.NoError(t, err)
	rsp := testutil.DecodeResp[*mfa.TOTPUnbindRsp](t, resp)
	require.True(t, rsp.Success)
	require.Equal(t, 1, rsp.DeviceCount)

	assertBackupCodeHashCount(t, keptDeviceID, 9)

	// A consumed recovery code cannot be replayed for another unbind.
	resp, err = cli.Do(http.MethodPost, unbindPath, mfa.TOTPUnbindReq{
		DeviceID:   keptDeviceID,
		BackupCode: backupCodes[0],
	})
	require.NoError(t, err)
	rsp = testutil.DecodeResp[*mfa.TOTPUnbindRsp](t, resp)
	require.False(t, rsp.Success)
	require.Equal(t, 1, rsp.DeviceCount)
	assertBackupCodeHashCount(t, keptDeviceID, 9)
}

func TestTOTPVerifyReplayProtection(t *testing.T) {
	account := newTOTPTestAccount(t, "totp_replay_user")
	challengeID, secret := createTOTPBindingChallenge(t, account.SessionID)
	cli := mfaSessionClient(t, account.SessionID)

	confirmCode, err := totp.GenerateCode(secret, time.Now())
	require.NoError(t, err)
	confirmRsp, err := client.Post[mfa.TOTPConfirmRsp](cli, confirmPath, mfa.TOTPConfirmReq{
		ChallengeID: challengeID,
		Code:        confirmCode,
		DeviceName:  "test-device-replay",
	})
	require.NoError(t, err)
	require.NotEmpty(t, confirmRsp.DeviceID)

	t.Run("code_consumed_by_confirm_is_rejected", func(t *testing.T) {
		resp, err := cli.Do(http.MethodPost, verifyPath, mfa.TOTPVerifyReq{TOTPCode: confirmCode})
		require.NoError(t, err)
		rsp := testutil.DecodeResp[*mfa.TOTPVerifyRsp](t, resp)
		require.False(t, rsp.Valid)
	})

	t.Run("fresh_code_verifies_once_then_rejected", func(t *testing.T) {
		code := nextPeriodTOTPCode(t, secret)

		resp, err := cli.Do(http.MethodPost, verifyPath, mfa.TOTPVerifyReq{TOTPCode: code})
		require.NoError(t, err)
		rsp := testutil.DecodeResp[*mfa.TOTPVerifyRsp](t, resp)
		require.True(t, rsp.Valid)

		resp, err = cli.Do(http.MethodPost, verifyPath, mfa.TOTPVerifyReq{TOTPCode: code})
		require.NoError(t, err)
		rsp = testutil.DecodeResp[*mfa.TOTPVerifyRsp](t, resp)
		require.False(t, rsp.Valid)
	})
}

func TestTOTPConfirmConcurrentDuplicate(t *testing.T) {
	account := newTOTPTestAccount(t, "totp_concurrent_user")
	challengeID, secret := createTOTPBindingChallenge(t, account.SessionID)

	code, err := totp.GenerateCode(secret, time.Now())
	require.NoError(t, err)

	// Clients are built on the main goroutine because the session helper
	// asserts through t, which must not run on spawned goroutines.
	clients := []*client.Client{
		mfaSessionClient(t, account.SessionID),
		mfaSessionClient(t, account.SessionID),
	}
	results := make([]error, len(clients))
	var wg sync.WaitGroup
	for i, cli := range clients {
		wg.Add(1)
		go func(i int, cli *client.Client) {
			defer wg.Done()
			_, results[i] = cli.Do(http.MethodPost, confirmPath, mfa.TOTPConfirmReq{
				ChallengeID: challengeID,
				Code:        code,
				DeviceName:  fmt.Sprintf("test-device-concurrent-%d", i),
			})
		}(i, cli)
	}
	wg.Wait()

	succeeded := 0
	for _, resultErr := range results {
		if resultErr == nil {
			succeeded++
		}
	}
	require.Equal(t, 1, succeeded, "exactly one concurrent confirm must win: %v", results)

	devices := make([]*modelmfa.TOTPDevice, 0)
	require.NoError(t, database.Database[*modelmfa.TOTPDevice](context.Background()).
		WithQuery(&modelmfa.TOTPDevice{UserID: account.UserID}).
		List(&devices))
	require.Len(t, devices, 1)
}

func TestTOTPDeviceUniqueSecretIndex(t *testing.T) {
	ctx := context.Background()
	secret := "UNIQUEINDEXSECRETUNIQUEINDEXSECRETUNIQUEINDEXSECRETA"

	first := &modelmfa.TOTPDevice{
		UserID:     "unique-index-user-1",
		DeviceName: "device-a",
		Secret:     secret,
		IsActive:   true,
	}
	require.NoError(t, database.Database[*modelmfa.TOTPDevice](ctx).Create(first))
	t.Cleanup(func() {
		_ = database.Database[*modelmfa.TOTPDevice](ctx).WithPurge(true).Delete(first)
	})

	duplicate := &modelmfa.TOTPDevice{
		UserID:     "unique-index-user-1",
		DeviceName: "device-b",
		Secret:     secret,
		IsActive:   true,
	}
	require.Error(t, database.Database[*modelmfa.TOTPDevice](ctx).Create(duplicate),
		"binding the same secret twice for one user must hit the unique index")

	otherUser := &modelmfa.TOTPDevice{
		UserID:     "unique-index-user-2",
		DeviceName: "device-c",
		Secret:     secret,
		IsActive:   true,
	}
	require.NoError(t, database.Database[*modelmfa.TOTPDevice](ctx).Create(otherUser),
		"the unique index scopes secrets per user, not globally")
	t.Cleanup(func() {
		_ = database.Database[*modelmfa.TOTPDevice](ctx).WithPurge(true).Delete(otherUser)
	})
}

func TestTOTPVerificationRateLimit(t *testing.T) {
	account := newTOTPTestAccount(t, "totp_ratelimit_user")
	cli := mfaSessionClient(t, account.SessionID)

	// The user has no bound device: every attempt fails fast but still spends
	// rate budget, because throttling runs before the handler.
	for range 5 {
		_, err := cli.Do(http.MethodPost, verifyPath, mfa.TOTPVerifyReq{TOTPCode: "000000"})
		testutil.RequireError(t, err, http.StatusBadRequest)
	}

	_, err := cli.Do(http.MethodPost, verifyPath, mfa.TOTPVerifyReq{TOTPCode: "000000"})
	testutil.RequireError(t, err, http.StatusTooManyRequests, "too many requests")

	// Other throttled endpoints keep their own budget: the same user's next
	// unbind attempt is judged by the handler, not the limiter.
	resp, err := cli.Do(http.MethodPost, unbindPath, mfa.TOTPUnbindReq{DeviceID: "missing-device"})
	require.NoError(t, err)
	rsp := testutil.DecodeResp[*mfa.TOTPUnbindRsp](t, resp)
	require.False(t, rsp.Success)
}

func newTOTPTestAccount(t *testing.T, prefix string) totpTestAccount {
	t.Helper()

	account := totpTestAccount{
		Username: fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano()),
		Password: "12345678",
	}

	cli, err := client.New(baseURL)
	require.NoError(t, err)
	rsp, err := client.Post[iam.SignupRsp](cli, signupPath, iam.SignupReq{
		Username:   account.Username,
		Password:   account.Password,
		RePassword: account.Password,
	})
	require.NoError(t, err)
	require.Equal(t, account.Username, rsp.Username)
	require.NotEmpty(t, rsp.UserID)
	require.NotEmpty(t, rsp.Message)
	account.UserID = rsp.UserID

	account.SessionID = loginSessionIDFromCookie(t, iam.LoginReq{
		Username: account.Username,
		Password: account.Password,
	})
	return account
}

// mfaSessionClient returns a client that presents the given session id.
func mfaSessionClient(t *testing.T, sessionID string) *client.Client {
	t.Helper()

	cli, err := client.New(baseURL, client.WithCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	}))
	require.NoError(t, err)
	return cli
}

func requestTOTPStatus(t *testing.T, sessionID string) *client.Envelope {
	t.Helper()

	cli := mfaSessionClient(t, sessionID)
	resp, err := cli.Do(http.MethodGet, statusPath, nil)
	require.NoError(t, err)
	return resp
}

func requestTOTPCheck(t *testing.T, account totpTestAccount) *client.Envelope {
	t.Helper()

	cli := mfaSessionClient(t, account.SessionID)
	resp, err := cli.Do(http.MethodPost, checkPath, mfa.TOTPCheckReq{
		Username: account.Username,
		Password: account.Password,
	})
	require.NoError(t, err)
	return resp
}

func createTOTPBindingChallenge(t *testing.T, sessionID string) (string, string) {
	t.Helper()

	cli := mfaSessionClient(t, sessionID)
	rsp, err := client.Post[mfa.TOTPBindRsp](cli, bindPath, nil)
	require.NoError(t, err)
	require.NotEmpty(t, rsp.ChallengeID)
	require.NotEmpty(t, rsp.OtpauthURL)
	return rsp.ChallengeID, extractSecretFromOtpauthURL(t, rsp.OtpauthURL)
}

func unbindTOTPDeviceWithPassword(t *testing.T, sessionID, deviceID, password string) {
	t.Helper()

	cli := mfaSessionClient(t, sessionID)
	rsp, err := client.Post[mfa.TOTPUnbindRsp](cli, unbindPath, mfa.TOTPUnbindReq{
		DeviceID: deviceID,
		Password: password,
	})
	require.NoError(t, err)
	require.True(t, rsp.Success)
	require.NotEmpty(t, rsp.Message)
}

func loginSessionIDFromCookie(t *testing.T, reqPayload iam.LoginReq) string {
	t.Helper()

	cli, err := client.New(baseURL)
	require.NoError(t, err)

	apiResp, err := cli.Do(http.MethodPost, loginPath, reqPayload)
	require.NoError(t, err)

	rsp := testutil.DecodeResp[iam.LoginRsp](t, apiResp)
	require.False(t, rsp.ServerTime.IsZero())
	require.False(t, rsp.Session.ExpiresAt.IsZero())

	var data map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(apiResp.Data, &data), "response data: %s", string(apiResp.Data))
	require.NotContains(t, data, "session_id")

	cookie := apiResp.Cookie("session_id")
	require.NotNil(t, cookie, "session cookie not found")
	require.NotEmpty(t, cookie.Value)
	require.Regexp(t, `^[0-9a-f]{64}$`, cookie.Value)
	return cookie.Value
}

func extractSecretFromOtpauthURL(t *testing.T, otpauthURL string) string {
	t.Helper()

	key, err := otp.NewKeyFromURL(otpauthURL)
	require.NoError(t, err)
	require.NotEmpty(t, key.Secret())

	return key.Secret()
}

func assertBackupCodeHashesStored(t *testing.T, deviceID string, backupCodes []string) {
	t.Helper()

	device := getTOTPDeviceForTest(t, deviceID)
	require.Len(t, device.BackupCodeHashes, len(backupCodes))
	for i, code := range backupCodes {
		normalizedCode := normalizeBackupCodeForTest(code)
		require.NotEqual(t, code, device.BackupCodeHashes[i])
		require.NotEqual(t, normalizedCode, device.BackupCodeHashes[i])
		require.NoError(t, bcrypt.CompareHashAndPassword([]byte(device.BackupCodeHashes[i]), []byte(normalizedCode)))
	}
}

func assertBackupCodeHashCount(t *testing.T, deviceID string, want int) {
	t.Helper()

	device := getTOTPDeviceForTest(t, deviceID)
	require.Len(t, device.BackupCodeHashes, want)
}

func assertTOTPDeviceActive(t *testing.T, deviceID string) {
	t.Helper()

	device := getTOTPDeviceForTest(t, deviceID)
	require.True(t, device.IsActive)
}

func bindTOTPDeviceForTest(t *testing.T, sessionID, deviceName string) (string, string, []string) {
	t.Helper()

	cli := mfaSessionClient(t, sessionID)

	bindRsp, err := client.Post[mfa.TOTPBindRsp](cli, bindPath, nil)
	require.NoError(t, err)
	require.NotEmpty(t, bindRsp.ChallengeID)
	require.NotEmpty(t, bindRsp.OtpauthURL)
	secret := extractSecretFromOtpauthURL(t, bindRsp.OtpauthURL)

	code, err := totp.GenerateCode(secret, time.Now())
	require.NoError(t, err)

	confirmRsp, err := client.Post[mfa.TOTPConfirmRsp](cli, confirmPath, mfa.TOTPConfirmReq{
		ChallengeID: bindRsp.ChallengeID,
		Code:        code,
		DeviceName:  deviceName,
	})
	require.NoError(t, err)
	require.NotEmpty(t, confirmRsp.DeviceID)
	require.NotEmpty(t, confirmRsp.BackupCodes)

	return confirmRsp.DeviceID, secret, confirmRsp.BackupCodes
}

// nextPeriodTOTPCode returns a code from the next TOTP period. Replay
// protection consumes the current period's code at confirm time, so follow-up
// requests inside the same test period need the adjacent code, which
// Validate's default skew of one period still accepts.
func nextPeriodTOTPCode(t *testing.T, secret string) string {
	t.Helper()

	code, err := totp.GenerateCode(secret, time.Now().Add(30*time.Second))
	require.NoError(t, err)
	return code
}

func getTOTPDeviceForTest(t *testing.T, deviceID string) *modelmfa.TOTPDevice {
	t.Helper()

	device := new(modelmfa.TOTPDevice)
	require.NoError(t, database.Database[*modelmfa.TOTPDevice](context.Background()).Get(device, deviceID))
	return device
}

func normalizeBackupCodeForTest(code string) string {
	code = strings.TrimSpace(code)
	code = strings.ReplaceAll(code, "-", "")
	return strings.ToUpper(code)
}

// assertResponseDataArrayField asserts that the named data field is a JSON
// array and not null, which RequireDataFields alone does not cover.
func assertResponseDataArrayField(t *testing.T, resp *client.Envelope, field string) {
	t.Helper()

	data := responseDataMap(t, resp)
	raw, ok := data[field]
	require.True(t, ok, "response data: %s", string(resp.Data))
	require.NotEqual(t, "null", strings.TrimSpace(string(raw)), "response data: %s", string(resp.Data))
	var values []json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &values), "response data: %s", string(resp.Data))
	require.NotNil(t, values, "response data: %s", string(resp.Data))
}

func responseDataMap(t *testing.T, resp *client.Envelope) map[string]json.RawMessage {
	t.Helper()

	require.NotNil(t, resp)
	var data map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(resp.Data, &data), "response data: %s", string(resp.Data))
	return data
}
