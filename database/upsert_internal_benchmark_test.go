package database

import (
	"context"
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
)

type syncBenchPlainItem struct {
	Code string `gorm:"size:191"`
	Name string `gorm:"size:191"`

	modelregistry.Base
}

type syncBenchUniqueItem struct {
	Code string `gorm:"size:191;uniqueIndex"`
	Name string `gorm:"size:191"`

	modelregistry.Base
}

type syncBenchIndexerItem struct {
	Code string `gorm:"size:191"`
	Name string `gorm:"size:191"`

	modelregistry.Base
}

func (*syncBenchIndexerItem) TableName() string { return "sync_bench_indexer_items" }

// Indexes declares the unique key the sync must resolve from index plans.
func (*syncBenchIndexerItem) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Code"}, Unique: true}}
}

func BenchmarkSyncSaveResultsByUniqueIndexes(b *testing.B) {
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DryRun: true,
		Logger: glogger.Default.LogMode(glogger.Silent),
	})
	if err != nil {
		b.Fatal(err)
	}

	b.Run("no_unique_index", func(b *testing.B) {
		db := &database[*syncBenchPlainItem]{
			ins: gormDB,
			m:   &syncBenchPlainItem{},
			ctx: context.Background(),
		}
		objs := []*syncBenchPlainItem{
			{
				Code: "code",
				Name: "name",
				ID:   "id",
			},
		}

		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if err := db.syncSaveResultsByUniqueIndexes("", objs); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("unique_index", func(b *testing.B) {
		db := &database[*syncBenchUniqueItem]{
			ins: gormDB,
			m:   &syncBenchUniqueItem{},
			ctx: context.Background(),
		}
		objs := []*syncBenchUniqueItem{
			{
				Code: "code",
				Name: "name",
				ID:   "id",
			},
		}

		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if err := db.syncSaveResultsByUniqueIndexes("", objs); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("indexer_unique", func(b *testing.B) {
		db := &database[*syncBenchIndexerItem]{
			ins: gormDB,
			m:   &syncBenchIndexerItem{},
			ctx: context.Background(),
		}
		objs := []*syncBenchIndexerItem{
			{
				Code: "code",
				Name: "name",
				ID:   "id",
			},
		}

		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if err := db.syncSaveResultsByUniqueIndexes("", objs); err != nil {
				b.Fatal(err)
			}
		}
	})
}
