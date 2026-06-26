package mfa_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/bootstrap"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/helper"
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	"github.com/hydroan/gst/model"
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
	port  = 8000

	signupAPI  = fmt.Sprintf("http://localhost:%d/api/signup", port)
	loginAPI   = fmt.Sprintf("http://localhost:%d/api/login", port)
	verifyAPI  = fmt.Sprintf("http://localhost:%d/api/mfa/totp/verify", port)
	checkAPI   = fmt.Sprintf("http://localhost:%d/api/mfa/totp/check", port)
	bindAPI    = fmt.Sprintf("http://localhost:%d/api/mfa/totp/bind", port)
	confirmAPI = fmt.Sprintf("http://localhost:%d/api/mfa/totp/confirm", port)
	unbindAPI  = fmt.Sprintf("http://localhost:%d/api/mfa/totp/unbind", port)
	statusAPI  = fmt.Sprintf("http://localhost:%d/api/mfa/totp/status", port)
)

type ListResponse[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

func init() {
	os.Setenv(config.DATABASE_TYPE, string(config.DBSqlite))
	os.Setenv(config.SQLITE_IS_MEMORY, "true")
	os.Setenv(config.SERVER_PORT, strconv.Itoa(port))
	os.Setenv(config.REDIS_ENABLE, "true")
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

	for {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			l.Close()
			time.Sleep(1 * time.Second)
			continue
		}
		if errors.Is(err, syscall.EADDRINUSE) {
			break
		}
		panic(err)

	}
}

func TestTOTP(t *testing.T) {
	username := "user01"
	password := "12345678"
	userID := ""
	sessionID := ""
	secret := ""
	challengeID := ""
	deviceID := ""
	var backupCodes []string

	_, _, _, _, _, _, _ = sessionID, password, userID, secret, challengeID, deviceID, backupCodes

	t.Run("signup", func(t *testing.T) {
		cli, err := client.New(signupAPI)
		require.NoError(t, err)

		resp, err := cli.Create(iam.SignupReq{
			Username:   username,
			Password:   password,
			RePassword: password,
		})
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp iam.SignupRsp) {
			t.Helper(
			// #modeliam.SignupRsp {
			//   +UserID   => "019cbc8e-0659-7989-b112-12e889ef4f21" #string
			//   +Username => "user01" #string
			//   +Message  => "User created successfully" #string
			// }
			)

			require.Equal(t, rsp.Username, username)
			require.NotEmpty(t, rsp.UserID)
			require.NotEmpty(t, rsp.Message)
			userID = rsp.UserID
		})
	})

	t.Run("login", func(t *testing.T) {
		sessionID = loginSessionIDFromCookie(t, iam.LoginReq{
			Username: username,
			Password: password,
		})
	})

	t.Run("status_not_enabled", func(t *testing.T) {
		cli, err := client.New(statusAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: sessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Request(http.MethodGet, nil)
		require.NoError(t, err)
		helper.TestResp[*mfa.TOTPStatusRsp](t, resp, func(t *testing.T, rsp *mfa.TOTPStatusRsp) {
			t.Helper(
			// #*modelmfa.TOTPStatusRsp {
			//   +Enabled     => false #bool
			//   +DeviceCount => 0 #int
			//   +Devices     => []modelmfa.TOTPDeviceInfo(nil)
			// }
			)

			require.Equal(t, 0, rsp.DeviceCount)
			require.Empty(t, rsp.Devices)
			require.False(t, rsp.Enabled)
		})
		assertResponseDataFieldExists(t, resp, "enabled")
		assertResponseDataFieldExists(t, resp, "device_count")
		assertResponseDataArrayField(t, resp, "devices")
	})

	t.Run("check_not_enabled", func(t *testing.T) {
		cli, err := client.New(checkAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: sessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Create(mfa.TOTPCheckReq{
			Username: username,
			Password: password,
		})
		require.NoError(t, err)

		helper.TestResp[*mfa.TOTPCheckRsp](t, resp, func(t *testing.T, rsp *mfa.TOTPCheckRsp) {
			t.Helper(
			// *modelmfa.TOTPStatusRsp {
			//   +Enabled     => true #bool
			//   +DeviceCount => 1 #int
			//   +Devices     => #[]modelmfa.TOTPDeviceInfo [
			//     0 => #modelmfa.TOTPDeviceInfo {
			//       +ID         => "019cb9a5-b52f-7e73-8ee2-e18a8971dd82" #string
			//       +DeviceName => "test-device" #string
			//       +LastUsedAt => "2026-03-05T00:19:30+08:00" #*string
			//       +CreatedAt  => "2026-03-05T00:19:30+08:00" #string
			//     }
			//   ]
			// }
			)

			require.False(t, rsp.RequiresMFA)
			require.NotEmpty(t, rsp.Message)
		})
		assertResponseDataFieldExists(t, resp, "requires_mfa")
	})

	t.Run("bind", func(t *testing.T) {
		cli, err := client.New(bindAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: sessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Create(nil)
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp *mfa.TOTPBindRsp) {
			t.Helper()
			require.NotNil(t, rsp)
			require.NotEmpty(t, rsp.ChallengeID)
			require.NotEmpty(t, rsp.OtpauthURL)
			require.NotEmpty(t, rsp.QRCodeImageDataURL)
			require.Equal(t, consts.FrameworkName, rsp.Issuer)
			require.Equal(t, username, rsp.AccountName)
			challengeID = rsp.ChallengeID
			secret = extractSecretFromOtpauthURL(t, rsp.OtpauthURL)
		})
		assertResponseDataFieldExists(t, resp, "qr_code_image_data_url")
	})

	t.Run("confirm", func(t *testing.T) {
		t.Run("invalid_challenge", func(t *testing.T) {
			cli, err := client.New(confirmAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: sessionID,
			}))
			require.NoError(t, err)

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
			cli, err := client.New(confirmAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: sessionID,
			}))
			require.NoError(t, err)

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
			helper.TestResp(t, resp, func(t *testing.T, rsp *mfa.TOTPConfirmRsp) {
				t.Helper(
				// #*modelmfa.TOTPConfirmRsp {
				//   +DeviceID    => "019cbc8d-857e-7e29-b2dc-ff983097a2e9" #string
				//   +Message     => "TOTP device confirmed and activated successfully" #string
				//   +BackupCodes => #[]string [
				//     0 => "J7KQ-4M2D-9VXA-P3RT" #string
				//     1 => "R8WP-B6ZD-7H3M-KQ2Y" #string
				//   ]
				// }
				)

				require.NotEmpty(t, rsp.DeviceID)
				require.NotEmpty(t, rsp.Message)
				require.NotEmpty(t, rsp.BackupCodes)
				require.Len(t, rsp.BackupCodes, 10)
				for _, bc := range rsp.BackupCodes {
					require.Regexp(t, `^[23456789ABCDEFGHJKLMNPQRSTUVWXYZ]{4}(-[23456789ABCDEFGHJKLMNPQRSTUVWXYZ]{4}){3}$`, bc)
				}
				deviceID = rsp.DeviceID
				backupCodes = rsp.BackupCodes
				assertBackupCodeHashesStored(t, deviceID, backupCodes)
			})
		})

		t.Run("duplicate_challenge", func(t *testing.T) {
			cli, err := client.New(confirmAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: sessionID,
			}))
			require.NoError(t, err)

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
	})

	t.Run("status_enabled", func(t *testing.T) {
		cli, err := client.New(statusAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: sessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Request(http.MethodGet, nil)
		require.NoError(t, err)
		helper.TestResp[*mfa.TOTPStatusRsp](t, resp, func(t *testing.T, rsp *mfa.TOTPStatusRsp) {
			t.Helper(
			// #*modelmfa.TOTPStatusRsp {
			//   +Enabled     => true #bool
			//   +DeviceCount => 1 #int
			//   +Devices     => #[]modelmfa.TOTPDeviceInfo [
			//     0 => #modelmfa.TOTPDeviceInfo {
			//       +ID         => "019cbc88-e885-7d4a-8811-5d4e23b177dc" #string
			//       +DeviceName => "test-device" #string
			//       +LastUsedAt => "2026-03-05T13:46:54+08:00" #*string
			//       +CreatedAt  => "2026-03-05T13:46:54+08:00" #string
			//     }
			//   ]
			// }
			)

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

	t.Run("check_enabled", func(t *testing.T) {
		cli, err := client.New(checkAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: sessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Create(mfa.TOTPCheckReq{
			Username: username,
			Password: password,
		})
		require.NoError(t, err)

		helper.TestResp[*mfa.TOTPCheckRsp](t, resp, func(t *testing.T, rsp *mfa.TOTPCheckRsp) {
			t.Helper(
			// #*modelmfa.TOTPCheckRsp {
			//   +RequiresMFA => true #bool
			//   +Message     => "MFA is enabled" #string
			// }
			)

			require.True(t, rsp.RequiresMFA)
			require.NotEmpty(t, rsp.Message)
		})
		assertResponseDataFieldExists(t, resp, "requires_mfa")
	})

	t.Run("login_requires_second_factor", func(t *testing.T) {
		cli, err := client.New(loginAPI)
		require.NoError(t, err)

		resp, err := cli.Create(iam.LoginReq{
			Username: username,
			Password: password,
		})
		require.Error(t, err)
		require.Nil(t, resp)
	})

	t.Run("login_with_totp_code", func(t *testing.T) {
		code, err := totp.GenerateCode(secret, time.Now())
		require.NoError(t, err)

		_ = loginSessionIDFromCookie(t, iam.LoginReq{
			Username: username,
			Password: password,
			TOTPCode: code,
		})
	})

	t.Run("login_rejects_conflicting_second_factors", func(t *testing.T) {
		if len(backupCodes) == 0 {
			t.Skip("no backup codes available")
		}
		cli, err := client.New(loginAPI)
		require.NoError(t, err)

		code, err := totp.GenerateCode(secret, time.Now())
		require.NoError(t, err)

		resp, err := cli.Create(iam.LoginReq{
			Username:   username,
			Password:   password,
			TOTPCode:   code,
			BackupCode: backupCodes[0],
		})
		require.Error(t, err)
		require.Nil(t, resp)
		assertBackupCodeHashCount(t, deviceID, 10)
	})

	t.Run("verify", func(t *testing.T) {
		t.Run("valid_code", func(t *testing.T) {
			cli, err := client.New(verifyAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: sessionID,
			}))
			require.NoError(t, err)

			code, err := totp.GenerateCode(secret, time.Now())
			require.NoError(t, err)

			resp, err := cli.Create(mfa.TOTPVerifyReq{
				TOTPCode: code,
			})
			require.NoError(t, err)
			helper.TestResp(t, resp, func(t *testing.T, rsp *mfa.TOTPVerifyRsp) {
				t.Helper(
				// #*modelmfa.TOTPVerifyRsp {
				//   +Valid   => true #bool
				//   +Message => "verification successful" #string
				// }
				)

				require.True(t, rsp.Valid)
				require.NotEmpty(t, rsp.Message)
			})
			assertResponseDataFieldExists(t, resp, "valid")
		})

		t.Run("invalid_code", func(t *testing.T) {
			cli, err := client.New(verifyAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: sessionID,
			}))
			require.NoError(t, err)

			resp, err := cli.Create(mfa.TOTPVerifyReq{
				TOTPCode: "000000",
			})
			require.NoError(t, err)
			helper.TestResp(t, resp, func(t *testing.T, rsp *mfa.TOTPVerifyRsp) {
				t.Helper(
				// #*modelmfa.TOTPVerifyRsp {
				//   +Valid   => false #bool
				//   +Message => "invalid verification code" #string
				// }
				)

				require.False(t, rsp.Valid)
				require.NotEmpty(t, rsp.Message)
			})
			assertResponseDataFieldExists(t, resp, "valid")
		})

		t.Run("invalid_format", func(t *testing.T) {
			cli, err := client.New(verifyAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: sessionID,
			}))
			require.NoError(t, err)

			resp, err := cli.Create(mfa.TOTPVerifyReq{
				TOTPCode: "abc123",
			})
			require.Error(t, err)
			require.Nil(t, resp)
		})
	})

	t.Run("login_with_backup_code", func(t *testing.T) {
		if len(backupCodes) < 2 {
			t.Skip("not enough backup codes available")
		}

		_ = loginSessionIDFromCookie(t, iam.LoginReq{
			Username:   username,
			Password:   password,
			BackupCode: backupCodes[1],
		})

		cli, err := client.New(loginAPI)
		require.NoError(t, err)

		resp, err := cli.Create(iam.LoginReq{
			Username:   username,
			Password:   password,
			BackupCode: backupCodes[1],
		})
		require.Error(t, err)
		require.Nil(t, resp)
		assertBackupCodeHashCount(t, deviceID, 9)
	})

	t.Run("unbind", func(t *testing.T) {
		t.Run("missing_fresh_auth", func(t *testing.T) {
			cli, err := client.New(unbindAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: sessionID,
			}))
			require.NoError(t, err)

			resp, err := cli.Create(mfa.TOTPUnbindReq{
				DeviceID: deviceID,
			})
			require.NoError(t, err)
			helper.TestResp(t, resp, func(t *testing.T, rsp *mfa.TOTPUnbindRsp) {
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
			if len(backupCodes) < 3 {
				t.Skip("not enough backup codes available")
			}
			cli, err := client.New(unbindAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: sessionID,
			}))
			require.NoError(t, err)

			resp, err := cli.Create(mfa.TOTPUnbindReq{
				DeviceID:   deviceID,
				Password:   password,
				BackupCode: backupCodes[2],
			})
			require.NoError(t, err)
			helper.TestResp(t, resp, func(t *testing.T, rsp *mfa.TOTPUnbindRsp) {
				t.Helper()

				require.False(t, rsp.Success)
				require.Equal(t, 1, rsp.DeviceCount)
				require.NotEmpty(t, rsp.Message)
			})
			assertResponseDataFieldExists(t, resp, "success")
			assertResponseDataFieldExists(t, resp, "device_count")
			assertTOTPDeviceActive(t, deviceID)
			assertBackupCodeHashCount(t, deviceID, 9)
		})

		t.Run("invalid_totp", func(t *testing.T) {
			cli, err := client.New(unbindAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: sessionID,
			}))
			require.NoError(t, err)

			resp, err := cli.Create(mfa.TOTPUnbindReq{
				DeviceID: deviceID,
				TOTPCode: "000000",
			})
			require.NoError(t, err)
			helper.TestResp(t, resp, func(t *testing.T, rsp *mfa.TOTPUnbindRsp) {
				t.Helper(
				// #*modelmfa.TOTPUnbindRsp {
				//   +Success     => false #bool
				//   +Message     => "Invalid TOTP code" #string
				//   +DeviceCount => 1 #int
				// }
				)

				require.False(t, rsp.Success)
				require.Equal(t, 1, rsp.DeviceCount)
				require.NotEmpty(t, rsp.Message)
			})
			assertResponseDataFieldExists(t, resp, "success")
			assertResponseDataFieldExists(t, resp, "device_count")
			assertTOTPDeviceActive(t, deviceID)
		})

		t.Run("valid_password", func(t *testing.T) {
			secondDeviceID, _, _ := bindTOTPDeviceForTest(t, sessionID, "test-device-password")
			cli, err := client.New(unbindAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: sessionID,
			}))
			require.NoError(t, err)

			resp, err := cli.Create(mfa.TOTPUnbindReq{
				DeviceID: secondDeviceID,
				Password: password,
			})
			require.NoError(t, err)
			helper.TestResp(t, resp, func(t *testing.T, rsp *mfa.TOTPUnbindRsp) {
				t.Helper()

				require.True(t, rsp.Success)
				require.Equal(t, 1, rsp.DeviceCount)
				require.NotEmpty(t, rsp.Message)
			})
			assertResponseDataFieldExists(t, resp, "success")
			assertResponseDataFieldExists(t, resp, "device_count")
		})

		t.Run("valid_totp", func(t *testing.T) {
			cli, err := client.New(unbindAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: sessionID,
			}))
			require.NoError(t, err)

			code, err := totp.GenerateCode(secret, time.Now())
			require.NoError(t, err)

			resp, err := cli.Create(mfa.TOTPUnbindReq{
				DeviceID: deviceID,
				TOTPCode: code,
			})
			require.NoError(t, err)
			helper.TestResp(t, resp, func(t *testing.T, rsp *mfa.TOTPUnbindRsp) {
				t.Helper(
				// #*modelmfa.TOTPUnbindRsp {
				//   +Success     => true #bool
				//   +Message     => "Device 'test-device' unbound successfully" #string
				//   +DeviceCount => 0 #int
				// }
				)

				require.True(t, rsp.Success)
				require.Equal(t, 0, rsp.DeviceCount)
				require.NotEmpty(t, rsp.Message)
			})
			assertResponseDataFieldExists(t, resp, "success")
			assertResponseDataFieldExists(t, resp, "device_count")
		})
	})

	t.Run("status_disabled_after_unbind", func(t *testing.T) {
		cli, err := client.New(statusAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: sessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Request(http.MethodGet, nil)
		require.NoError(t, err)
		helper.TestResp[*mfa.TOTPStatusRsp](t, resp, func(t *testing.T, rsp *mfa.TOTPStatusRsp) {
			t.Helper(
			// #*modelmfa.TOTPStatusRsp {
			//   +Enabled     => false #bool
			//   +DeviceCount => 0 #int
			//   +Devices     => []modelmfa.TOTPDeviceInfo(nil)
			// }
			)

			require.False(t, rsp.Enabled)
			require.Equal(t, 0, rsp.DeviceCount)
			require.Empty(t, rsp.Devices)
		})
		assertResponseDataFieldExists(t, resp, "enabled")
		assertResponseDataFieldExists(t, resp, "device_count")
		assertResponseDataArrayField(t, resp, "devices")
	})
}

func loginSessionIDFromCookie(t *testing.T, reqPayload iam.LoginReq) string {
	t.Helper()

	cli, err := client.New(loginAPI)
	require.NoError(t, err)

	apiResp, err := cli.Create(reqPayload)
	require.NoError(t, err)

	helper.TestResp(t, apiResp, func(t *testing.T, rsp *model.Empty) {
		t.Helper()
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
	helper.TestResp(t, bindResp, func(t *testing.T, rsp *mfa.TOTPBindRsp) {
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
	helper.TestResp(t, confirmResp, func(t *testing.T, rsp *mfa.TOTPConfirmRsp) {
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
