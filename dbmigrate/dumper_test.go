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

func (*User) TableName() string { return "users" }

type Group struct {
	Name string `json:"name"`

	model.Base
}

func (*Group) TableName() string { return "groups" }

// Sample declares custom indexes through the Indexer capability while also
// carrying a plain struct tag column. The indexed columns carry a size:
// without one gorm maps them to TEXT on MySQL, which cannot be indexed
// whole (key length error 1170).
type Sample struct {
	Title string `json:"title" gorm:"size:191"`
	Tag   string `json:"tag" gorm:"size:191"`

	model.Base
}

func (*Sample) TableName() string { return "samples" }

func (*Sample) Indexes() []model.Index {
	return []model.Index{
		{Fields: []string{"Tag", "CreatedAt"}},
		{Fields: []string{"Title"}, Unique: true},
	}
}

// ConflictSampleA and ConflictSampleB declare an index over the same column
// sequence of one table; Dump must reject the pair instead of rendering the
// statements of both.
type ConflictSampleA struct {
	Kind string `json:"kind" gorm:"size:191"`

	model.Base
}

func (*ConflictSampleA) TableName() string { return "conflict_samples" }

func (*ConflictSampleA) Indexes() []model.Index {
	return []model.Index{{Fields: []string{"Kind"}}}
}

type ConflictSampleB struct {
	Kind string `json:"kind" gorm:"size:191"`

	model.Base
}

func (*ConflictSampleB) TableName() string { return "conflict_samples" }

func (*ConflictSampleB) Indexes() []model.Index {
	return []model.Index{{Fields: []string{"Kind"}, Unique: true}}
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
		requireNoBaseSecondaryIndexes(t, schema, "groups")
		requireNoBaseSecondaryIndexes(t, schema, "users")
		require.NotContains(t, schema, "DROP TABLE IF EXISTS")
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
	})

	t.Run("sqlite", func(t *testing.T) {
		schema, err := dumper.Dump(config.DBSqlite, User{}, &Group{})
		require.NoError(t, err)
		require.NotEmpty(t, schema)
	})
}

func TestDumperOrder(t *testing.T) {
	dumper, err := dbmigrate.NewSchemaDumper()
	require.NoError(t, err)
	defer dumper.Close()

	// Models render sorted by type name, whatever order they arrive in:
	// "*dbmigrate_test.Group" < "*dbmigrate_test.User".
	schema, err := dumper.Dump(config.DBMySQL, &User{}, &Group{})
	require.NoError(t, err)

	idxGroup := strings.Index(schema, "CREATE TABLE `groups`")
	idxUser := strings.Index(schema, "CREATE TABLE `users`")

	require.NotEqual(t, -1, idxGroup)
	require.NotEqual(t, -1, idxUser)
	require.Less(t, idxGroup, idxUser, "Group should appear before User because *...Group < *...User")
}

func TestDumperCustomIndexes(t *testing.T) {
	dumper, err := dbmigrate.NewSchemaDumper()
	require.NoError(t, err)
	defer dumper.Close()

	schema, err := dumper.Dump(config.DBMySQL, &Sample{})
	require.NoError(t, err)
	require.Contains(t, schema, "CREATE TABLE `samples`")
	require.Contains(t, schema, "idx_samples_tag_created_at")
	require.Contains(t, schema, "uniq_samples_title")
	// Index statements must come after the CREATE TABLE they belong to.
	require.Less(t,
		strings.Index(schema, "CREATE TABLE `samples`"),
		strings.Index(schema, "idx_samples_tag_created_at"))

	// The same plans render with dialect-specific quoting on postgres.
	schema, err = dumper.Dump(config.DBPostgres, &Sample{})
	require.NoError(t, err)
	require.Contains(t, schema, "idx_samples_tag_created_at")
	require.Contains(t, schema, "uniq_samples_title")
}

func TestDumperKeepsCallerOrder(t *testing.T) {
	dumper, err := dbmigrate.NewSchemaDumper()
	require.NoError(t, err)
	defer dumper.Close()

	// Callers expand a slice of their own into dst, so the deterministic
	// output order must come from a copy and leave that slice untouched.
	user, group := &User{}, &Group{}
	models := []any{user, group}
	schema, err := dumper.Dump(config.DBMySQL, models...)
	require.NoError(t, err)
	require.NotEmpty(t, schema)
	require.Equal(t, []any{user, group}, models)
}

func TestDumperClosed(t *testing.T) {
	dumper, err := dbmigrate.NewSchemaDumper()
	require.NoError(t, err)
	// The mock connection carries no ExpectClose, so Close reports the call as
	// unexpected; this test only covers what Dump does once it is closed.
	_ = dumper.Close()

	_, err = dumper.Dump(config.DBMySQL, &User{})
	require.ErrorContains(t, err, "schema dumper is closed")
}

func TestDumperRejectsCrossModelIndexConflicts(t *testing.T) {
	dumper, err := dbmigrate.NewSchemaDumper()
	require.NoError(t, err)
	defer dumper.Close()

	_, err = dumper.Dump(config.DBMySQL, &ConflictSampleA{}, &ConflictSampleB{})
	require.ErrorContains(t, err, `conflict on table "conflict_samples"`)
	require.ErrorContains(t, err, "ConflictSampleA")
	require.ErrorContains(t, err, "ConflictSampleB")
}

// requireNoBaseSecondaryIndexes asserts that the base struct contributes no
// secondary index of its own: the primary key is the framework's only
// tag-declared index, and everything else is a model's explicit Indexes()
// decision.
func requireNoBaseSecondaryIndexes(t *testing.T, schema, table string) {
	t.Helper()

	require.NotContains(t, schema, "idx_"+table+"_deleted_at")
	require.NotContains(t, schema, "idx_"+table+"_created_by")
	require.NotContains(t, schema, "idx_"+table+"_updated_by")
	require.NotContains(t, schema, "idx_"+table+"_created_at")
	require.NotContains(t, schema, "idx_"+table+"_updated_at")
}
