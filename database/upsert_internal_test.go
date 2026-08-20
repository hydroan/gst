package database

import (
	"context"
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
	gormschema "gorm.io/gorm/schema"
)

// The collect fixtures below are parsed through a DryRun sqlite instance;
// none of them registers a model or touches a database.

type syncCollectMergedItem struct {
	Code string `gorm:"size:191;uniqueIndex"`
	Ref  string `gorm:"size:191"`

	model.Base
}

func (*syncCollectMergedItem) TableName() string { return "sync_collect_merged_items" }

// Indexes declares the unique key on Ref, next to the tag-declared one on Code.
func (*syncCollectMergedItem) Indexes() []model.Index {
	return []model.Index{{Fields: []string{"Ref"}, Unique: true}}
}

type syncCollectCompositeItem struct {
	Code string `gorm:"size:191"`
	Kind string `gorm:"size:191"`

	model.Base
}

func (*syncCollectCompositeItem) TableName() string { return "sync_collect_composite_items" }

// Indexes declares a composite unique key whose column order must survive.
func (*syncCollectCompositeItem) Indexes() []model.Index {
	return []model.Index{{Fields: []string{"Kind", "Code"}, Unique: true}}
}

type syncCollectPlainIndexItem struct {
	Code string `gorm:"size:191"`

	model.Base
}

func (*syncCollectPlainIndexItem) TableName() string { return "sync_collect_plain_index_items" }

// Indexes declares a non-unique index, which the sync must ignore.
func (*syncCollectPlainIndexItem) Indexes() []model.Index {
	return []model.Index{{Fields: []string{"Code"}}}
}

type syncCollectPrimaryOnlyItem struct {
	model.Base
}

func (*syncCollectPrimaryOnlyItem) TableName() string { return "sync_collect_primary_only_items" }

// Indexes declares a unique key covering only the primary key: a primary-key
// merge keeps the caller's id, so the sync must drop the declaration.
func (*syncCollectPrimaryOnlyItem) Indexes() []model.Index {
	return []model.Index{{Fields: []string{"ID"}, Unique: true}}
}

// newSyncCollectDB opens the DryRun sqlite instance the collect tests parse
// their fixtures through.
func newSyncCollectDB(t *testing.T) *gorm.DB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DryRun: true,
		Logger: glogger.Default.LogMode(glogger.Silent),
	})
	require.NoError(t, err)
	return gormDB
}

// syncCollectInputs resolves the parsed schema and the index plans of m the
// way saveResultSyncUniqueIndexes does before collecting.
func syncCollectInputs(t *testing.T, gormDB *gorm.DB, m any) (*gormschema.Schema, []modelregistry.IndexPlan) {
	t.Helper()
	stmt := &gorm.Statement{DB: gormDB}
	require.NoError(t, stmt.Parse(m))
	plans, err := modelregistry.ParseIndexPlans(gormDB, m)
	require.NoError(t, err)
	return stmt.Schema, plans
}

// indexColumns flattens an index to its column names in index order.
func indexColumns(index *gormschema.Index) []string {
	columns := make([]string, 0, len(index.Fields))
	for _, field := range index.Fields {
		columns = append(columns, field.Field.DBName)
	}
	return columns
}

func TestCollectSaveResultSyncUniqueIndexes(t *testing.T) {
	gormDB := newSyncCollectDB(t)

	t.Run("merges tag and Indexes sources", func(t *testing.T) {
		schema, plans := syncCollectInputs(t, gormDB, &syncCollectMergedItem{})
		indexes, err := collectSaveResultSyncUniqueIndexes(schema, plans)
		require.NoError(t, err)
		require.Len(t, indexes, 2, "one index per declaration source")

		require.Equal(t, []string{"code"}, indexColumns(indexes[0]), "the tag-declared key comes first")
		require.Equal(t, "UNIQUE", indexes[0].Class)

		require.Equal(t, []string{"ref"}, indexColumns(indexes[1]), "the Indexes-declared key follows")
		require.Equal(t, "UNIQUE", indexes[1].Class)
		require.Equal(t, "uniq_sync_collect_merged_items_ref", indexes[1].Name, "plan indexes keep the framework-generated name")
	})

	t.Run("keeps the declared composite column order", func(t *testing.T) {
		schema, plans := syncCollectInputs(t, gormDB, &syncCollectCompositeItem{})
		indexes, err := collectSaveResultSyncUniqueIndexes(schema, plans)
		require.NoError(t, err)
		require.Len(t, indexes, 1)
		require.Equal(t, []string{"kind", "code"}, indexColumns(indexes[0]))
	})

	t.Run("ignores non-unique plans", func(t *testing.T) {
		schema, plans := syncCollectInputs(t, gormDB, &syncCollectPlainIndexItem{})
		indexes, err := collectSaveResultSyncUniqueIndexes(schema, plans)
		require.NoError(t, err)
		require.Empty(t, indexes)
	})

	t.Run("drops declarations covering only primary key columns", func(t *testing.T) {
		schema, plans := syncCollectInputs(t, gormDB, &syncCollectPrimaryOnlyItem{})
		indexes, err := collectSaveResultSyncUniqueIndexes(schema, plans)
		require.NoError(t, err)
		require.Empty(t, indexes)
	})

	t.Run("fails on a plan column the schema does not carry", func(t *testing.T) {
		schema, _ := syncCollectInputs(t, gormDB, &syncCollectMergedItem{})
		_, err := collectSaveResultSyncUniqueIndexes(schema, []modelregistry.IndexPlan{
			{Name: "uniq_sync_collect_merged_items_ghost", Table: "sync_collect_merged_items", Columns: []string{"ghost"}, Unique: true},
		})
		require.ErrorContains(t, err, `column "ghost"`)
	})
}

func TestSaveResultSyncUniqueIndexesCached(t *testing.T) {
	gormDB := newSyncCollectDB(t)
	db := &database[*syncCollectMergedItem]{
		ins: gormDB,
		m:   &syncCollectMergedItem{},
		ctx: context.Background(),
	}

	first, err := db.saveResultSyncUniqueIndexes()
	require.NoError(t, err)
	require.Len(t, first, 2)

	second, err := db.saveResultSyncUniqueIndexes()
	require.NoError(t, err)
	require.Len(t, second, 2)
	require.Same(t, first[0], second[0], "later calls must answer from the per-type cache")
}
