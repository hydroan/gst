package database_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/tenant"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

const (
	remarkUserCreateBefore = "user create before"
	remarkUserUpdateBefore = "user update before"
)

// Column references shared by the option and write tests; test models carry
// no generated Cols vars.
var (
	colName      = types.NewColumn[string]("name")
	colEmail     = types.NewColumn[string]("email")
	colAge       = types.NewNumericColumn[int]("age")
	colStatus    = types.NewColumn[string]("status")
	colNotExists = types.NewColumn[string]("notexists")
)

var (
	errTestHookGroupCreateAfter = errors.New("test hook group create after failed")

	u1 = &TestUser{Name: "user1", Email: "user1@example.com", Age: 18, ID: "u1"}
	u2 = &TestUser{Name: "user2", Email: "user2@example.com", Age: 19, ID: "u2"}
	u3 = &TestUser{Name: "user3", Email: "user3@example.com", Age: 20, ID: "u3"}

	ul = []*TestUser{u1, u2, u3}

	categoryRootID = "root"
	categoryRoot   = &TestCategory{
		Name:     categoryRootID,
		ParentID: categoryRootID, // parent is itself
		ID:       categoryRootID,
	}

	categoryParentID = "parent"
	categoryParent   = &TestCategory{
		Name:     categoryParentID,
		ParentID: categoryRootID, // parent is "root"
		ID:       categoryParentID,
	}
)

// requireRuntimeStack asserts that err carries a run-time stack trace, per
// the error-stack contract in doc.go: every first-hand exit of a stack-less
// GORM/driver/sentinel error embeds the stack via errors.WithStack, so the
// error_stack log field can locate the call site without call-site logging.
func requireRuntimeStack(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	require.NotNil(t, errors.GetReportableStackTrace(err),
		"error must carry a run-time stack captured at its first-hand database exit")
}

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
	u1 = &TestUser{Name: "user1", Email: "user1@example.com", Age: 18, ID: "u1"}
	u2 = &TestUser{Name: "user2", Email: "user2@example.com", Age: 19, ID: "u2"}
	u3 = &TestUser{Name: "user3", Email: "user3@example.com", Age: 20, ID: "u3"}
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

	items := make([]*TestItem, 0)
	_ = database.Database[*TestItem](context.Background()).List(&items)
	_ = database.Database[*TestItem](context.Background()).Delete(items...)

	plainItems := make([]*TestPlainItem, 0)
	_ = database.Database[*TestPlainItem](context.Background()).List(&plainItems)
	_ = database.Database[*TestPlainItem](context.Background()).Delete(plainItems...)

	uniqueItems := make([]*TestUniqueItem, 0)
	_ = database.Database[*TestUniqueItem](context.Background()).List(&uniqueItems)
	_ = database.Database[*TestUniqueItem](context.Background()).Delete(uniqueItems...)

	indexerUniqueItems := make([]*TestIndexerUniqueItem, 0)
	_ = database.Database[*TestIndexerUniqueItem](context.Background()).List(&indexerUniqueItems)
	_ = database.Database[*TestIndexerUniqueItem](context.Background()).Delete(indexerUniqueItems...)

	mixedUniqueItems := make([]*TestMixedUniqueItem, 0)
	_ = database.Database[*TestMixedUniqueItem](context.Background()).List(&mixedUniqueItems)
	_ = database.Database[*TestMixedUniqueItem](context.Background()).Delete(mixedUniqueItems...)

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

// quoteIdent renders an identifier the way the dialect under test quotes it,
// so SQL assertions stay dialect-neutral: backticks on mysql and sqlite,
// double quotes on postgres, mirroring the gorm dialectors.
func quoteIdent(name string) string {
	if config.App.Database.Type == config.DBPostgres {
		return `"` + name + `"`
	}
	return "`" + name + "`"
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

func (t *TestUser) TableName() string { return "test_users" }
func (t *TestUser) Purge() bool       { return true }
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

func (t *TestUser2) Purge() bool       { return true }
func (t *TestUser2) TableName() string { return "test_users" }

type TestItem struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Score       float64 `json:"score"`
	GroupID     string  `json:"group_id"`

	model.Base
}

func (*TestItem) TableName() string { return "test_items" }
func (*TestItem) Purge() bool       { return true }

type TestPlainItem struct {
	Code          string `json:"code" gorm:"size:191"`
	Name          string `json:"name" gorm:"size:191"`
	CreateAfterID string `json:"-" gorm:"-"`

	model.Base
}

func (*TestPlainItem) TableName() string { return "test_plain_items" }
func (*TestPlainItem) Purge() bool       { return true }

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

func (*TestUniqueItem) TableName() string { return "test_unique_items" }
func (*TestUniqueItem) Purge() bool       { return true }

func (i *TestUniqueItem) CreateAfter(ctx context.Context) error {
	i.CreateAfterID = i.ID
	return nil
}

func (i *TestUniqueItem) UpdateAfter(ctx context.Context) error {
	i.UpdateAfterID = i.ID
	return nil
}

// TestIndexerUniqueItem declares its composite unique key only through the
// Indexes method: the struct tags carry no index at all, mirroring models
// whose secondary indexes moved off gorm tags entirely.
type TestIndexerUniqueItem struct {
	Code string `json:"code" gorm:"size:191"`
	Kind string `json:"kind" gorm:"size:191"`
	Name string `json:"name" gorm:"size:191"`

	model.Base
}

func (*TestIndexerUniqueItem) TableName() string { return "test_indexer_unique_items" }
func (*TestIndexerUniqueItem) Purge() bool       { return true }

// Indexes declares the composite unique key on (Code, Kind).
func (*TestIndexerUniqueItem) Indexes() []model.Index {
	return []model.Index{{Fields: []string{"Code", "Kind"}, Unique: true}}
}

// TestMixedUniqueItem carries one unique key in a struct tag and a second one
// on another column through the Indexes method, so a collector reading only
// one of the two sources loses a key.
type TestMixedUniqueItem struct {
	Code string `json:"code" gorm:"size:191;uniqueIndex"`
	Ref  string `json:"ref" gorm:"size:191"`
	Name string `json:"name" gorm:"size:191"`

	model.Base
}

func (*TestMixedUniqueItem) TableName() string { return "test_mixed_unique_items" }
func (*TestMixedUniqueItem) Purge() bool       { return true }

// Indexes declares the unique key on Ref, next to the tag-declared one on Code.
func (*TestMixedUniqueItem) Indexes() []model.Index {
	return []model.Index{{Fields: []string{"Ref"}, Unique: true}}
}

type TestAutoItem struct {
	Code string `json:"code" gorm:"size:191"`
	Name string `json:"name" gorm:"size:191"`

	model.AutoBase
}

func (*TestAutoItem) TableName() string { return "test_auto_items" }
func (*TestAutoItem) Purge() bool       { return true }

// TestSoftDeleteItem keeps the model.Base default Purge (soft delete) so write
// tests can assert how writes treat soft-deleted rows. Its table is migrated
// on demand inside the tests that need it and cleaned up with raw SQL because
// soft-deleted rows are invisible to List.
type TestSoftDeleteItem struct {
	Code string `json:"code" gorm:"size:191;uniqueIndex"`
	Name string `json:"name" gorm:"size:191"`

	model.Base
}

func (*TestSoftDeleteItem) TableName() string { return "test_soft_delete_items" }

// TestTenantSoftDeleteItem is tenant-scoped and keeps the model.Base default
// Purge (soft delete), so the WithDeleted tests can assert that lifting the
// soft-delete condition never lifts tenant scoping: the tenant predicate
// ignores GORM's Unscoped flag by design (see tenant.ID). Its table is
// migrated on demand and cleaned up with raw SQL like TestSoftDeleteItem.
type TestTenantSoftDeleteItem struct {
	Name string `json:"name" gorm:"size:191"`

	tenant.Scope
	model.Base
}

func (*TestTenantSoftDeleteItem) TableName() string { return "test_tenant_soft_delete_items" }

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

func (*TestAggregateRecord) TableName() string { return "test_aggregate_records" }

// TestRecordTag is the related model of TestAggregateRecord, used by the
// correlated-subquery filters. It soft deletes like its parent, so the tests
// can assert that a subquery hides the same rows a List on it hides. Category
// denormalizes the record's category onto the tag, giving the composite-key
// tests a second column to correlate on.
type TestRecordTag struct {
	RecordID string `json:"record_id" gorm:"size:191"`
	Label    string `json:"label" gorm:"size:191"`
	Category string `json:"category" gorm:"size:191"`

	model.Base
}

func (*TestRecordTag) TableName() string { return "test_record_tags" }

// TestTagAlias points at the same table as TestRecordTag while its struct
// name would derive a different one ("test_tag_aliases"). gorm and the
// framework read the same TableName method, so the subquery's FROM and its
// correlation prefix must both resolve to the declared table rather than
// anything derived from the struct name.
type TestTagAlias struct {
	RecordID string `json:"record_id" gorm:"size:191"`
	Label    string `json:"label" gorm:"size:191"`

	model.Base
}

func (*TestTagAlias) TableName() string { return "test_record_tags" }

// TestTagNote is the grandchild used by the nested-subquery test: a subquery
// inside a subquery must correlate against the table directly enclosing it.
type TestTagNote struct {
	TagID string `json:"tag_id" gorm:"size:191"`
	Body  string `json:"body" gorm:"size:191"`

	model.Base
}

func (*TestTagNote) TableName() string { return "test_tag_notes" }

type TestHookConfig struct {
	Value string `json:"value" gorm:"size:191"`

	model.Base
}

func (*TestHookConfig) TableName() string { return "test_hook_configs" }
func (*TestHookConfig) Purge() bool       { return true }

type TestHookGroup struct {
	ConfigID string `json:"config_id" gorm:"size:191"`
	Value    string `json:"value" gorm:"size:191"`

	model.Base
}

func (*TestHookGroup) TableName() string { return "test_hook_groups" }
func (*TestHookGroup) Purge() bool       { return true }

func (g *TestHookGroup) CreateAfter(ctx context.Context) error {
	if strings.TrimSpace(g.ConfigID) == "" {
		return nil
	}
	if err := database.Database[*TestHookConfig](ctx).UpdateByID(g.ConfigID, types.Assign("value", g.Value)); err != nil {
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

func (*TestCategory) TableName() string { return "test_categories" }
func (*TestCategory) Purge() bool       { return true }

// envTestDatabase overrides the dialect this suite runs against. It is a
// contract between this TestMain and the Makefile test target, which repeats
// the package once per dialect. testutil knows nothing about the variable, so
// projects built on the public testutil keep full control of their own
// Server.Database.
const envTestDatabase = "GST_TEST_DATABASE"

// TestMain runs the suite against MySQL, the framework's primary dialect, by
// default, and against the dialect envTestDatabase names when it is set. An
// unsupported value fails the run through the Server.Database validation.
// Every test in this package must either behave identically across dialects
// or branch on config.App.Database.Type where a per-dialect contract differs
// (the Upsert collision test is the pattern). A dialect broken by an open bug
// takes a t.Skip carrying the bug number, so the account stays greppable
// until the fix lands.
func TestMain(m *testing.M) {
	dbType := config.DBMySQL
	if override := os.Getenv(envTestDatabase); len(override) > 0 {
		dbType = config.DBType(override)
	}
	testutil.Run(m, testutil.Server{
		Database: dbType,
		Register: func() {
			model.Register[*TestUser]()
			model.Register[*TestItem]()
			model.Register[*TestPlainItem]()
			model.Register[*TestUniqueItem]()
			model.Register[*TestIndexerUniqueItem]()
			model.Register[*TestMixedUniqueItem]()
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
