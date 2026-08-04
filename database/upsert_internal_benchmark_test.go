package database

import (
	"context"
	"testing"

	"github.com/hydroan/gst/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
)

type syncBenchPlainItem struct {
	Code string `gorm:"size:191"`
	Name string `gorm:"size:191"`

	model.Base
}

type syncBenchUniqueItem struct {
	Code string `gorm:"size:191;uniqueIndex"`
	Name string `gorm:"size:191"`

	model.Base
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
				Base: model.Base{ID: "id"},
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
				Base: model.Base{ID: "id"},
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
