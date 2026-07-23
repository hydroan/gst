package dbmigrate_test

import (
	"strings"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/dbmigrate"
	"github.com/hydroan/gst/model"
	"github.com/stretchr/testify/require"
)

type User struct {
	Username string `json:"username"`
	Addr     string `json:"addr"`

	model.Base
}

type Group struct {
	Name string `json:"name"`

	model.Base
}

func TestDumper(t *testing.T) {
	dumper, err := dbmigrate.NewSchemaDumper()
	require.NoError(t, err)
	defer dumper.Close()

	t.Run("mysql", func(t *testing.T) {
		schema, err := dumper.Dump(config.DBMySQL, User{}, &Group{})
		require.NoError(t, err)
		require.NotEmpty(t, schema)
		require.Contains(t, schema, "-- Model: dbmigrate_test.Group\nCREATE TABLE `groups`")
		require.Contains(t, schema, "-- Model: dbmigrate_test.User\nCREATE TABLE `users`")
		requireOnlyBaseSoftDeleteIndex(t, schema, "groups")
		requireOnlyBaseSoftDeleteIndex(t, schema, "users")
		require.NotContains(t, schema, "DROP TABLE IF EXISTS")
		// fmt.Println(schema)
	})

	t.Run("postgres", func(t *testing.T) {
		schema, err := dumper.Dump(config.DBPostgres, User{}, &Group{})
		require.NoError(t, err)
		require.NotEmpty(t, schema)
		require.Contains(t, schema, "-- Model: dbmigrate_test.Group\nCREATE TABLE \"groups\"")
		require.Contains(t, schema, "-- Model: dbmigrate_test.User\nCREATE TABLE \"users\"")
		require.NotContains(t, schema, "-- Model: dbmigrate_test.Group\nCREATE INDEX")
		require.NotContains(t, schema, "-- Model: dbmigrate_test.User\nCREATE INDEX")
		require.Contains(t, schema, `CREATE TABLE "groups"`)
		require.NotContains(t, schema, "ENGINE = InnoDB")
		require.NotContains(t, schema, "CREATE TABLE `groups`")
		// fmt.Println(schema)
		t.Log(schema)
	})

	t.Run("sqlite", func(t *testing.T) {
		schema, err := dumper.Dump(config.DBSqlite, User{}, &Group{})
		require.NoError(t, err)
		require.NotEmpty(t, schema)
		// fmt.Println(schema)
	})
}

func requireOnlyBaseSoftDeleteIndex(t *testing.T, schema, table string) {
	t.Helper()

	require.Contains(t, schema, "INDEX `idx_"+table+"_deleted_at` (`deleted_at`)")
	require.NotContains(t, schema, "idx_"+table+"_created_by")
	require.NotContains(t, schema, "idx_"+table+"_updated_by")
	require.NotContains(t, schema, "idx_"+table+"_created_at")
	require.NotContains(t, schema, "idx_"+table+"_updated_at")
}

func TestDumpOrder(t *testing.T) {
	dumper, err := dbmigrate.NewSchemaDumper()
	require.NoError(t, err)
	defer dumper.Close()

	// reflect.TypeOf(User{}).String() is "dbmigrate_test.User"
	// reflect.TypeOf(&Group{}).String() is "*dbmigrate_test.Group"
	// '*' (42) < 'd' (100), so &Group{} should be first.
	// However, if we use pointers for both:
	// *dbmigrate_test.Group vs *dbmigrate_test.User
	// G < U, so Group first.

	t.Run("stable_order", func(t *testing.T) {
		// Pass User then Group (as pointers)
		// Expected order: Group (sorted first), then User
		schema, err := dumper.Dump(config.DBMySQL, &User{}, &Group{})
		require.NoError(t, err)

		idxGroup := strings.Index(schema, "CREATE TABLE `groups`")
		idxUser := strings.Index(schema, "CREATE TABLE `users`")

		require.NotEqual(t, -1, idxGroup)
		require.NotEqual(t, -1, idxUser)
		require.Less(t, idxGroup, idxUser, "Group should appear before User because *...Group < *...User")
	})
}

// Article declares custom indexes through the Indexer capability while also
// carrying a plain struct tag column.
type Article struct {
	Title string `json:"title"`
	Tag   string `json:"tag"`

	model.Base
}

func (*Article) GetTableName() string { return "articles" }

func (*Article) Indexes() []model.Index {
	return []model.Index{
		{Fields: []string{"Tag", "CreatedAt"}},
		{Fields: []string{"Title"}, Unique: true},
	}
}

func TestDumperCustomIndexes(t *testing.T) {
	dumper, err := dbmigrate.NewSchemaDumper()
	require.NoError(t, err)
	defer dumper.Close()

	schema, err := dumper.Dump(config.DBMySQL, &Article{})
	require.NoError(t, err)
	require.Contains(t, schema, "CREATE TABLE `articles`")
	require.Contains(t, schema, "idx_articles_tag_created_at")
	require.Contains(t, schema, "uniq_articles_title")
	// Index statements must come after the CREATE TABLE they belong to.
	require.Less(t,
		strings.Index(schema, "CREATE TABLE `articles`"),
		strings.Index(schema, "idx_articles_tag_created_at"))

	// The same plans render with dialect-specific quoting on postgres.
	schema, err = dumper.Dump(config.DBPostgres, &Article{})
	require.NoError(t, err)
	require.Contains(t, schema, "idx_articles_tag_created_at")
	require.Contains(t, schema, "uniq_articles_title")
}
