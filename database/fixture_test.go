package database_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/model"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

const (
	remarkUserCreateBefore = "user create before"
	remarkUserUpdateBefore = "user update before"
)

var (
	errTestHookGroupCreateAfter = errors.New("test hook group create after failed")

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
	_ = database.Database[*TestUser](context.Background()).List(&users)
	_ = database.Database[*TestUser](context.Background()).Delete(users...)
	// Purge soft-deleted leftovers: List cannot see them, but they keep
	// occupying primary/unique keys and would break later pure INSERTs.
	_ = database.DB().Exec("DELETE FROM test_users").Error
	// Restore original values
	u1 = &TestUser{Name: "user1", Email: "user1@example.com", Age: 18, Base: model.Base{ID: "u1"}}
	u2 = &TestUser{Name: "user2", Email: "user2@example.com", Age: 19, Base: model.Base{ID: "u2"}}
	u3 = &TestUser{Name: "user3", Email: "user3@example.com", Age: 20, Base: model.Base{ID: "u3"}}
	ul = []*TestUser{u1, u2, u3}

	categories := make([]*TestCategory, 0)
	err := database.Database[*TestCategory](context.Background()).List(&categories)
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
	if err = database.Database[*TestCategory](context.Background()).Delete(categories...); err != nil {
		panic(err)
	}

	products := make([]*TestProduct, 0)
	_ = database.Database[*TestProduct](context.Background()).List(&products)
	_ = database.Database[*TestProduct](context.Background()).Delete(products...)

	plainItems := make([]*TestPlainItem, 0)
	_ = database.Database[*TestPlainItem](context.Background()).List(&plainItems)
	_ = database.Database[*TestPlainItem](context.Background()).Delete(plainItems...)

	uniqueItems := make([]*TestUniqueItem, 0)
	_ = database.Database[*TestUniqueItem](context.Background()).List(&uniqueItems)
	_ = database.Database[*TestUniqueItem](context.Background()).Delete(uniqueItems...)

	autoItems := make([]*TestAutoItem, 0)
	_ = database.Database[*TestAutoItem](context.Background()).List(&autoItems)
	_ = database.Database[*TestAutoItem](context.Background()).Delete(autoItems...)

	hookGroups := make([]*TestHookGroup, 0)
	_ = database.Database[*TestHookGroup](context.Background()).List(&hookGroups)
	_ = database.Database[*TestHookGroup](context.Background()).Delete(hookGroups...)

	hookConfigs := make([]*TestHookConfig, 0)
	_ = database.Database[*TestHookConfig](context.Background()).List(&hookConfigs)
	_ = database.Database[*TestHookConfig](context.Background()).Delete(hookConfigs...)
}

// setupTestData deletes existing test data and creates all test users (ul).
// This is a common setup pattern used in most test cases.
func setupTestData(t *testing.T) {
	t.Helper()
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete(ul...))
	require.NoError(t, database.Database[*TestUser](context.Background()).Create(ul...))
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
	Remark   *string                     `json:"remark,omitempty" gorm:"size:10240" query:"remark"`

	model.Base
}

func (t *TestUser) Purge() bool { return true }
func (t *TestUser) CreateBefore(ctx context.Context) error {
	t.Remark = new(string(remarkUserCreateBefore))
	return nil
}

func (t *TestUser) UpdateBefore(ctx context.Context) error {
	t.Remark = new(string(remarkUserUpdateBefore))
	return nil
}

type TestUser2 struct {
	Name     string  `json:"name"`
	Email    string  `json:"email"`
	Age      int     `json:"age"`
	IsActive *bool   `json:"is_active"`
	Remark   *string `json:"remark,omitempty" gorm:"size:10240" query:"remark"`

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

type TestPlainItem struct {
	Code          string `json:"code" gorm:"size:191"`
	Name          string `json:"name" gorm:"size:191"`
	CreateAfterID string `json:"-" gorm:"-"`

	model.Base
}

func (*TestPlainItem) Purge() bool { return true }

func (i *TestPlainItem) CreateAfter(ctx context.Context) error {
	i.CreateAfterID = i.ID
	return nil
}

type TestUniqueItem struct {
	UniqueCode    string `json:"unique_code" gorm:"size:191;uniqueIndex"`
	Name          string `json:"name" gorm:"size:191"`
	CreateAfterID string `json:"-" gorm:"-"`
	UpdateAfterID string `json:"-" gorm:"-"`

	model.Base
}

func (*TestUniqueItem) Purge() bool { return true }

func (i *TestUniqueItem) CreateAfter(ctx context.Context) error {
	i.CreateAfterID = i.ID
	return nil
}

func (i *TestUniqueItem) UpdateAfter(ctx context.Context) error {
	i.UpdateAfterID = i.ID
	return nil
}

type TestAutoItem struct {
	Code string `json:"code" gorm:"size:191"`
	Name string `json:"name" gorm:"size:191"`

	model.AutoBase
}

func (*TestAutoItem) Purge() bool { return true }

// TestSoftDeleteItem keeps the model.Base default Purge (soft delete) so write
// tests can assert how writes treat soft-deleted rows. Its table is migrated
// on demand inside the tests that need it and cleaned up with raw SQL because
// soft-deleted rows are invisible to List.
type TestSoftDeleteItem struct {
	Code string `json:"code" gorm:"size:191;uniqueIndex"`
	Name string `json:"name" gorm:"size:191"`

	model.Base
}

// TestAggregateRecord is the fixture for aggregate reads: a group key, a
// second dimension for conditional aggregation, a numeric measure, a float
// measure, a caller-controlled timestamp for bucketing and a nullable
// timestamp for the NULL-safety rule on result rows. The seed never fills
// ClosedAt; it exists so the tests can point a measure at a column that can
// hold NULL.
//
// It deliberately keeps the model.Base default Purge, so its rows soft delete.
// That is what lets the aggregate tests assert the rule an aggregate is most
// likely to break: scanning into a plain result row parses no model, so
// without the model the soft-delete condition disappears and an aggregate
// counts rows a List on the same model hides.
type TestAggregateRecord struct {
	Category   string     `json:"category" gorm:"size:191"`
	Status     string     `json:"status" gorm:"size:191"`
	Amount     int64      `json:"amount"`
	Score      float64    `json:"score"`
	OccurredAt time.Time  `json:"occurred_at"`
	ClosedAt   *time.Time `json:"closed_at"`

	model.Base
}

// TestRecordTag is the related model of TestAggregateRecord, used by the
// correlated-subquery filters. It soft deletes like its parent, so the tests
// can assert that a subquery hides the same rows a List on it hides.
type TestRecordTag struct {
	RecordID string `json:"record_id" gorm:"size:191"`
	Label    string `json:"label" gorm:"size:191"`

	model.Base
}

// TestTagAlias points at the same table as TestRecordTag but under a struct
// name gorm would derive a different table from ("test_tag_aliases"). gorm
// reads its own TableName method, not the framework's GetTableName, so a
// subquery that takes its FROM from the struct and its correlation prefix from
// GetTableName would name two different tables. It deliberately declares only
// the framework method, which is what every model in a real project does.
type TestTagAlias struct {
	RecordID string `json:"record_id" gorm:"size:191"`
	Label    string `json:"label" gorm:"size:191"`

	model.Base
}

func (*TestTagAlias) GetTableName() string { return "test_record_tags" }

// TestTagNote is the grandchild used by the nested-subquery test: a subquery
// inside a subquery must correlate against the table directly enclosing it.
type TestTagNote struct {
	TagID string `json:"tag_id" gorm:"size:191"`
	Body  string `json:"body" gorm:"size:191"`

	model.Base
}

type TestHookConfig struct {
	Value string `json:"value" gorm:"size:191"`

	model.Base
}

func (*TestHookConfig) Purge() bool { return true }

type TestHookGroup struct {
	ConfigID string `json:"config_id" gorm:"size:191"`
	Value    string `json:"value" gorm:"size:191"`

	model.Base
}

func (*TestHookGroup) Purge() bool { return true }

func (g *TestHookGroup) CreateAfter(ctx context.Context) error {
	if strings.TrimSpace(g.ConfigID) == "" {
		return nil
	}
	if err := database.Database[*TestHookConfig](ctx).UpdateByID(g.ConfigID, "value", g.Value); err != nil {
		return err
	}
	return errTestHookGroupCreateAfter
}

type TestCategory struct {
	Name     string          `json:"name"`
	ParentID string          `json:"parent_id" gorm:"size:191;not null;index:idx_parent_id"`
	Children []*TestCategory `json:"children,omitempty" gorm:"foreignKey:ParentID"`
	Parent   *TestCategory   `json:"parent,omitempty" gorm:"foreignKey:ParentID;references:ID"`
	model.Base
}

func (*TestCategory) Purge() bool { return true }

// TODO: test for sqlite, mysql, postgresql
func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Database: config.DBMySQL,
		Register: func() {
			model.Register[*TestUser]()
			model.Register[*TestProduct]()
			model.Register[*TestPlainItem]()
			model.Register[*TestUniqueItem]()
			model.Register[*TestAutoItem]()
			model.Register[*TestHookConfig]()
			model.Register[*TestHookGroup]()
			model.Register[*TestCategory]()
			model.Register[*TestAggregateRecord]()
			model.Register[*TestRecordTag]()
			model.Register[*TestTagNote]()
		},
	})
}
