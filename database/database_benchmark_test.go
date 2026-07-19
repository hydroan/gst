package database_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/model"
	"github.com/stretchr/testify/require"
)

func BenchmarkDatabaseCreate(b *testing.B) {
	defer cleanupTestData()

	b.Run("size_1", func(b *testing.B) {
		for b.Loop() {
			id := strconv.Itoa(int(time.Now().UnixNano()))
			_ = database.Database[*TestUser](context.Background()).Create(&TestUser{Name: id, Base: model.Base{ID: id}})
		}
	})
	b.Run("size_10", func(b *testing.B) {
		benchmarkDatabaseCreateBatch(b, 10)
	})
	b.Run("size_100", func(b *testing.B) {
		benchmarkDatabaseCreateBatch(b, 100)
	})
	b.Run("size_1000", func(b *testing.B) {
		benchmarkDatabaseCreateBatch(b, 1000)
	})
}

func BenchmarkDatabaseCreateSyncOverhead(b *testing.B) {
	defer cleanupTestData()

	b.Run("baseline_without_unique_index", func(b *testing.B) {
		cleanupTestData()

		i := 0
		for b.Loop() {
			i++
			code := strconv.Itoa(i)
			_ = database.Database[*TestPlainItem](context.Background()).Create(&TestPlainItem{
				Code: code,
				Name: code,
			})
		}
	})
	b.Run("unique_index_insert_with_sync", func(b *testing.B) {
		cleanupTestData()

		i := 0
		for b.Loop() {
			i++
			code := strconv.Itoa(i)
			_ = database.Database[*TestUniqueItem](context.Background()).Create(&TestUniqueItem{
				UniqueCode: code,
				Name:       code,
			})
		}
	})
	b.Run("unique_index_conflict_with_sync", func(b *testing.B) {
		cleanupTestData()

		const code = "same-code"
		require.NoError(b, database.Database[*TestUniqueItem](context.Background()).Create(&TestUniqueItem{
			UniqueCode: code,
			Name:       "seed",
		}))

		i := 0
		for b.Loop() {
			i++
			name := strconv.Itoa(i)
			_ = database.Database[*TestUniqueItem](context.Background()).Create(&TestUniqueItem{
				UniqueCode: code,
				Name:       name,
			})
		}
	})
}

func BenchmarkDatabaseDelete(b *testing.B) {
	defer cleanupTestData()

	b.Run("size_1", func(b *testing.B) {
		for b.Loop() {
			id := strconv.Itoa(int(time.Now().UnixNano()))
			user := &TestUser{Name: id, Base: model.Base{ID: id}}
			_ = database.Database[*TestUser](context.Background()).Create(user)
			_ = database.Database[*TestUser](context.Background()).Delete(user)
		}
	})
	b.Run("size_10", func(b *testing.B) {
		benchmarkDatabaseDeleteBatch(b, 10)
	})
	b.Run("size_100", func(b *testing.B) {
		benchmarkDatabaseDeleteBatch(b, 100)
	})
	b.Run("size_1000", func(b *testing.B) {
		benchmarkDatabaseDeleteBatch(b, 1000)
	})
}

func BenchmarkDatabaseUpdate(b *testing.B) {
	defer cleanupTestData()

	b.Run("size_1", func(b *testing.B) {
		for b.Loop() {
			id := strconv.Itoa(int(time.Now().UnixNano()))
			user := &TestUser{Name: id, Base: model.Base{ID: id}}
			_ = database.Database[*TestUser](context.Background()).Create(user)

			user.Name = id + "_updated"
			_ = database.Database[*TestUser](context.Background()).Update(user)
		}
	})
	b.Run("size_10", func(b *testing.B) {
		benchmarkDatabaseUpdateBatch(b, 10)
	})
	b.Run("size_100", func(b *testing.B) {
		benchmarkDatabaseUpdateBatch(b, 100)
	})
	b.Run("size_1000", func(b *testing.B) {
		benchmarkDatabaseUpdateBatch(b, 1000)
	})
}

func BenchmarkDatabaseUpdateByID(b *testing.B) {
	defer cleanupTestData()

	require.NoError(b, database.Database[*TestUser](context.Background()).Create(ul...))
	for b.Loop() {
		_ = database.Database[*TestUser](context.Background()).UpdateByID(u1.ID, "name", "user_modified")
	}
}

func BenchmarkDatabaseList(b *testing.B) {
	defer cleanupTestData()
	require.NoError(b, database.Database[*TestUser](context.Background()).Create(ul...))

	b.Run("nocache", func(b *testing.B) {
		users := make([]*TestUser, 0)
		for b.Loop() {
			_ = database.Database[*TestUser](context.Background()).List(&users)
		}
	})
	b.Run("withcache", func(b *testing.B) {
		users := make([]*TestUser, 0)
		for b.Loop() {
			_ = database.Database[*TestUser](context.Background()).WithCache().List(&users)
		}
	})
}

func BenchmarkDatabaseGet(b *testing.B) {
	defer cleanupTestData()
	require.NoError(b, database.Database[*TestUser](context.Background()).Create(ul...))

	b.Run("nocache", func(b *testing.B) {
		u := new(TestUser)
		for b.Loop() {
			_ = database.Database[*TestUser](context.Background()).Get(u, u1.ID)
		}
	})
	b.Run("withcache", func(b *testing.B) {
		u := new(TestUser)
		for b.Loop() {
			_ = database.Database[*TestUser](context.Background()).WithCache().Get(u, u1.ID)
		}
	})
}

func BenchmarkDatabaseCount(b *testing.B) {
	defer cleanupTestData()
	require.NoError(b, database.Database[*TestUser](context.Background()).Create(ul...))

	b.Run("nocache", func(b *testing.B) {
		count := new(int)
		for b.Loop() {
			_ = database.Database[*TestUser](context.Background()).Count(count)
		}
	})
	b.Run("withcache", func(b *testing.B) {
		count := new(int)
		for b.Loop() {
			_ = database.Database[*TestUser](context.Background()).WithCache().Count(count)
		}
	})
}

func BenchmarkDatabaseFirst(b *testing.B) {
	defer cleanupTestData()
	require.NoError(b, database.Database[*TestUser](context.Background()).Create(ul...))

	b.Run("nocache", func(b *testing.B) {
		u := new(TestUser)
		for b.Loop() {
			_ = database.Database[*TestUser](context.Background()).First(u)
		}
	})
	b.Run("withcache", func(b *testing.B) {
		u := new(TestUser)
		for b.Loop() {
			_ = database.Database[*TestUser](context.Background()).WithCache().First(u)
		}
	})
}

func BenchmarkDatabaseLast(b *testing.B) {
	defer cleanupTestData()
	require.NoError(b, database.Database[*TestUser](context.Background()).Create(ul...))

	b.Run("nocache", func(b *testing.B) {
		u := new(TestUser)
		for b.Loop() {
			_ = database.Database[*TestUser](context.Background()).Last(u)
		}
	})
	b.Run("withcache", func(b *testing.B) {
		u := new(TestUser)
		for b.Loop() {
			_ = database.Database[*TestUser](context.Background()).WithCache().Last(u)
		}
	})
}

func BenchmarkDatabaseTake(b *testing.B) {
	defer cleanupTestData()
	require.NoError(b, database.Database[*TestUser](context.Background()).Create(ul...))

	b.Run("nocache", func(b *testing.B) {
		u := new(TestUser)
		for b.Loop() {
			_ = database.Database[*TestUser](context.Background()).Take(u)
		}
	})
	b.Run("withcache", func(b *testing.B) {
		u := new(TestUser)
		for b.Loop() {
			_ = database.Database[*TestUser](context.Background()).WithCache().Take(u)
		}
	})
}

func benchmarkDatabaseCreateBatch(b *testing.B, size int) {
	b.Helper()

	users := make([]*TestUser, size)

	for b.Loop() {
		baseID := time.Now().UnixNano()

		for i := range users {
			id := strconv.FormatInt(baseID+int64(i), 10)
			users[i] = &TestUser{
				Name: id,
				Base: model.Base{ID: id},
			}
		}

		_ = database.Database[*TestUser](context.Background()).Create(users...)
	}
}

func benchmarkDatabaseDeleteBatch(b *testing.B, size int) {
	b.Helper()

	users := make([]*TestUser, size)

	for b.Loop() {
		baseID := time.Now().UnixNano()

		for i := range users {
			id := strconv.FormatInt(baseID+int64(i), 10)
			users[i] = &TestUser{
				Name: id,
				Base: model.Base{ID: id},
			}
		}

		_ = database.Database[*TestUser](context.Background()).Create(users...)
		_ = database.Database[*TestUser](context.Background()).Delete(users...)
	}
}

func benchmarkDatabaseUpdateBatch(b *testing.B, size int) {
	b.Helper()

	users := make([]*TestUser, size)

	for b.Loop() {
		baseID := time.Now().UnixNano()

		for i := range users {
			id := strconv.FormatInt(baseID+int64(i), 10)
			users[i] = &TestUser{
				Name: id,
				Base: model.Base{ID: id},
			}
		}

		_ = database.Database[*TestUser](context.Background()).Create(users...)

		for i := range users {
			users[i].Name += "_updated"
		}

		_ = database.Database[*TestUser](context.Background()).Update(users...)
	}
}
