package database_test

import (
	"os"
	"testing"

	"github.com/hydroan/gst/bootstrap"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

const (
	remarkUserCreateBefore = "user create before"
	remarkUserUpdateBefore = "user update before"
)

var (
	u1 = &TestUser{Name: "user1", Email: "user1@example.com", Age: 18, Base: model.Base{ID: "u1"}}
	u2 = &TestUser{Name: "user2", Email: "user2@example.com", Age: 19, Base: model.Base{ID: "u2"}}
	u3 = &TestUser{Name: "user3", Email: "user3@example.com", Age: 20, Base: model.Base{ID: "u3"}}

	ul = []*TestUser{u1, u2, u3}

	categoryRootID = "root"
	categoryRoot   = &TestCategory{
		Name:     categoryRootID,
		ParentID: categoryRootID, // parent is itself
		Base:     model.Base{ID: categoryRootID},
	}

	categoryParentID = "parent"
	categoryParent   = &TestCategory{
		Name:     categoryParentID,
		ParentID: categoryRootID, // parent is "root"
		Base:     model.Base{ID: categoryParentID},
	}
)

// cleanupTestData deletes test data from database and restores original values of test users.
// This function should be called in defer to ensure cleanup after each test.
func cleanupTestData() {
	users := make([]*TestUser, 0)
	_ = database.Database[*TestUser](nil).List(&users)
	_ = database.Database[*TestUser](nil).Delete(users...)
	// Restore original values
	u1 = &TestUser{Name: "user1", Email: "user1@example.com", Age: 18, Base: model.Base{ID: "u1"}}
	u2 = &TestUser{Name: "user2", Email: "user2@example.com", Age: 19, Base: model.Base{ID: "u2"}}
	u3 = &TestUser{Name: "user3", Email: "user3@example.com", Age: 20, Base: model.Base{ID: "u3"}}
	ul = []*TestUser{u1, u2, u3}

	categories := make([]*TestCategory, 0)
	err := database.Database[*TestCategory](nil).List(&categories)
	if err != nil {
		panic(err)
	}

	// disable foreign key check
	switch config.App.Database.Type {
	case config.DBMySQL:
		database.DB().Exec("SET FOREIGN_KEY_CHECKS=0")
	case config.DBPostgres:
		database.DB().Exec("SET CONSTRAINTS ALL DEFERRED")
	case config.DBSqlite:
		database.DB().Exec("PRAGMA foreign_keys = OFF")
	}
	defer func() {
		// enable foreign key check
		switch config.App.Database.Type {
		case config.DBMySQL:
			database.DB().Exec("SET FOREIGN_KEY_CHECKS=1")
		case config.DBSqlite:
			database.DB().Exec("PRAGMA foreign_keys = ON")
		}
	}()
	// delete all categories, we must temporarily disable foreign key check
	if err = database.Database[*TestCategory](nil).Delete(categories...); err != nil {
		panic(err)
	}

	products := make([]*TestProduct, 0)
	_ = database.Database[*TestProduct](nil).List(&products)
	_ = database.Database[*TestProduct](nil).Delete(products...)
}

// setupTestData deletes existing test data and creates all test users (ul).
// This is a common setup pattern used in most test cases.
func setupTestData(t *testing.T) {
	t.Helper()
	require.NoError(t, database.Database[*TestUser](nil).Delete(ul...))
	require.NoError(t, database.Database[*TestUser](nil).Create(ul...))
}

// findUsersByID finds users from a slice by their IDs and returns them in order (u1, u2, u3).
// Returns nil for users that are not found.
func findUsersByID(users []*TestUser) (u11, u22, u33 *TestUser) {
	for _, u := range users {
		switch u.ID {
		case u1.ID:
			u11 = u
		case u2.ID:
			u22 = u
		case u3.ID:
			u33 = u
		}
	}
	return
}

type TestUser struct {
	Name     string                      `json:"name"`
	Email    string                      `json:"email"`
	Age      int                         `json:"age"`
	Addr     datatypes.JSONSlice[string] `json:"addr"`
	IsActive *bool                       `json:"is_active"`
	Remark   *string                     `json:"remark,omitempty" gorm:"size:10240" schema:"remark"`

	model.Base
}

func (t *TestUser) Purge() bool { return true }
func (t *TestUser) CreateBefore(ctx *types.ModelContext) error {
	t.Remark = new(string(remarkUserCreateBefore))
	return nil
}

func (t *TestUser) UpdateBefore(ctx *types.ModelContext) error {
	t.Remark = new(string(remarkUserUpdateBefore))
	return nil
}

type TestUser2 struct {
	Name     string  `json:"name"`
	Email    string  `json:"email"`
	Age      int     `json:"age"`
	IsActive *bool   `json:"is_active"`
	Remark   *string `json:"remark,omitempty" gorm:"size:10240" schema:"remark"`

	model.Base
}

func (t *TestUser2) Purge() bool          { return true }
func (t *TestUser2) GetTableName() string { return "test_users" }

type TestProduct struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	CategoryID  string  `json:"category_id"`

	model.Base
}

func (*TestProduct) Purge() bool { return true }

type TestCategory struct {
	Name     string          `json:"name"`
	ParentID string          `json:"parent_id" gorm:"not null;index:idx_parent_id,length:191"`
	Children []*TestCategory `json:"children,omitempty" gorm:"foreignKey:ParentID"`
	Parent   *TestCategory   `json:"parent,omitempty" gorm:"foreignKey:ParentID;references:ID"`
	model.Base
}

func (*TestCategory) Purge() bool { return true }

func init() {
	os.Setenv(config.LOGGER_DIR, "/tmp/test_database")
	os.Setenv(config.DATABASE_TYPE, string(config.DBSqlite))
	os.Setenv(config.SQLITE_IS_MEMORY, "false")
	os.Setenv(config.SQLITE_PATH, "/tmp/test.db")
	_ = os.Remove("/tmp/test.db")

	os.Setenv(config.DATABASE_TYPE, string(config.DBMySQL))
	os.Setenv(config.MYSQL_DATABASE, "test")
	os.Setenv(config.MYSQL_USERNAME, "test")
	os.Setenv(config.MYSQL_PASSWORD, "test")

	// TODO: test for sqlite, mysql, postgresql

	model.Register[*TestUser]()
	model.Register[*TestProduct]()
	model.Register[*TestCategory]()

	// block here until database migration is ready
	dbruntime.Wait()

	if err := bootstrap.Bootstrap(); err != nil {
		panic(err)
	}
}
