package twofa_test

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hydroan/gst/bootstrap"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/helper"
	modeltwofa "github.com/hydroan/gst/internal/model/twofa"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/module/twofa"
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
	verifyAPI  = fmt.Sprintf("http://localhost:%d/api/2fa/totp/verify", port)
	checkAPI   = fmt.Sprintf("http://localhost:%d/api/2fa/totp/check", port)
	bindAPI    = fmt.Sprintf("http://localhost:%d/api/2fa/totp/bind", port)
	confirmAPI = fmt.Sprintf("http://localhost:%d/api/2fa/totp/confirm", port)
	unbindAPI  = fmt.Sprintf("http://localhost:%d/api/2fa/totp/unbind", port)
	statusAPI  = fmt.Sprintf("http://localhost:%d/api/2fa/totp/status", port)
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
		twofa.Register()

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

func Test2fa(t *testing.T) {
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
		cli, err := client.New(loginAPI)
		require.NoError(t, err)

		resp, err := cli.Create(iam.LoginReq{
			Username: username,
			Password: password,
		})
		require.NoError(t, err)

		helper.TestResp(t, resp, func(t *testing.T, rsp *iam.LoginRsp) {
			t.Helper(
			// #*modeliam.LoginRsp {
			//   +SessionID => "019cbc8c-98d5-72ee-813c-c2a098780bfc" #string
			// }
			)

			require.NotEmpty(t, rsp.SessionID)
			sessionID = rsp.SessionID
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
		helper.TestResp[*twofa.TOTPStatusRsp](t, resp, func(t *testing.T, rsp *twofa.TOTPStatusRsp) {
			t.Helper(
			// #*modeltwofa.TOTPStatusRsp {
			//   +Enabled     => false #bool
			//   +DeviceCount => 0 #int
			//   +Devices     => []modeltwofa.TOTPDeviceInfo(nil)
			// }
			)

			require.Equal(t, 0, rsp.DeviceCount)
			require.Empty(t, rsp.Devices)
			require.False(t, rsp.Enabled)
		})
	})

	t.Run("check_not_enabled", func(t *testing.T) {
		cli, err := client.New(checkAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: sessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Create(twofa.TOTPCheckReq{
			Username: username,
			Password: password,
		})
		require.NoError(t, err)

		helper.TestResp[*twofa.TOTPCheckRsp](t, resp, func(t *testing.T, rsp *twofa.TOTPCheckRsp) {
			t.Helper(
			// *modeltwofa.TOTPStatusRsp {
			//   +Enabled     => true #bool
			//   +DeviceCount => 1 #int
			//   +Devices     => #[]modeltwofa.TOTPDeviceInfo [
			//     0 => #modeltwofa.TOTPDeviceInfo {
			//       +ID         => "019cb9a5-b52f-7e73-8ee2-e18a8971dd82" #string
			//       +DeviceName => "test-device" #string
			//       +IsActive   => true #bool
			//       +LastUsedAt => "2026-03-05T00:19:30+08:00" #*string
			//       +CreatedAt  => "2026-03-05T00:19:30+08:00" #string
			//     }
			//   ]
			// }
			)

			require.False(t, rsp.Requires2FA)
			require.NotEmpty(t, rsp.Message)
		})
	})

	t.Run("bind", func(t *testing.T) {
		cli, err := client.New(bindAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: sessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Create(nil)
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp *twofa.TOTPBindRsp) {
			t.Helper()
			require.NotNil(t, rsp)
			require.NotEmpty(t, rsp.ChallengeID)
			require.NotEmpty(t, rsp.OtpauthURL)
			require.NotEmpty(t, rsp.QRCodeImage)
			require.Equal(t, consts.FrameworkName, rsp.Issuer)
			require.Equal(t, username, rsp.AccountName)
			challengeID = rsp.ChallengeID
			secret = extractSecretFromOtpauthURL(t, rsp.OtpauthURL)
		})
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

			resp, err := cli.Create(twofa.TOTPConfirmReq{
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

			resp, err := cli.Create(twofa.TOTPConfirmReq{
				ChallengeID: challengeID,
				Code:        invalidCode,
				DeviceName:  "test-device-2",
			})
			require.Error(t, err)
			require.Nil(t, resp)

			resp, err = cli.Create(twofa.TOTPConfirmReq{
				ChallengeID: challengeID,
				Code:        code,
				DeviceName:  "test-device",
			})
			require.NoError(t, err)
			helper.TestResp(t, resp, func(t *testing.T, rsp *twofa.TOTPConfirmRsp) {
				t.Helper(
				// #*modeltwofa.TOTPConfirmRsp {
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

			resp, err := cli.Create(twofa.TOTPConfirmReq{
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
		helper.TestResp[*twofa.TOTPStatusRsp](t, resp, func(t *testing.T, rsp *twofa.TOTPStatusRsp) {
			t.Helper(
			// #*modeltwofa.TOTPStatusRsp {
			//   +Enabled     => true #bool
			//   +DeviceCount => 1 #int
			//   +Devices     => #[]modeltwofa.TOTPDeviceInfo [
			//     0 => #modeltwofa.TOTPDeviceInfo {
			//       +ID         => "019cbc88-e885-7d4a-8811-5d4e23b177dc" #string
			//       +DeviceName => "test-device" #string
			//       +IsActive   => true #bool
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
				require.True(t, d.IsActive)
				require.NotEmpty(t, d.LastUsedAt)
			}
		})
	})

	t.Run("check_enabled", func(t *testing.T) {
		cli, err := client.New(checkAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: sessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Create(twofa.TOTPCheckReq{
			Username: username,
			Password: password,
		})
		require.NoError(t, err)

		helper.TestResp[*twofa.TOTPCheckRsp](t, resp, func(t *testing.T, rsp *twofa.TOTPCheckRsp) {
			t.Helper(
			// #*modeltwofa.TOTPCheckRsp {
			//   +Requires2FA => true #bool
			//   +Message     => "2FA is enabled" #string
			// }
			)

			require.True(t, rsp.Requires2FA)
			require.NotEmpty(t, rsp.Message)
		})
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
		cli, err := client.New(loginAPI)
		require.NoError(t, err)

		code, err := totp.GenerateCode(secret, time.Now())
		require.NoError(t, err)

		resp, err := cli.Create(iam.LoginReq{
			Username: username,
			Password: password,
			TOTPCode: code,
		})
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp *iam.LoginRsp) {
			t.Helper()

			require.NotEmpty(t, rsp.SessionID)
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

			resp, err := cli.Create(twofa.TOTPVerifyReq{
				Code: code,
			})
			require.NoError(t, err)
			helper.TestResp(t, resp, func(t *testing.T, rsp *twofa.TOTPVerifyRsp) {
				t.Helper(
				// #*modeltwofa.TOTPVerifyRsp {
				//   +Valid   => true #bool
				//   +Message => "verification successful" #string
				// }
				)

				require.True(t, rsp.Valid)
				require.NotEmpty(t, rsp.Message)
			})
		})

		t.Run("invalid_code", func(t *testing.T) {
			cli, err := client.New(verifyAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: sessionID,
			}))
			require.NoError(t, err)

			resp, err := cli.Create(twofa.TOTPVerifyReq{
				Code: "000000",
			})
			require.NoError(t, err)
			helper.TestResp(t, resp, func(t *testing.T, rsp *twofa.TOTPVerifyRsp) {
				t.Helper(
				// #*modeltwofa.TOTPVerifyRsp {
				//   +Valid   => false #bool
				//   +Message => "invalid verification code" #string
				// }
				)

				require.False(t, rsp.Valid)
				require.NotEmpty(t, rsp.Message)
			})
		})

		t.Run("valid_backup_code", func(t *testing.T) {
			if len(backupCodes) == 0 {
				t.Skip("no backup codes available")
			}
			cli, err := client.New(verifyAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: sessionID,
			}))
			require.NoError(t, err)

			normalizedInput := strings.ToLower(strings.ReplaceAll(backupCodes[0], "-", ""))
			resp, err := cli.Create(twofa.TOTPVerifyReq{
				Code:     normalizedInput,
				IsBackup: true,
			})
			require.NoError(t, err)
			helper.TestResp(t, resp, func(t *testing.T, rsp *twofa.TOTPVerifyRsp) {
				t.Helper(
				// #*modeltwofa.TOTPVerifyRsp {
				//   +Valid   => true #bool
				//   +Message => "verification successful" #string
				// }
				)

				require.True(t, rsp.Valid)
				require.NotEmpty(t, rsp.Message)
			})

			resp, err = cli.Create(twofa.TOTPVerifyReq{
				Code:     backupCodes[0],
				IsBackup: true,
			})
			require.NoError(t, err)
			helper.TestResp(t, resp, func(t *testing.T, rsp *twofa.TOTPVerifyRsp) {
				t.Helper()

				require.False(t, rsp.Valid)
				require.NotEmpty(t, rsp.Message)
			})
			assertBackupCodeHashCount(t, deviceID, 9)
		})
	})

	t.Run("login_with_backup_code", func(t *testing.T) {
		if len(backupCodes) < 2 {
			t.Skip("not enough backup codes available")
		}
		cli, err := client.New(loginAPI)
		require.NoError(t, err)

		resp, err := cli.Create(iam.LoginReq{
			Username:   username,
			Password:   password,
			BackupCode: backupCodes[1],
		})
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp *iam.LoginRsp) {
			t.Helper()

			require.NotEmpty(t, rsp.SessionID)
		})

		resp, err = cli.Create(iam.LoginReq{
			Username:   username,
			Password:   password,
			BackupCode: backupCodes[1],
		})
		require.Error(t, err)
		require.Nil(t, resp)
		assertBackupCodeHashCount(t, deviceID, 8)
	})

	t.Run("unbind", func(t *testing.T) {
		t.Run("missing_fresh_auth", func(t *testing.T) {
			cli, err := client.New(unbindAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: sessionID,
			}))
			require.NoError(t, err)

			resp, err := cli.Create(twofa.TOTPUnbindReq{
				DeviceID: deviceID,
			})
			require.NoError(t, err)
			helper.TestResp(t, resp, func(t *testing.T, rsp *twofa.TOTPUnbindRsp) {
				t.Helper()

				require.False(t, rsp.Success)
				require.NotEmpty(t, rsp.Message)
			})
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

			resp, err := cli.Create(twofa.TOTPUnbindReq{
				DeviceID:   deviceID,
				Password:   password,
				BackupCode: backupCodes[2],
			})
			require.NoError(t, err)
			helper.TestResp(t, resp, func(t *testing.T, rsp *twofa.TOTPUnbindRsp) {
				t.Helper()

				require.False(t, rsp.Success)
				require.NotEmpty(t, rsp.Message)
			})
			assertTOTPDeviceActive(t, deviceID)
			assertBackupCodeHashCount(t, deviceID, 8)
		})

		t.Run("invalid_totp", func(t *testing.T) {
			cli, err := client.New(unbindAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: sessionID,
			}))
			require.NoError(t, err)

			resp, err := cli.Create(twofa.TOTPUnbindReq{
				DeviceID: deviceID,
				TOTPCode: "000000",
			})
			require.NoError(t, err)
			helper.TestResp(t, resp, func(t *testing.T, rsp *twofa.TOTPUnbindRsp) {
				t.Helper(
				// #*modeltwofa.TOTPUnbindRsp {
				//   +Success     => false #bool
				//   +Message     => "Invalid TOTP code" #string
				//   +DeviceCount => 0 #int
				// }
				)

				require.False(t, rsp.Success)
				require.NotEmpty(t, rsp.Message)
			})
			assertTOTPDeviceActive(t, deviceID)
		})

		t.Run("valid_password", func(t *testing.T) {
			secondDeviceID, _, _ := bindTOTPDeviceForTest(t, sessionID, "test-device-password")
			cli, err := client.New(unbindAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: sessionID,
			}))
			require.NoError(t, err)

			resp, err := cli.Create(twofa.TOTPUnbindReq{
				DeviceID: secondDeviceID,
				Password: password,
			})
			require.NoError(t, err)
			helper.TestResp(t, resp, func(t *testing.T, rsp *twofa.TOTPUnbindRsp) {
				t.Helper()

				require.True(t, rsp.Success)
				require.Equal(t, 1, rsp.DeviceCount)
				require.NotEmpty(t, rsp.Message)
			})
		})

		t.Run("valid_totp", func(t *testing.T) {
			cli, err := client.New(unbindAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: sessionID,
			}))
			require.NoError(t, err)

			code, err := totp.GenerateCode(secret, time.Now())
			require.NoError(t, err)

			resp, err := cli.Create(twofa.TOTPUnbindReq{
				DeviceID: deviceID,
				TOTPCode: code,
			})
			require.NoError(t, err)
			helper.TestResp(t, resp, func(t *testing.T, rsp *twofa.TOTPUnbindRsp) {
				t.Helper(
				// #*modeltwofa.TOTPUnbindRsp {
				//   +Success     => true #bool
				//   +Message     => "Device 'test-device' unbound successfully" #string
				//   +DeviceCount => 0 #int
				// }
				)

				require.True(t, rsp.Success)
				require.Equal(t, 0, rsp.DeviceCount)
				require.NotEmpty(t, rsp.Message)
			})
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
		helper.TestResp[*twofa.TOTPStatusRsp](t, resp, func(t *testing.T, rsp *twofa.TOTPStatusRsp) {
			t.Helper(
			// #*modeltwofa.TOTPStatusRsp {
			//   +Enabled     => false #bool
			//   +DeviceCount => 0 #int
			//   +Devices     => []modeltwofa.TOTPDeviceInfo(nil)
			// }
			)

			require.False(t, rsp.Enabled)
			require.Equal(t, 0, rsp.DeviceCount)
		})
	})
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

	bindResp, err := bindCli.Create(twofa.TOTPBind{})
	require.NoError(t, err)

	var challengeID string
	var secret string
	helper.TestResp(t, bindResp, func(t *testing.T, rsp *twofa.TOTPBindRsp) {
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

	confirmResp, err := confirmCli.Create(twofa.TOTPConfirmReq{
		ChallengeID: challengeID,
		Code:        code,
		DeviceName:  deviceName,
	})
	require.NoError(t, err)

	var deviceID string
	var backupCodes []string
	helper.TestResp(t, confirmResp, func(t *testing.T, rsp *twofa.TOTPConfirmRsp) {
		t.Helper()

		require.NotEmpty(t, rsp.DeviceID)
		require.NotEmpty(t, rsp.BackupCodes)
		deviceID = rsp.DeviceID
		backupCodes = rsp.BackupCodes
	})

	return deviceID, secret, backupCodes
}

func getTOTPDeviceForTest(t *testing.T, deviceID string) *modeltwofa.TOTPDevice {
	t.Helper()

	device := new(modeltwofa.TOTPDevice)
	require.NoError(t, database.Database[*modeltwofa.TOTPDevice](nil).Get(device, deviceID))
	return device
}

func normalizeBackupCodeForTest(code string) string {
	code = strings.TrimSpace(code)
	code = strings.ReplaceAll(code, "-", "")
	return strings.ToUpper(code)
}
