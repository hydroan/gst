package database

import (
	"context"
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/tenant"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	gormschema "gorm.io/gorm/schema"
)

// The collect fixtures below are parsed through a DryRun sqlite instance;
// none of them registers a model or touches a database.

type syncCollectMergedItem struct {
	Code string `gorm:"size:191;uniqueIndex"`
	Ref  string `gorm:"size:191"`

	modelregistry.Base
}

func (*syncCollectMergedItem) TableName() string { return "sync_collect_merged_items" }

// Indexes declares the unique key on Ref, next to the tag-declared one on Code.
func (*syncCollectMergedItem) Indexes() []modelschema.Index {
	return []modelschema.Index{{Fields: []string{"Ref"}, Unique: true}}
}

type syncCollectCompositeItem struct {
	Code string `gorm:"size:191"`
	Kind string `gorm:"size:191"`

	modelregistry.Base
}

func (*syncCollectCompositeItem) TableName() string { return "sync_collect_composite_items" }

// Indexes declares a composite unique key whose column order must survive.
func (*syncCollectCompositeItem) Indexes() []modelschema.Index {
	return []modelschema.Index{{Fields: []string{"Kind", "Code"}, Unique: true}}
}

type syncCollectPlainIndexItem struct {
	Code string `gorm:"size:191"`

	modelregistry.Base
}

func (*syncCollectPlainIndexItem) TableName() string { return "sync_collect_plain_index_items" }

// Indexes declares a non-unique index, which the sync must ignore.
func (*syncCollectPlainIndexItem) Indexes() []modelschema.Index {
	return []modelschema.Index{{Fields: []string{"Code"}}}
}

type syncCollectPrimaryOnlyItem struct {
	modelregistry.Base
}

func (*syncCollectPrimaryOnlyItem) TableName() string { return "sync_collect_primary_only_items" }

// Indexes declares a unique key covering only the primary key: a primary-key
// merge keeps the caller's id, so the sync must drop the declaration.
func (*syncCollectPrimaryOnlyItem) Indexes() []modelschema.Index {
	return []modelschema.Index{{Fields: []string{"ID"}, Unique: true}}
}

// syncCollectTenantItem belongs to a tenant: unique on its code within the
// tenant by its Indexes declaration, and on its ref by a struct tag.
type syncCollectTenantItem struct {
	Code string `gorm:"size:191"`
	Ref  string `gorm:"size:191;uniqueIndex"`

	tenant.Scope
	modelregistry.Base
}

func (*syncCollectTenantItem) TableName() string { return "sync_collect_tenant_items" }

func (*syncCollectTenantItem) Indexes() []modelschema.Index {
	return []modelschema.Index{{Fields: []string{"TenantID", "Code"}, Unique: true}}
}

// newSyncCollectDB opens the DryRun sqlite instance the collect tests parse
// their fixtures through.
func newSyncCollectDB(t *testing.T) *gorm.DB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DryRun: true,
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	return gormDB
}

// syncCollectInputs resolves the parsed schema and the index plans of m the
// way uniqueIndexes does before collecting.
func syncCollectInputs(t *testing.T, gormDB *gorm.DB, m any) (*gormschema.Schema, []modelschema.IndexPlan) {
	t.Helper()
	stmt := &gorm.Statement{DB: gormDB}
	require.NoError(t, stmt.Parse(m))
	plans, err := modelschema.ParseIndexPlans(gormDB, m)
	require.NoError(t, err)
	return stmt.Schema, plans
}

func TestCollectUniqueIndexesGathersTheDeclaredUniqueKeys(t *testing.T) {
	gormDB := newSyncCollectDB(t)

	t.Run("merges tag and Indexes sources", func(t *testing.T) {
		schema, plans := syncCollectInputs(t, gormDB, &syncCollectMergedItem{})
		indexes, err := collectUniqueIndexes(schema, plans)
		require.NoError(t, err)
		require.Len(t, indexes, 2, "one index per declaration source")

		require.Equal(t, []string{"code"}, indexColumns(indexes[0]), "the tag-declared key comes first")
		require.Equal(t, "UNIQUE", indexes[0].Class)
		require.Empty(t, indexes[0].Where, "a tag-declared key holds every row")

		require.Equal(t, []string{"ref"}, indexColumns(indexes[1]), "the Indexes-declared key follows")
		require.Equal(t, "UNIQUE", indexes[1].Class)
		require.Equal(t, "uniq_sync_collect_merged_items_ref", indexes[1].Name, "plan indexes keep the framework-generated name")
		require.Equal(t, "deleted_at IS NULL", indexes[1].Where, "an Indexes-declared key holds the live rows alone, under the condition gorm keeps the deleted ones out by")
	})

	t.Run("keeps the declared composite column order", func(t *testing.T) {
		schema, plans := syncCollectInputs(t, gormDB, &syncCollectCompositeItem{})
		indexes, err := collectUniqueIndexes(schema, plans)
		require.NoError(t, err)
		require.Len(t, indexes, 1)
		require.Equal(t, []string{"kind", "code"}, indexColumns(indexes[0]))
	})

	t.Run("ignores non-unique plans", func(t *testing.T) {
		schema, plans := syncCollectInputs(t, gormDB, &syncCollectPlainIndexItem{})
		indexes, err := collectUniqueIndexes(schema, plans)
		require.NoError(t, err)
		require.Empty(t, indexes)
	})

	t.Run("drops declarations covering only primary key columns", func(t *testing.T) {
		schema, plans := syncCollectInputs(t, gormDB, &syncCollectPrimaryOnlyItem{})
		indexes, err := collectUniqueIndexes(schema, plans)
		require.NoError(t, err)
		require.Empty(t, indexes)
	})

	t.Run("fails on a plan column the schema does not carry", func(t *testing.T) {
		schema, _ := syncCollectInputs(t, gormDB, &syncCollectMergedItem{})
		_, err := collectUniqueIndexes(schema, []modelschema.IndexPlan{
			{Name: "uniq_sync_collect_merged_items_ghost", Table: "sync_collect_merged_items", Columns: []string{"ghost"}, Unique: true},
		})
		require.ErrorContains(t, err, `column "ghost"`)
	})
}

// TestRefuseForeignTenantRowsReadsEachKeyOverTheRowsItHolds pins the read a
// tenant model's Upsert runs before its write: the primary key and a
// tag-declared unique index are read over every row, a soft-deleted one
// included, and a unique index over the live rows alone under its condition,
// which is also what lets the read run on that index.
func TestRefuseForeignTenantRowsReadsEachKeyOverTheRowsItHolds(t *testing.T) {
	gormDB := newSyncCollectDB(t)
	var statements []string
	require.NoError(t, gormDB.Callback().Query().After("gorm:query").Register("capture", func(tx *gorm.DB) {
		statements = append(statements, tx.Statement.SQL.String())
	}))
	db := &database[*syncCollectTenantItem]{
		ins: gormDB,
		m:   &syncCollectTenantItem{},
		ctx: tenant.In(context.Background(), "tenant-a"),
	}

	item := &syncCollectTenantItem{Code: "c-1", Ref: "r-1", TenantID: "tenant-a"}
	item.SetID("id-1")
	require.NoError(t, db.refuseForeignTenantRows("sync_collect_tenant_items", []*syncCollectTenantItem{item}))
	require.Equal(t, []string{
		"SELECT `tenant_id` FROM `sync_collect_tenant_items` WHERE (" +
			"`sync_collect_tenant_items`.`id` = ? OR " +
			"`sync_collect_tenant_items`.`ref` = ? OR " +
			"((`sync_collect_tenant_items`.`tenant_id`,`sync_collect_tenant_items`.`code`) IN ((?,?)) AND deleted_at IS NULL))",
	}, statements)
}

func TestUniqueIndexesCached(t *testing.T) {
	gormDB := newSyncCollectDB(t)
	db := &database[*syncCollectMergedItem]{
		ins: gormDB,
		m:   &syncCollectMergedItem{},
		ctx: context.Background(),
	}

	first, err := db.uniqueIndexes()
	require.NoError(t, err)
	require.Len(t, first, 2)

	second, err := db.uniqueIndexes()
	require.NoError(t, err)
	require.Len(t, second, 2)
	require.Same(t, first[0], second[0], "later calls must answer from the per-type cache")
}
