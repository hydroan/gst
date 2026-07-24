package client_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/hydroan/gst/bootstrap"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/router"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	token = "-"
	port  int
	addr2 string

	id1     = "user1"
	id2     = "user2"
	id3     = "user3"
	id4     = "user4"
	id5     = "user5"
	name1   = id1
	name2   = id2
	name3   = id3
	name4   = id4
	name5   = id5
	email1  = "user1@gmail.com"
	email2  = "user2@gmail.com"
	email3  = "user3@gmail.com"
	email4  = "user4@gmail.com"
	email5  = "user5@gmail.com"
	avatar1 = "avatar1"
	avatar2 = "avatar2"
	avatar3 = "avatar3"
	avatar4 = "avatar4"
	avatar5 = "avatar5"

	name1Modified   = id1 + "_modified"
	email1Modified  = email1 + "_modified"
	avatar1Modified = avatar1 + "_modified"

	avatar2Modified = avatar2 + "_modified"

	user1 = User{Name: name1, Email: email1, Avatar: avatar1, Base: model.Base{ID: id1}}
	user2 = User{Name: name2, Email: email2, Avatar: avatar2, Base: model.Base{ID: id2}}
	user3 = User{Name: name3, Email: email3, Avatar: avatar3, Base: model.Base{ID: id3}}
	user4 = User{Name: name4, Email: email4, Avatar: avatar4, Base: model.Base{ID: id4}}
	user5 = User{Name: name5, Email: email5, Avatar: avatar5, Base: model.Base{ID: id5}}

	serverOnce sync.Once
)

func startServer(t *testing.T) {
	t.Helper()

	serverOnce.Do(func() {
		startServerOnce(t)
	})
}

func startServerOnce(t *testing.T) {
	t.Helper()

	model.Register[*User]()

	port = testutil.SetupRandomServerPort()
	addr2 = testutil.URL(port, "/api/test-user/")

	t.Setenv(config.DATABASE_TYPE, string(config.DBSqlite))
	t.Setenv(config.SQLITE_IS_MEMORY, "true")
	t.Setenv(config.LOGGER_DIR, "/tmp/test_client")
	t.Setenv(config.AUTH_NONE_EXPIRE_TOKEN, token)

	// os.Setenv(config.DATABASE_TYPE, string(config.DBMySQL))
	// os.Setenv(config.MYSQL_DATABASE, "test")
	// os.Setenv(config.MYSQL_USERNAME, "test")
	// os.Setenv(config.MYSQL_PASSWORD, "test")

	if err := bootstrap.Bootstrap(); err != nil {
		require.NoError(t, err)
	}

	go func() {
		router.Register[*User, *User, *User](router.Auth(), "test-user", nil, consts.Create)
		router.Register[*User, *User, *User](router.Auth(), "test-user/:id", &types.ControllerConfig[*User]{ParamName: "id"}, consts.Delete)
		router.Register[*User, *User, *User](router.Auth(), "test-user/:id", &types.ControllerConfig[*User]{ParamName: "id"}, consts.Update)
		router.Register[*User, *User, *User](router.Auth(), "test-user/:id", &types.ControllerConfig[*User]{ParamName: "id"}, consts.Patch)
		router.Register[*User, *User, *User](router.Auth(), "test-user", nil, consts.List)
		router.Register[*User, *User, *User](router.Auth(), "test-user/:id", &types.ControllerConfig[*User]{ParamName: "id"}, consts.Get)
		router.Register[*User, *User, *User](router.Auth(), "test-user/batch", nil, consts.CreateMany)
		router.Register[*User, *User, *User](router.Auth(), "test-user/batch", nil, consts.DeleteMany)
		router.Register[*User, *User, *User](router.Auth(), "test-user/batch", nil, consts.UpdateMany)
		router.Register[*User, *User, *User](router.Auth(), "test-user/batch", nil, consts.PatchMany)
		if err := bootstrap.Run(); err != nil {
			panic(err)
		}
		os.Exit(0)
	}()
	testutil.MustWaitForServer(port)
}

func Test_Client(t *testing.T) {
	startServer(t)

	cli, err := client.New(addr2, client.WithToken(token), client.WithQueryPagination(1, 2))
	require.NoError(t, err)
	fmt.Println(cli.QueryString())
	fmt.Println(cli.RequestURL())

	_, err = cli.Create(user1)
	require.NoError(t, err)
	_, err = cli.Create(user2)
	require.NoError(t, err)
	_, err = cli.Create(user3)
	require.NoError(t, err)
	_, err = cli.Create(user4)
	require.NoError(t, err)
	_, err = cli.Create(user5)
	require.NoError(t, err)

	users := make([]User, 0)
	total := new(int)
	user := new(User)

	// test List
	t.Run("list", func(t *testing.T) {
		resp, err := cli.List(&users, total)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.TraceID)
		require.Len(t, users, 2)
		require.Equal(t, 5, *total)
	})
	// test Get
	t.Run("get", func(t *testing.T) {
		resp, err := cli.Get(id1, user)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.TraceID)
		require.Equal(t, id1, user.ID)
		require.Equal(t, name1, user.Name)
		require.Equal(t, email1, user.Email)
		require.Equal(t, avatar1, user.Avatar)

		resp, err = cli.Get(id2, user)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.TraceID)
		require.Equal(t, id2, user.ID)
		require.Equal(t, name2, user.Name)
		require.Equal(t, email2, user.Email)
		require.Equal(t, avatar2, user.Avatar)

		resp, err = cli.Get(id3, user)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.TraceID)
		require.Equal(t, id3, user.ID)
		require.Equal(t, name3, user.Name)
		require.Equal(t, email3, user.Email)
		require.Equal(t, avatar3, user.Avatar)

		resp, err = cli.Get(id4, user)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.TraceID)
		require.Equal(t, id4, user.ID)
		require.Equal(t, name4, user.Name)
		require.Equal(t, email4, user.Email)
		require.Equal(t, avatar4, user.Avatar)

		resp, err = cli.Get(id5, user)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.TraceID)
		require.Equal(t, id5, user.ID)
		require.Equal(t, name5, user.Name)
		require.Equal(t, email5, user.Email)
		require.Equal(t, avatar5, user.Avatar)
	})

	// Test Update
	t.Run("update", func(t *testing.T) {
		resp, err := cli.Update(id1, &User{Name: name1Modified, Email: email1Modified, Base: model.Base{ID: id1}})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.TraceID)

		resp, err = cli.Get(id1, user)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.TraceID)
		require.Equal(t, id1, user.ID)
		require.Equal(t, name1Modified, user.Name)
		require.Equal(t, email1Modified, user.Email)
		require.Empty(t, user.Avatar)
	})

	// Test Patch
	t.Run("patch", func(t *testing.T) {
		resp, err := cli.Patch(id1, &User{Avatar: avatar1Modified, Base: model.Base{ID: id1}})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.TraceID)

		resp, err = cli.Get(id1, user)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.TraceID)
		require.Equal(t, id1, user.ID)
		require.Equal(t, name1Modified, user.Name)
		require.Equal(t, email1Modified, user.Email)
		require.Equal(t, avatar1Modified, user.Avatar)

		resp, err = cli.Patch(id1, &User{Name: name1, Base: model.Base{ID: id1}})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.TraceID)

		resp, err = cli.Get(id1, user)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.TraceID)
		require.Equal(t, id1, user.ID)
		require.Equal(t, name1, user.Name)
		require.Equal(t, email1Modified, user.Email)
		require.Equal(t, avatar1Modified, user.Avatar)

		resp, err = cli.Patch(id1, &User{Email: email1, Avatar: avatar1, Base: model.Base{ID: id1}})
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.TraceID)
		require.NoError(t, err)
		resp, err = cli.Get(id1, user)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.TraceID)
		require.Equal(t, id1, user.ID)
		require.Equal(t, name1, user.Name)
		require.Equal(t, email1, user.Email)
		require.Equal(t, avatar1, user.Avatar)
	})

	// Test CreateMany
	t.Run("create_many", func(t *testing.T) {
		cli, err := client.New(addr2, client.WithToken(token))
		require.NoError(t, err)
		items := make([]User, 0)
		total := 0

		// 1. delete all resources.
		_, err = cli.DeleteMany([]string{id1, id2, id3, id4, id5})
		require.NoError(t, err)
		_, err = cli.CreateMany(user1)
		require.ErrorIs(t, err, client.ErrNotStructSlice)

		// 2.check the number of resources after create.
		_, err = cli.List(&items, &total)
		require.NoError(t, err)
		require.Empty(t, items)
		require.Equal(t, 0, total)

		// 3.create resources.
		resp, err := cli.CreateMany([]User{user1, user2, user3, user4, user5})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.TraceID)

		// 4.check the number of resources after create.
		_, err = cli.List(&items, &total)
		require.NoError(t, err)
		require.Len(t, items, 5)
		require.Equal(t, 5, total)
	})

	// Test DeleteMany
	t.Run("delete_many", func(t *testing.T) {
		cli, err := client.New(addr2, client.WithToken(token))
		require.NoError(t, err)
		items := make([]User, 0)
		total := 0

		// 1.create resources from a clean slate: UpdateMany cannot create, so
		// drop the fixed-id rows and recreate them explicitly.
		_, err = cli.DeleteMany([]string{id1, id2, id3, id4, id5})
		require.NoError(t, err)
		_, err = cli.CreateMany([]User{user1, user2, user3, user4, user5})
		require.NoError(t, err)

		// 2.check the number of resources after create.
		_, err = cli.List(&items, &total)
		require.NoError(t, err)
		require.Len(t, items, 5)
		require.Equal(t, 5, total)

		// 3.delete resources
		resp, err := cli.DeleteMany([]string{id1, id2, id3, id4, id5})
		require.NoError(t, err)
		_ = resp
		// require.NotNil(t, resp)
		// require.NotEmpty(t, resp.TraceID)
		_, err = cli.DeleteMany([]int{1})
		require.ErrorIs(t, err, client.ErrNotStringSlice)

		// 4.check the number of resources after delete
		_, err = cli.List(&items, &total)
		require.NoError(t, err)
		require.Empty(t, items)
		require.Equal(t, 0, total)
	})

	// Test UpdateMany
	t.Run("update_many", func(t *testing.T) {
		cli, err := client.New(addr2, client.WithToken(token))
		require.NoError(t, err)

		// 1.delete all resources
		_, err = cli.DeleteMany([]string{id1, id2, id3, id4, id5})
		require.NoError(t, err)

		// 2.creat all resources
		_, err = cli.CreateMany([]User{user1, user2, user3, user4, user5})
		require.NoError(t, err)

		// u1 only modified email
		u1 := user1
		u1.Email = email1Modified
		// u2 only modified avator
		u2 := user2
		u2.Avatar = avatar2Modified
		resp, err := cli.UpdateMany([]User{u1, u2})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.TraceID)

		u := new(User)
		_, err = cli.Get(id1, u)
		require.NoError(t, err)
		require.Equal(t, u.Name, user1.Name)
		require.Equal(t, u.Email, email1Modified)
		require.Equal(t, u.Avatar, user1.Avatar)

		_, err = cli.Get(id2, u)
		require.NoError(t, err)
		require.Equal(t, u.Name, user2.Name)
		require.Equal(t, u.Email, user2.Email)
		require.Equal(t, u.Avatar, avatar2Modified)

		_, err = cli.Get(id3, u)
		require.NoError(t, err)
		require.Equal(t, u.Name, user3.Name)
		require.Equal(t, u.Email, user3.Email)
		require.Equal(t, u.Avatar, user3.Avatar)

		_, err = cli.Get(id4, u)
		require.NoError(t, err)
		require.Equal(t, u.Name, user4.Name)
		require.Equal(t, u.Email, user4.Email)
		require.Equal(t, u.Avatar, user4.Avatar)
	})

	// Test PatchMany
	t.Run("patch_many", func(t *testing.T) {
		cli, err := client.New(addr2, client.WithToken(token))
		require.NoError(t, err)

		// 1.delete all resources
		_, err = cli.DeleteMany([]string{id1, id2, id3, id4, id5})
		require.NoError(t, err)

		// 2.creat all resources
		_, err = cli.CreateMany([]User{user1, user2, user3, user4, user5})
		require.NoError(t, err)

		// u1 only modified email
		u1 := &User{Email: email1Modified}
		u1.ID = id1
		// u2 only modified avator
		u2 := &User{Avatar: avatar2Modified}
		u2.ID = id2
		resp, err := cli.PatchMany([]*User{u1, u2})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.TraceID)

		u := new(User)
		_, err = cli.Get(id1, u)
		require.NoError(t, err)
		require.Equal(t, u.Name, user1.Name)
		require.Equal(t, u.Email, email1Modified)
		require.Equal(t, u.Avatar, user1.Avatar)

		_, err = cli.Get(id2, u)
		require.NoError(t, err)
		require.Equal(t, u.Name, user2.Name)
		require.Equal(t, u.Email, user2.Email)
		require.Equal(t, u.Avatar, avatar2Modified)

		_, err = cli.Get(id3, u)
		require.NoError(t, err)
		require.Equal(t, u.Name, user3.Name)
		require.Equal(t, u.Email, user3.Email)
		require.Equal(t, u.Avatar, user3.Avatar)

		_, err = cli.Get(id4, u)
		require.NoError(t, err)
		require.Equal(t, u.Name, user4.Name)
		require.Equal(t, u.Email, user4.Email)
		require.Equal(t, u.Avatar, user4.Avatar)
	})
}

func Test_Client_WithAPI(t *testing.T) {
	startServer(t)

	baseAddr := testutil.URL(port, "/api")

	// Create test users first, dropping fixed-id leftovers from earlier tests
	// because Create rejects duplicates.
	cliSetup, err := client.New(baseAddr+"/test-user", client.WithToken(token))
	require.NoError(t, err)
	_, err = cliSetup.DeleteMany([]string{id1, id2})
	require.NoError(t, err)
	_, err = cliSetup.Create(user1)
	require.NoError(t, err)
	_, err = cliSetup.Create(user2)
	require.NoError(t, err)

	// Test WithAPI option with List method
	t.Run("with_api_option_list", func(t *testing.T) {
		cli, err := client.New(baseAddr, client.WithToken(token), client.WithAPI("test-user"))
		require.NoError(t, err)

		// Test List using apiPath from WithAPI
		users := make([]User, 0)
		total := new(int)
		resp, err := cli.List(&users, total)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.TraceID)
		require.GreaterOrEqual(t, *total, 0)
	})

	// Test WithAPI option with Get method
	t.Run("with_api_option_get", func(t *testing.T) {
		cli, err := client.New(baseAddr, client.WithToken(token), client.WithAPI("test-user"))
		require.NoError(t, err)

		// Test Get using apiPath from WithAPI
		user := new(User)
		resp, err := cli.Get(id1, user)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.TraceID)
		require.Equal(t, id1, user.ID)
		require.Equal(t, name1, user.Name)
		require.Equal(t, email1, user.Email)
		require.Equal(t, avatar1, user.Avatar)
	})

	// Test WithAPI option with Create method
	t.Run("with_api_option_create", func(t *testing.T) {
		cli, err := client.New(baseAddr, client.WithToken(token), client.WithAPI("test-user"))
		require.NoError(t, err)

		newUser := User{
			Name:   "test_user",
			Email:  "test@example.com",
			Avatar: "test_avatar",
		}

		resp, err := cli.Create(newUser)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.TraceID)

		createdUser := new(User)
		err = json.Unmarshal(resp.Data, createdUser)
		require.NoError(t, err)
		require.NotEmpty(t, createdUser.ID)
		require.Equal(t, newUser.Name, createdUser.Name)
		require.Equal(t, newUser.Email, createdUser.Email)
		require.Equal(t, newUser.Avatar, createdUser.Avatar)

		// Clean up: delete the created user
		_, err = cli.Delete(createdUser.ID)
		require.NoError(t, err)
	})

	// Test WithAPI option with Update method
	t.Run("with_api_option_update", func(t *testing.T) {
		cli, err := client.New(baseAddr, client.WithToken(token), client.WithAPI("test-user"))
		require.NoError(t, err)

		// First create a user
		newUser := User{
			Name:   "update_test",
			Email:  "update@example.com",
			Avatar: "update_avatar",
		}
		resp, err := cli.Create(newUser)
		require.NoError(t, err)
		createdUser := new(User)
		err = json.Unmarshal(resp.Data, createdUser)
		require.NoError(t, err)

		// Update the user
		updatedUser := User{
			Name:   "updated_name",
			Email:  "updated@example.com",
			Avatar: "updated_avatar",
		}
		resp, err = cli.Update(createdUser.ID, updatedUser)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Verify update
		user := new(User)
		_, err = cli.Get(createdUser.ID, user)
		require.NoError(t, err)
		require.Equal(t, updatedUser.Name, user.Name)
		require.Equal(t, updatedUser.Email, user.Email)
		require.Equal(t, updatedUser.Avatar, user.Avatar)

		// Clean up
		_, err = cli.Delete(createdUser.ID)
		require.NoError(t, err)
	})

	// Test WithAPI option with Patch method
	t.Run("with_api_option_patch", func(t *testing.T) {
		cli, err := client.New(baseAddr, client.WithToken(token), client.WithAPI("test-user"))
		require.NoError(t, err)

		// First create a user
		newUser := User{
			Name:   "patch_test",
			Email:  "patch@example.com",
			Avatar: "patch_avatar",
		}
		resp, err := cli.Create(newUser)
		require.NoError(t, err)
		createdUser := new(User)
		err = json.Unmarshal(resp.Data, createdUser)
		require.NoError(t, err)

		// Partially update the user
		patchedUser := User{
			Avatar: "patched_avatar",
		}
		resp, err = cli.Patch(createdUser.ID, patchedUser)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Verify patch
		user := new(User)
		_, err = cli.Get(createdUser.ID, user)
		require.NoError(t, err)
		require.Equal(t, newUser.Name, user.Name)         // Name should remain unchanged
		require.Equal(t, newUser.Email, user.Email)       // Email should remain unchanged
		require.Equal(t, patchedUser.Avatar, user.Avatar) // Avatar should be updated

		// Clean up
		_, err = cli.Delete(createdUser.ID)
		require.NoError(t, err)
	})

	// Test WithAPI option with Delete method
	t.Run("with_api_option_delete", func(t *testing.T) {
		cli, err := client.New(baseAddr, client.WithToken(token), client.WithAPI("test-user"))
		require.NoError(t, err)

		// First create a user
		newUser := User{
			Name:   "delete_test",
			Email:  "delete@example.com",
			Avatar: "delete_avatar",
		}
		resp, err := cli.Create(newUser)
		require.NoError(t, err)
		createdUser := new(User)
		err = json.Unmarshal(resp.Data, createdUser)
		require.NoError(t, err)

		// Delete the user
		resp, err = cli.Delete(createdUser.ID)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Verify deletion
		user := new(User)
		_, err = cli.Get(createdUser.ID, user)
		require.Error(t, err)
	})

	// Test WithAPI option with query parameters
	t.Run("with_api_option_query", func(t *testing.T) {
		cli, err := client.New(baseAddr, client.WithToken(token), client.WithAPI("test-user"), client.WithQueryPagination(1, 2))
		require.NoError(t, err)

		users := make([]User, 0)
		total := new(int)
		resp, err := cli.List(&users, total)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.GreaterOrEqual(t, *total, 0)
	})
}

type User struct {
	Name   string `json:"name,omitempty"`
	Email  string `json:"email,omitempty"`
	Avatar string `json:"avatar,omitempty"`

	model.Query
	model.Base
}

func (u *User) GetTableName() string {
	return "test_users"
}

// Purge opts into hard delete so the fixed-id records can be recreated across
// sub-tests: soft-deleted rows would keep occupying the primary key and make
// later pure INSERTs fail with a duplicate error.
func (u *User) Purge() bool { return true }

func Test_Client_WithCookie(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "session=abc123", r.Header.Get("Cookie"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"","data":null,"trace_id":"test"}`))
	}))
	defer srv.Close()

	cli, err := client.New(srv.URL, client.WithCookie(&http.Cookie{
		Name:  "session",
		Value: "abc123",
	}))
	require.NoError(t, err)

	_, err = cli.Create(nil)
	require.NoError(t, err)
}
