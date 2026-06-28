package mfa_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/bootstrap"
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

var (
	token = "-"
	port  = testutil.SetupRandomServerPort()

	signupAPI  = testutil.URL(port, "/api/signup")
	loginAPI   = testutil.URL(port, "/api/login")
	verifyAPI  = testutil.URL(port, "/api/mfa/totp/verify")
	checkAPI   = testutil.URL(port, "/api/mfa/totp/check")
	bindAPI    = testutil.URL(port, "/api/mfa/totp/bind")
	confirmAPI = testutil.URL(port, "/api/mfa/totp/confirm")
	unbindAPI  = testutil.URL(port, "/api/mfa/totp/unbind")
	statusAPI  = testutil.URL(port, "/api/mfa/totp/status")
)

type ListResponse[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

type totpTestAccount struct {
	Username  string
	Password  string
	UserID    string
	SessionID string
}

func init() {
	os.Setenv(config.DATABASE_TYPE, string(config.DBSqlite))
	os.Setenv(config.SQLITE_IS_MEMORY, "true")
	os.Setenv(config.REDIS_ENABLE, "true")
	testutil.SetupRandomRedisNamespace()
	os.Setenv(config.LOGGER_DIR, "./logs")
	os.Setenv(config.AUTH_NONE_EXPIRE_TOKEN, token)
	// Enable audit and sync write before Bootstrap so operationlog test can list logs immediately.
	os.Setenv(config.AUDIT_ENABLE, "true")
	os.Setenv(config.AUDIT_ASYNC_WRITE, "false")

	if err := bootstrap.Bootstrap(); err != nil {
		panic(err)
	}

	go func() {
		iam.Register()
		mfa.Register()

		if err := bootstrap.Run(); err != nil {
			panic(err)
		}
	}()

	testutil.MustWaitForServer(port)
}

func TestTOTPStatus(t *testing.T) {
	account := newTOTPTestAccount(t, "totp_status_user")

	t.Run("not_enabled", func(t *testing.T) {
		resp := requestTOTPStatus(t, account.SessionID)
		testutil.TestResp[*mfa.TOTPStatusRsp](t, resp, func(t *testing.T, rsp *mfa.TOTPStatusRsp) {
			t.Helper()
			require.Equal(t, 0, rsp.DeviceCount)
			require.Empty(t, rsp.Devices)
			require.False(t, rsp.Enabled)
		})
		assertResponseDataFieldExists(t, resp, "enabled")
		assertResponseDataFieldExists(t, resp, "device_count")
		assertResponseDataArrayField(t, resp, "devices")
	})

	deviceID, _, _ := bindTOTPDeviceForTest(t, account.SessionID, "test-device-status")

	t.Run("enabled", func(t *testing.T) {
		resp := requestTOTPStatus(t, account.SessionID)
		testutil.TestResp[*mfa.TOTPStatusRsp](t, resp, func(t *testing.T, rsp *mfa.TOTPStatusRsp) {
			t.Helper()
			require.True(t, rsp.Enabled)
			require.NotEmpty(t, rsp.DeviceCount)
			for _, d := range rsp.Devices {
				require.NotEmpty(t, d.ID)
				require.NotEmpty(t, d.DeviceName)
				require.NotEmpty(t, d.LastUsedAt)
			}
		})
		assertResponseDataFieldExists(t, resp, "enabled")
		assertResponseDataFieldExists(t, resp, "device_count")
		assertResponseDataArrayField(t, resp, "devices")
	})

	unbindTOTPDeviceWithPassword(t, account.SessionID, deviceID, account.Password)

	t.Run("disabled_after_unbind", func(t *testing.T) {
		resp := requestTOTPStatus(t, account.SessionID)
		testutil.TestResp[*mfa.TOTPStatusRsp](t, resp, func(t *testing.T, rsp *mfa.TOTPStatusRsp) {
			t.Helper()
			require.False(t, rsp.Enabled)
			require.Equal(t, 0, rsp.DeviceCount)
			require.Empty(t, rsp.Devices)
		})
		assertResponseDataFieldExists(t, resp, "enabled")
		assertResponseDataFieldExists(t, resp, "device_count")
		assertResponseDataArrayField(t, resp, "devices")
	})
}

func TestTOTPCheck(t *testing.T) {
	account := newTOTPTestAccount(t, "totp_check_user")

	t.Run("not_enabled", func(t *testing.T) {
		resp := requestTOTPCheck(t, account)
		testutil.TestResp[*mfa.TOTPCheckRsp](t, resp, func(t *testing.T, rsp *mfa.TOTPCheckRsp) {
			t.Helper()
			require.False(t, rsp.RequiresMFA)
			require.NotEmpty(t, rsp.Message)
		})
		assertResponseDataFieldExists(t, resp, "requires_mfa")
	})

	bindTOTPDeviceForTest(t, account.SessionID, "test-device-check")

	t.Run("enabled", func(t *testing.T) {
		resp := requestTOTPCheck(t, account)
		testutil.TestResp[*mfa.TOTPCheckRsp](t, resp, func(t *testing.T, rsp *mfa.TOTPCheckRsp) {
			t.Helper()
			require.True(t, rsp.RequiresMFA)
			require.NotEmpty(t, rsp.Message)
		})
		assertResponseDataFieldExists(t, resp, "requires_mfa")
	})
}

func TestTOTPBind(t *testing.T) {
	account := newTOTPTestAccount(t, "totp_bind_user")
	cli := newTOTPClient(t, bindAPI, account.SessionID)

	resp, err := cli.Create(nil)
	require.NoError(t, err)
	testutil.TestResp(t, resp, func(t *testing.T, rsp *mfa.TOTPBindRsp) {
		t.Helper()
		require.NotNil(t, rsp)
		require.NotEmpty(t, rsp.ChallengeID)
		require.NotEmpty(t, rsp.OtpauthURL)
		require.NotEmpty(t, rsp.QRCodeImageDataURL)
		require.Equal(t, consts.FrameworkName, rsp.Issuer)
		require.Equal(t, account.Username, rsp.AccountName)
		require.NotEmpty(t, extractSecretFromOtpauthURL(t, rsp.OtpauthURL))
	})
	assertResponseDataFieldExists(t, resp, "qr_code_image_data_url")
}

func TestTOTPConfirm(t *testing.T) {
	account := newTOTPTestAccount(t, "totp_confirm_user")
	challengeID, secret := createTOTPBindingChallenge(t, account.SessionID)
	cli := newTOTPClient(t, confirmAPI, account.SessionID)

	t.Run("invalid_challenge", func(t *testing.T) {
		code, err := totp.GenerateCode(secret, time.Now())
		require.NoError(t, err)
		resp, err := cli.Create(mfa.TOTPConfirmReq{
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
		resp, err := cli.Create(mfa.TOTPConfirmReq{
			ChallengeID: challengeID,
			Code:        invalidCode,
			DeviceName:  "test-device-2",
		})
		require.Error(t, err)
		require.Nil(t, resp)

		resp, err = cli.Create(mfa.TOTPConfirmReq{
			ChallengeID: challengeID,
			Code:        code,
			DeviceName:  "test-device",
		})
		require.NoError(t, err)
		testutil.TestResp(t, resp, func(t *testing.T, rsp *mfa.TOTPConfirmRsp) {
			t.Helper()
			require.NotEmpty(t, rsp.DeviceID)
			require.NotEmpty(t, rsp.Message)
			require.NotEmpty(t, rsp.BackupCodes)
			require.Len(t, rsp.BackupCodes, 10)
			for _, bc := range rsp.BackupCodes {
				require.Regexp(t, `^[23456789ABCDEFGHJKLMNPQRSTUVWXYZ]{4}(-[23456789ABCDEFGHJKLMNPQRSTUVWXYZ]{4}){3}$`, bc)
			}
			assertBackupCodeHashesStored(t, rsp.DeviceID, rsp.BackupCodes)
		})
	})

	t.Run("duplicate_challenge", func(t *testing.T) {
		code, err := totp.GenerateCode(secret, time.Now())
		require.NoError(t, err)
		resp, err := cli.Create(mfa.TOTPConfirmReq{
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
	_, secret, _ := bindTOTPDeviceForTest(t, account.SessionID, "test-device-verify")
	cli := newTOTPClient(t, verifyAPI, account.SessionID)

	t.Run("valid_code", func(t *testing.T) {
		code, err := totp.GenerateCode(secret, time.Now())
		require.NoError(t, err)
		resp, err := cli.Create(mfa.TOTPVerifyReq{TOTPCode: code})
		require.NoError(t, err)
		testutil.TestResp(t, resp, func(t *testing.T, rsp *mfa.TOTPVerifyRsp) {
			t.Helper()
			require.True(t, rsp.Valid)
			require.NotEmpty(t, rsp.Message)
		})
		assertResponseDataFieldExists(t, resp, "valid")
	})

	t.Run("invalid_code", func(t *testing.T) {
		resp, err := cli.Create(mfa.TOTPVerifyReq{TOTPCode: "000000"})
		require.NoError(t, err)
		testutil.TestResp(t, resp, func(t *testing.T, rsp *mfa.TOTPVerifyRsp) {
			t.Helper()
			require.False(t, rsp.Valid)
			require.NotEmpty(t, rsp.Message)
		})
		assertResponseDataFieldExists(t, resp, "valid")
	})

	t.Run("invalid_format", func(t *testing.T) {
		resp, err := cli.Create(mfa.TOTPVerifyReq{TOTPCode: "abc123"})
		require.Error(t, err)
		require.Nil(t, resp)
	})
}

func TestTOTPLogin(t *testing.T) {
	account := newTOTPTestAccount(t, "totp_login_user")
	deviceID, secret, backupCodes := bindTOTPDeviceForTest(t, account.SessionID, "test-device-login")

	t.Run("requires_second_factor", func(t *testing.T) {
		cli, err := client.New(loginAPI)
		require.NoError(t, err)
		resp, err := cli.Create(iam.LoginReq{
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
		cli, err := client.New(loginAPI)
		require.NoError(t, err)
		code, err := totp.GenerateCode(secret, time.Now())
		require.NoError(t, err)
		resp, err := cli.Create(iam.LoginReq{
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
		cli, err := client.New(loginAPI)
		require.NoError(t, err)
		resp, err := cli.Create(iam.LoginReq{
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
	cli := newTOTPClient(t, unbindAPI, account.SessionID)

	t.Run("missing_fresh_auth", func(t *testing.T) {
		resp, err := cli.Create(mfa.TOTPUnbindReq{DeviceID: deviceID})
		require.NoError(t, err)
		testutil.TestResp(t, resp, func(t *testing.T, rsp *mfa.TOTPUnbindRsp) {
			t.Helper()
			require.False(t, rsp.Success)
			require.Equal(t, 1, rsp.DeviceCount)
			require.NotEmpty(t, rsp.Message)
		})
		assertResponseDataFieldExists(t, resp, "success")
		assertResponseDataFieldExists(t, resp, "device_count")
		assertTOTPDeviceActive(t, deviceID)
	})

	t.Run("multiple_verification_methods", func(t *testing.T) {
		require.NotEmpty(t, backupCodes)
		resp, err := cli.Create(mfa.TOTPUnbindReq{
			DeviceID:   deviceID,
			Password:   account.Password,
			BackupCode: backupCodes[0],
		})
		require.NoError(t, err)
		testutil.TestResp(t, resp, func(t *testing.T, rsp *mfa.TOTPUnbindRsp) {
			t.Helper()
			require.False(t, rsp.Success)
			require.Equal(t, 1, rsp.DeviceCount)
			require.NotEmpty(t, rsp.Message)
		})
		assertResponseDataFieldExists(t, resp, "success")
		assertResponseDataFieldExists(t, resp, "device_count")
		assertTOTPDeviceActive(t, deviceID)
		assertBackupCodeHashCount(t, deviceID, 10)
	})

	t.Run("invalid_totp", func(t *testing.T) {
		resp, err := cli.Create(mfa.TOTPUnbindReq{
			DeviceID: deviceID,
			TOTPCode: "000000",
		})
		require.NoError(t, err)
		testutil.TestResp(t, resp, func(t *testing.T, rsp *mfa.TOTPUnbindRsp) {
			t.Helper()
			require.False(t, rsp.Success)
			require.Equal(t, 1, rsp.DeviceCount)
			require.NotEmpty(t, rsp.Message)
		})
		assertResponseDataFieldExists(t, resp, "success")
		assertResponseDataFieldExists(t, resp, "device_count")
		assertTOTPDeviceActive(t, deviceID)
	})

	t.Run("valid_password", func(t *testing.T) {
		secondDeviceID, _, _ := bindTOTPDeviceForTest(t, account.SessionID, "test-device-password")
		resp, err := cli.Create(mfa.TOTPUnbindReq{
			DeviceID: secondDeviceID,
			Password: account.Password,
		})
		require.NoError(t, err)
		testutil.TestResp(t, resp, func(t *testing.T, rsp *mfa.TOTPUnbindRsp) {
			t.Helper()
			require.True(t, rsp.Success)
			require.Equal(t, 1, rsp.DeviceCount)
			require.NotEmpty(t, rsp.Message)
		})
		assertResponseDataFieldExists(t, resp, "success")
		assertResponseDataFieldExists(t, resp, "device_count")
	})

	t.Run("valid_totp", func(t *testing.T) {
		code, err := totp.GenerateCode(secret, time.Now())
		require.NoError(t, err)
		resp, err := cli.Create(mfa.TOTPUnbindReq{
			DeviceID: deviceID,
			TOTPCode: code,
		})
		require.NoError(t, err)
		testutil.TestResp(t, resp, func(t *testing.T, rsp *mfa.TOTPUnbindRsp) {
			t.Helper()
			require.True(t, rsp.Success)
			require.Equal(t, 0, rsp.DeviceCount)
			require.NotEmpty(t, rsp.Message)
		})
		assertResponseDataFieldExists(t, resp, "success")
		assertResponseDataFieldExists(t, resp, "device_count")
	})
}

func newTOTPTestAccount(t *testing.T, prefix string) totpTestAccount {
	t.Helper()

	account := totpTestAccount{
		Username: fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano()),
		Password: "12345678",
	}

	cli, err := client.New(signupAPI)
	require.NoError(t, err)
	resp, err := cli.Create(iam.SignupReq{
		Username:   account.Username,
		Password:   account.Password,
		RePassword: account.Password,
	})
	require.NoError(t, err)
	testutil.TestResp(t, resp, func(t *testing.T, rsp iam.SignupRsp) {
		t.Helper()
		require.Equal(t, account.Username, rsp.Username)
		require.NotEmpty(t, rsp.UserID)
		require.NotEmpty(t, rsp.Message)
		account.UserID = rsp.UserID
	})

	account.SessionID = loginSessionIDFromCookie(t, iam.LoginReq{
		Username: account.Username,
		Password: account.Password,
	})
	return account
}

func newTOTPClient(t *testing.T, api, sessionID string) *client.Client {
	t.Helper()

	cli, err := client.New(api, client.WithCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	}))
	require.NoError(t, err)
	return cli
}

func requestTOTPStatus(t *testing.T, sessionID string) *client.Resp {
	t.Helper()

	cli := newTOTPClient(t, statusAPI, sessionID)
	resp, err := cli.Request(http.MethodGet, nil)
	require.NoError(t, err)
	return resp
}

func requestTOTPCheck(t *testing.T, account totpTestAccount) *client.Resp {
	t.Helper()

	cli := newTOTPClient(t, checkAPI, account.SessionID)
	resp, err := cli.Create(mfa.TOTPCheckReq{
		Username: account.Username,
		Password: account.Password,
	})
	require.NoError(t, err)
	return resp
}

func createTOTPBindingChallenge(t *testing.T, sessionID string) (string, string) {
	t.Helper()

	cli := newTOTPClient(t, bindAPI, sessionID)
	resp, err := cli.Create(mfa.TOTPBind{})
	require.NoError(t, err)

	var challengeID string
	var secret string
	testutil.TestResp(t, resp, func(t *testing.T, rsp *mfa.TOTPBindRsp) {
		t.Helper()
		require.NotEmpty(t, rsp.ChallengeID)
		require.NotEmpty(t, rsp.OtpauthURL)
		challengeID = rsp.ChallengeID
		secret = extractSecretFromOtpauthURL(t, rsp.OtpauthURL)
	})
	return challengeID, secret
}

func unbindTOTPDeviceWithPassword(t *testing.T, sessionID, deviceID, password string) {
	t.Helper()

	cli := newTOTPClient(t, unbindAPI, sessionID)
	resp, err := cli.Create(mfa.TOTPUnbindReq{
		DeviceID: deviceID,
		Password: password,
	})
	require.NoError(t, err)
	testutil.TestResp(t, resp, func(t *testing.T, rsp *mfa.TOTPUnbindRsp) {
		t.Helper()
		require.True(t, rsp.Success)
		require.NotEmpty(t, rsp.Message)
	})
}

func loginSessionIDFromCookie(t *testing.T, reqPayload iam.LoginReq) string {
	t.Helper()

	cli, err := client.New(loginAPI)
	require.NoError(t, err)

	apiResp, err := cli.Create(reqPayload)
	require.NoError(t, err)

	testutil.TestResp(t, apiResp, func(t *testing.T, rsp iam.LoginRsp) {
		t.Helper()
		require.False(t, rsp.ServerTime.IsZero())
		require.False(t, rsp.Session.ExpiresAt.IsZero())
	})

	var data map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(apiResp.Data, &data), "response data: %s", string(apiResp.Data))
	require.NotContains(t, data, "session_id")

	for _, cookie := range apiResp.Cookies {
		if cookie.Name != "session_id" {
			continue
		}
		require.NotEmpty(t, cookie.Value)
		require.Regexp(t, `^[0-9a-f]{64}$`, cookie.Value)
		return cookie.Value
	}

	require.FailNow(t, "session cookie not found")
	return ""
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

	bindCli, err := client.New(bindAPI, client.WithCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	}))
	require.NoError(t, err)

	bindResp, err := bindCli.Create(mfa.TOTPBind{})
	require.NoError(t, err)

	var challengeID string
	var secret string
	testutil.TestResp(t, bindResp, func(t *testing.T, rsp *mfa.TOTPBindRsp) {
		t.Helper()

		require.NotEmpty(t, rsp.ChallengeID)
		require.NotEmpty(t, rsp.OtpauthURL)
		challengeID = rsp.ChallengeID
		secret = extractSecretFromOtpauthURL(t, rsp.OtpauthURL)
	})

	code, err := totp.GenerateCode(secret, time.Now())
	require.NoError(t, err)

	confirmCli, err := client.New(confirmAPI, client.WithCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	}))
	require.NoError(t, err)

	confirmResp, err := confirmCli.Create(mfa.TOTPConfirmReq{
		ChallengeID: challengeID,
		Code:        code,
		DeviceName:  deviceName,
	})
	require.NoError(t, err)

	var deviceID string
	var backupCodes []string
	testutil.TestResp(t, confirmResp, func(t *testing.T, rsp *mfa.TOTPConfirmRsp) {
		t.Helper()

		require.NotEmpty(t, rsp.DeviceID)
		require.NotEmpty(t, rsp.BackupCodes)
		deviceID = rsp.DeviceID
		backupCodes = rsp.BackupCodes
	})

	return deviceID, secret, backupCodes
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

func assertResponseDataFieldExists(t *testing.T, resp *client.Resp, field string) {
	t.Helper()

	data := responseDataMap(t, resp)
	require.Contains(t, data, field, "response data: %s", string(resp.Data))
}

func assertResponseDataArrayField(t *testing.T, resp *client.Resp, field string) {
	t.Helper()

	data := responseDataMap(t, resp)
	raw, ok := data[field]
	require.True(t, ok, "response data: %s", string(resp.Data))
	require.NotEqual(t, "null", strings.TrimSpace(string(raw)), "response data: %s", string(resp.Data))
	var values []json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &values), "response data: %s", string(resp.Data))
	require.NotNil(t, values, "response data: %s", string(resp.Data))
}

func responseDataMap(t *testing.T, resp *client.Resp) map[string]json.RawMessage {
	t.Helper()

	require.NotNil(t, resp)
	var data map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(resp.Data, &data), "response data: %s", string(resp.Data))
	return data
}
