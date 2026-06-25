package logmgmt_test

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/bootstrap"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/helper"
	modellogmgmt "github.com/hydroan/gst/internal/model/logmgmt"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/module/authz"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/module/logmgmt"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

var (
	token        = "-"
	port         = 8000
	rootUsername = "root"
	rootPassword = "12345678"

	signupAPI       = fmt.Sprintf("http://localhost:%d/api/signup", port)
	loginAPI        = fmt.Sprintf("http://localhost:%d/api/login", port)
	logoutAPI       = fmt.Sprintf("http://localhost:%d/api/logout", port)
	loginlogAPI     = fmt.Sprintf("http://localhost:%d/api/log/loginlog", port)
	operationlogAPI = fmt.Sprintf("http://localhost:%d/api/log/operationlog", port)
	roleAPI         = fmt.Sprintf("http://localhost:%d/api/authz/roles", port)
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
		iam.Register(iam.Config{
			DefaultUsers: []*iam.User{
				{
					Base:     model.Base{ID: "root"},
					Type:     "admin",
					Username: rootUsername,
					Password: rootPassword,
				},
			},
		})
		authz.Register()
		logmgmt.Register()

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

func TestLogmgmt(t *testing.T) {
	username := "user01"
	password := "12345678"
	userID := ""
	var sessionID string
	var adminSessionID string

	t.Run("loginlog", func(t *testing.T) {
		// signup a user
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
				//   +UserID   => "019cbcc0-d2dd-7399-a4be-fc4ba2cd6775" #string
				//   +Username => "user01" #string
				//   +Message  => "User created successfully" #string
				// }
				)

				require.Equal(t, rsp.Username, username)
				require.NotEmpty(t, rsp.UserID)
				userID = rsp.UserID
				require.NotEmpty(t, rsp.Message)
			})
		})

		// user login
		t.Run("login1", func(t *testing.T) {
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
				//   +SessionID => "019cbcc0-d312-7b46-a67d-57877893c929" #string
				// }
				)

				require.NotEmpty(t, rsp.SessionID)
				sessionID = rsp.SessionID
			})
		})

		// check the login log count is 1
		t.Run("loginlog1", func(t *testing.T) {
			cli, err := client.New(loginlogAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: sessionID,
			}))
			require.NoError(t, err)

			items := make([]*logmgmt.LoginLog, 0)
			total := new(int64)
			resp, err := cli.List(&items, total)
			require.NoError(t, err)

			helper.TestResp(t, resp, func(t *testing.T, rsp ListResponse[*logmgmt.LoginLog]) {
				t.Helper(
				// #logmgmt_test.ListResponse[*github.com/hydroan/gst/internal/model/logmgmt.LoginLog] {
				//   +Items => #[]*modellogmgmt.LoginLog [
				//     0 => #*modellogmgmt.LoginLog {
				//       +UserID      => "019cbcc0-d2dd-7399-a4be-fc4ba2cd6775" #string
				//       +Username    => "user01" #string
				//       +ClientIP    => "::1" #string
				//       +Status      => "success" #modellogmgmt.LoginStatus
				//       +Source      => "gst" #string
				//       +Platform    => " " #string
				//       +Engine      => " " #string
				//       +Browser     => "gst " #string
				//       +Base        => #model.Base {
				//         +ID        => "019cbcc0-d314-7edc-9652-3a5d91222bb6" #string
				//       }
				//     }
				//   ]
				//   +Total => 1 #int64
				// }
				)

				require.Len(t, rsp.Items, 1)
				l := rsp.Items[0]
				require.Equal(t, l.UserID, userID)
				require.Equal(t, l.Username, username)
				require.Equal(t, modellogmgmt.LoginStatusSuccess, string(l.Status))
			})
		})

		// logout
		t.Run("logout", func(t *testing.T) {
			t.Run("logout", func(t *testing.T) {
				cli, err := client.New(logoutAPI, client.WithCookie(&http.Cookie{
					Name:  "session_id",
					Value: sessionID,
				}))
				require.NoError(t, err)

				resp, err := cli.Create(nil)
				require.NoError(t, err)

				helper.TestResp(t, resp, func(t *testing.T, rsp *iam.LogoutRsp) {
					t.Helper(
					// #*modeliam.LogoutRsp {
					//   +Msg => "logout successful" #string
					// }
					)
				})
			})
		})

		// login again to query the login log
		t.Run("login2", func(t *testing.T) {
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
				//   +SessionID => "019cbcc0-d34d-7e0d-9b9e-89d08c3ada3c" #string
				// }
				)

				require.NotEmpty(t, rsp.SessionID)
				sessionID = rsp.SessionID
			})
		})

		// check the login log count is 2
		t.Run("loginlog2", func(t *testing.T) {
			cli, err := client.New(loginlogAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: sessionID,
			}))
			require.NoError(t, err)

			items := make([]*logmgmt.LoginLog, 0)
			total := new(int64)
			resp, err := cli.List(&items, total)
			require.NoError(t, err)

			helper.TestResp(t, resp, func(t *testing.T, rsp ListResponse[*logmgmt.LoginLog]) {
				t.Helper()
				require.Len(t, rsp.Items, 3)
				l1, l2, l3 := rsp.Items[0], rsp.Items[1], rsp.Items[2]

				require.Equal(t, l1.UserID, userID)
				require.Equal(t, l1.Username, username)
				require.Equal(t, modellogmgmt.LoginStatusSuccess, string(l1.Status))

				require.Equal(t, l2.UserID, userID)
				require.Equal(t, l2.Username, username)
				require.Equal(t, modellogmgmt.LoginStatusLogout, string(l2.Status))

				require.Equal(t, l3.UserID, userID)
				require.Equal(t, l3.Username, username)
				require.Equal(t, modellogmgmt.LoginStatusSuccess, string(l3.Status))
			})
		})
	})

	t.Run("operationlog", func(t *testing.T) {
		t.Run("operationlog1", func(t *testing.T) {
			cli, err := client.New(operationlogAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: sessionID,
			}))
			require.NoError(t, err)

			items := make([]*logmgmt.OperationLog, 0)
			total := new(int64)

			resp, err := cli.List(&items, total)
			require.NoError(t, err)

			// operation log count is 0
			helper.TestResp(t, resp, func(t *testing.T, rsp ListResponse[*logmgmt.OperationLog]) {
				t.Helper()
				require.Empty(t, rsp.Items)
			})
		})

		t.Run("login-root", func(t *testing.T) {
			cli, err := client.New(loginAPI)
			require.NoError(t, err)

			resp, err := cli.Create(iam.LoginReq{
				Username: rootUsername,
				Password: rootPassword,
			})
			require.NoError(t, err)

			helper.TestResp(t, resp, func(t *testing.T, rsp *iam.LoginRsp) {
				t.Helper()
				require.NotEmpty(t, rsp.SessionID)
				adminSessionID = rsp.SessionID
			})
		})

		t.Run("create-role", func(t *testing.T) {
			cli, err := client.New(roleAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: adminSessionID,
			}))
			require.NoError(t, err)

			createReq := &authz.Role{
				Name: "Logmgmt Test Role",
				Code: "logmgmt_test_role",
			}
			resp, err := cli.Create(createReq)
			require.NoError(t, err)

			helper.TestResp(t, resp, func(t *testing.T, rsp *authz.Role) {
				t.Helper(
				// #*modelauthz.Role {
				//   +Name => "Logmgmt Test Role" #string
				//   +Code => "logmgmt_test_role" #string
				//   +Base => #model.Base {
				//     +ID => "019cbcc5-0da0-7874-bd81-740fa7fdfe1f" #string
				//   }
				// }
				)

				require.NotNil(t, rsp)
				require.Equal(t, createReq.Name, rsp.Name)
				require.Equal(t, createReq.Code, rsp.Code)
			})
		})

		// 记录 operationlog 可能会有延迟，因为是异步写入的。
		time.Sleep(1 * time.Second)
		t.Run("operationlog2", func(t *testing.T) {
			cli, err := client.New(operationlogAPI, client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: sessionID,
			}))
			require.NoError(t, err)

			items := make([]*logmgmt.OperationLog, 0)
			total := new(int64)

			resp, err := cli.List(&items, total)
			require.NoError(t, err)

			// operation log count is 1
			helper.TestResp(t, resp, func(t *testing.T, rsp ListResponse[*logmgmt.OperationLog]) {
				t.Helper(
				// #logmgmt_test.ListResponse[*github.com/hydroan/gst/internal/model/logmgmt.OperationLog] {
				//   +Items => #[]*modellogmgmt.OperationLog [
				//     0 => #*modellogmgmt.OperationLog {
				//       +User        => "root" #string
				//       +IP          => "::1" #string
				//       +OP          => "create" #consts.OP
				//       +Table       => "roles" #string
				//       +Model       => "Role" #string
				//       +RecordID    => "019cbcc7-3f8e-7c96-b369-e3e16b543a23" #string
				//       +RecordName  => "" #string
				//       +Record      => "{"name":"Logmgmt Test Role","code":"logmgmt_test_role","id":"019cbcc7-3f8e-7c96-b369-e3e16b543a23","created_by":"root","updated_by":"root","created_at":"2026-03-05T14:55:00.494825+08:00","updated_at":"2026-03-05T14:55:00.494848+08:00"}" #string
				//       +Request     => "{"name":"Logmgmt Test Role","code":"logmgmt_test_role","id":"019cbcc7-3f8e-7c96-b369-e3e16b543a23","created_by":"root","updated_by":"root","created_at":"2026-03-05T14:55:00.494825+08:00","updated_at":"2026-03-05T14:55:00.494848+08:00"}" #string
				//       +Response    => "{"name":"Logmgmt Test Role","code":"logmgmt_test_role","id":"019cbcc7-3f8e-7c96-b369-e3e16b543a23","created_by":"root","updated_by":"root","created_at":"2026-03-05T14:55:00.494825+08:00","updated_at":"2026-03-05T14:55:00.494848+08:00"}" #string
				//       +OldRecord   => "" #string
				//       +NewRecord   => "" #string
				//       +Method      => "POST" #string
				//       +URI         => "/api/authz/roles" #string
				//       +UserAgent   => "gst" #string
				//       +TraceID   => "d6kihh65shg82oca209g" #string
				//       +Base        => #model.Base {
				//         +ID        => "019cbcc7-3f8f-7130-a72a-d68ec0a7c0f9" #string
				//       }
				//     }
				//   ]
				//   +Total => 1 #int64
				// }
				)

				require.Len(t, rsp.Items, 1)
				l := rsp.Items[0]
				require.NotNil(t, l)
				require.Equal(t, rootUsername, l.User)
				require.Equal(t, consts.OP_CREATE, l.OP)
				require.Equal(t, "roles", l.Table)
				require.Equal(t, "Role", l.Model)
				require.Equal(t, "/api/authz/roles", l.URI)
			})
		})
	})
}
