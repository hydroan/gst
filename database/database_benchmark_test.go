package database_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

func BenchmarkDatabaseCreate(b *testing.B) {
	defer cleanupTestData()

	b.Run("size_1", func(b *testing.B) {
		for b.Loop() {
			id := strconv.Itoa(int(time.Now().UnixNano()))
			_ = database.Database[*TestUser](context.Background()).Create(&TestUser{Name: id, ID: id})
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
	b.Run("unique_index_insert", func(b *testing.B) {
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
	// Upsert is the only write that pays the unique-index sync SELECT; these
	// two sub-benchmarks measure it on the insert path and on the conflict
	// path.
	b.Run("unique_index_upsert_insert_with_sync", func(b *testing.B) {
		cleanupTestData()

		i := 0
		for b.Loop() {
			i++
			code := strconv.Itoa(i)
			_ = database.Database[*TestUniqueItem](context.Background()).Upsert(&TestUniqueItem{
				UniqueCode: code,
				Name:       code,
			})
		}
	})
	b.Run("unique_index_upsert_conflict_with_sync", func(b *testing.B) {
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
			_ = database.Database[*TestUniqueItem](context.Background()).Upsert(&TestUniqueItem{
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
			user := &TestUser{Name: id, ID: id}
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
			user := &TestUser{Name: id, ID: id}
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
		_ = database.Database[*TestUser](context.Background()).UpdateByID(u1.ID, types.NewAssignment("", "name", "user_modified"))
	}
}

func BenchmarkDatabaseList(b *testing.B) {
	defer cleanupTestData()
	require.NoError(b, database.Database[*TestUser](context.Background()).Create(ul...))

	users := make([]*TestUser, 0)
	for b.Loop() {
		_ = database.Database[*TestUser](context.Background()).List(&users)
	}
}

func BenchmarkDatabaseGet(b *testing.B) {
	defer cleanupTestData()
	require.NoError(b, database.Database[*TestUser](context.Background()).Create(ul...))

	u := new(TestUser)
	for b.Loop() {
		_ = database.Database[*TestUser](context.Background()).Get(u, u1.ID)
	}
}

func BenchmarkDatabaseCount(b *testing.B) {
	defer cleanupTestData()
	require.NoError(b, database.Database[*TestUser](context.Background()).Create(ul...))

	count := new(int)
	for b.Loop() {
		_ = database.Database[*TestUser](context.Background()).Count(count)
	}
}

func BenchmarkDatabaseFirst(b *testing.B) {
	defer cleanupTestData()
	require.NoError(b, database.Database[*TestUser](context.Background()).Create(ul...))

	u := new(TestUser)
	for b.Loop() {
		_ = database.Database[*TestUser](context.Background()).First(u)
	}
}

func BenchmarkDatabaseLast(b *testing.B) {
	defer cleanupTestData()
	require.NoError(b, database.Database[*TestUser](context.Background()).Create(ul...))

	u := new(TestUser)
	for b.Loop() {
		_ = database.Database[*TestUser](context.Background()).Last(u)
	}
}

func BenchmarkDatabaseTake(b *testing.B) {
	defer cleanupTestData()
	require.NoError(b, database.Database[*TestUser](context.Background()).Create(ul...))

	u := new(TestUser)
	for b.Loop() {
		_ = database.Database[*TestUser](context.Background()).Take(u)
	}
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
				ID:   id,
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
				ID:   id,
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
				ID:   id,
			}
		}

		_ = database.Database[*TestUser](context.Background()).Create(users...)

		for i := range users {
			users[i].Name += "_updated"
		}

		_ = database.Database[*TestUser](context.Background()).Update(users...)
	}
}

// BenchmarkDatabaseListDryRunWithQuery measures building one list request's
// queries in dry run, the request's whole query chain short of the database
// round trip: the model conditions, which are the equality filters, a filter
// of every other URL operator and the COUNT built from the same conditions,
// as the standard List action builds them.
func BenchmarkDatabaseListDryRunWithQuery(b *testing.B) {
	remark := "world"
	query := &TestUser{Name: "user1", Age: 18, Remark: &remark}
	opts := types.QueryOptions{
		PresentFields: map[string]struct{}{"age": {}},
		Filters: []types.Filter{
			types.NewFilter("", "name", types.FilterOpNe, "abc"),
			types.NewFilter("", "age", types.FilterOpGt, "0"),
			types.NewFilter("", "age", types.FilterOpGte, "1"),
			types.NewFilter("", "age", types.FilterOpLt, "100"),
			types.NewFilter("", "age", types.FilterOpLte, "99"),
			types.NewFilter("", "age", types.FilterOpIn, []string{"1", "2", "3"}),
			types.NewFilter("", "age", types.FilterOpNotIn, []string{"8", "9"}),
			types.NewFilter("", "remark", types.FilterOpLike, "wor"),
			types.NewFilter("", "remark", types.FilterOpNotLike, "xyz"),
			types.NewFilter("", "remark", types.FilterOpStartsWith, "wo"),
			types.NewFilter("", "remark", types.FilterOpEndsWith, "ld"),
			types.NewFilter("", "is_active", types.FilterOpIsNull, false),
			types.NewFilter("", "created_at", types.FilterOpGte, "2020-01-01 00:00:00"),
		},
	}
	users := make([]*TestUser, 0)
	count := new(int)

	b.ReportAllocs()
	for b.Loop() {
		if err := database.Database[*TestUser](context.Background()).WithDryRun().WithPagination(1, 10).WithQuery(query, opts).List(&users); err != nil {
			b.Fatal(err)
		}
		if err := database.Database[*TestUser](context.Background()).WithDryRun().WithQuery(query, opts).Count(count); err != nil {
			b.Fatal(err)
		}
	}
}
