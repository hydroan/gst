package database_test

import (
	"context"
	"strings"
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDatabaseWithBatchSize(t *testing.T) {
	defer cleanupTestData()

	t.Run("Create", func(t *testing.T) {
		t.Run("batch_size_1", func(t *testing.T) {
			defer cleanupTestData()
			require.NoError(t, database.Database[*TestUser](context.Background()).WithBatchSize(1).Create(ul...))
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
			require.Len(t, users, 3)
			// Verify data integrity
			var foundU1, foundU2, foundU3 bool
			for _, u := range users {
				switch u.ID {
				case u1.ID:
					foundU1 = true
					require.Equal(t, u1.Name, u.Name)
					require.Equal(t, u1.Age, u.Age)
					require.Equal(t, u1.Email, u.Email)
				case u2.ID:
					foundU2 = true
					require.Equal(t, u2.Name, u.Name)
				case u3.ID:
					foundU3 = true
					require.Equal(t, u3.Name, u.Name)
				}
			}
			require.True(t, foundU1 && foundU2 && foundU3, "all users should be found after batch create")
		})

		t.Run("batch_size_2", func(t *testing.T) {
			defer cleanupTestData()
			require.NoError(t, database.Database[*TestUser](context.Background()).WithBatchSize(2).Create(ul...))
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
			require.Len(t, users, 3)
		})

		t.Run("batch_size_1000", func(t *testing.T) {
			defer cleanupTestData()
			require.NoError(t, database.Database[*TestUser](context.Background()).WithBatchSize(1000).Create(ul...))
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
			require.Len(t, users, 3)
		})

		t.Run("batch_size_0", func(t *testing.T) {
			defer cleanupTestData()
			require.NoError(t, database.Database[*TestUser](context.Background()).WithBatchSize(0).Create(ul...))
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
			require.Len(t, users, 3, "should use default batch size when set to 0")
		})

		t.Run("batch_size_negative", func(t *testing.T) {
			defer cleanupTestData()
			require.NoError(t, database.Database[*TestUser](context.Background()).WithBatchSize(-1).Create(ul...))
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
			require.Len(t, users, 3, "should use default batch size when set to negative")
		})
	})

	t.Run("Update", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		t.Run("batch_size_1", func(t *testing.T) {
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
			require.Len(t, users, 3)
			users[0].Age = 25
			users[1].Age = 26
			users[2].Age = 27
			require.NoError(t, database.Database[*TestUser](context.Background()).WithBatchSize(1).Update(users...))
			users = make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
			require.Len(t, users, 3)
			require.Equal(t, 25, users[0].Age)
			require.Equal(t, 26, users[1].Age)
			require.Equal(t, 27, users[2].Age)
		})

		t.Run("batch_size_2", func(t *testing.T) {
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
			require.Len(t, users, 3)
			users[0].Age = 30
			users[1].Age = 31
			users[2].Age = 32
			require.NoError(t, database.Database[*TestUser](context.Background()).WithBatchSize(2).Update(users...))
			users = make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
			require.Len(t, users, 3)
			require.Equal(t, 30, users[0].Age)
			require.Equal(t, 31, users[1].Age)
			require.Equal(t, 32, users[2].Age)
		})
	})

	t.Run("Delete", func(t *testing.T) {
		t.Run("batch_size_1", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
			require.Len(t, users, 3)
			require.NoError(t, database.Database[*TestUser](context.Background()).WithBatchSize(1).Delete(users[0]))
			users = make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
			require.Len(t, users, 2)
		})

		t.Run("batch_size_2", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
			require.Len(t, users, 3)
			require.NoError(t, database.Database[*TestUser](context.Background()).WithBatchSize(2).Delete(users...))
			users = make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
			require.Empty(t, users)
		})

		t.Run("batch_size_large", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)
			require.NoError(t, database.Database[*TestUser](context.Background()).WithBatchSize(10000).Delete(ul...))
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
			require.Empty(t, users)
		})
	})

	t.Run("Combined", func(t *testing.T) {
		defer cleanupTestData()
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithBatchSize(1000).
			WithQuery(&TestUser{Name: u1.Name}).
			Create(u1))
		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
		require.GreaterOrEqual(t, len(users), 1, "should find created user")
	})
}

func TestDatabaseWithDryRun(t *testing.T) {
	defer cleanupTestData()

	t.Run("Create", func(t *testing.T) {
		defer cleanupTestData()

		// WithDryRun should only build the INSERT statement without executing hooks or database I/O.
		require.NoError(t, database.Database[*TestUser](context.Background()).WithDryRun().Create(ul...))
		require.Nil(t, u1.Remark, "Create should not run model hooks in dry-run mode")
		require.Nil(t, u2.Remark, "Create should not run model hooks in dry-run mode")
		require.Nil(t, u3.Remark, "Create should not run model hooks in dry-run mode")
		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
		require.Empty(t, users, "records should not be created in dry-run mode")

		dryRunUser := &TestUser{Name: "dry-run-create", Email: "dry-run-create@example.com"}
		require.NoError(t, database.Database[*TestUser](context.Background()).WithDryRun().Create(dryRunUser))
		require.Empty(t, dryRunUser.ID, "Create should not set ID in dry-run mode")
		require.True(t, dryRunUser.CreatedAt.IsZero(), "Create should not set created_at in dry-run mode")
		require.True(t, dryRunUser.UpdatedAt.IsZero(), "Create should not set updated_at in dry-run mode")
		require.Nil(t, dryRunUser.Remark, "Create should not run model hooks in dry-run mode")
	})

	t.Run("Delete", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// WithDryRun should not actually delete records
		count := new(int)
		require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
		require.Equal(t, 3, *count, "should have 3 records initially")

		require.NoError(t, database.Database[*TestUser](context.Background()).WithDryRun().Delete(u1))
		require.NoError(t, database.Database[*TestUser](context.Background()).Count(count))
		require.Equal(t, 3, *count, "records should not be deleted in dry-run mode")

		softDeleteUser := &dryRunSoftDeleteUser{Name: "dry-run-soft-delete", Base: model.Base{ID: "dry-run-soft-delete"}}
		require.NoError(t, database.Database[*dryRunSoftDeleteUser](context.Background()).WithDryRun().Delete(softDeleteUser))
		require.False(t, softDeleteUser.DeletedAt.Valid, "Delete should not set deleted_at in dry-run mode")
	})

	t.Run("Update", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// WithDryRun should only build the UPDATE statement without executing hooks or database I/O.
		originalName := u1.Name
		u1.Name = "updated_name"
		u1.Remark = nil
		require.NoError(t, database.Database[*TestUser](context.Background()).WithDryRun().Update(u1))
		require.Nil(t, u1.Remark, "Update should not run model hooks in dry-run mode")

		// Verify record is not updated
		uu := new(TestUser)
		require.NoError(t, database.Database[*TestUser](context.Background()).Get(uu, u1.ID))
		require.Equal(t, originalName, uu.Name, "name should not be updated in dry-run mode")

		// Even in dry-run mode Update requires an ID: without a primary key
		// there is no WHERE clause to build the UPDATE statement from.
		dryRunUser := &TestUser{Name: "dry-run-update", Email: "dry-run-update@example.com"}
		require.ErrorIs(t, database.Database[*TestUser](context.Background()).WithDryRun().Update(dryRunUser), database.ErrIDRequired)
		require.Empty(t, dryRunUser.ID, "Update should not set ID in dry-run mode")
		require.True(t, dryRunUser.CreatedAt.IsZero(), "Update should not set created_at in dry-run mode")
		require.True(t, dryRunUser.UpdatedAt.IsZero(), "Update should not set updated_at in dry-run mode")
		require.Nil(t, dryRunUser.Remark, "Update should not run model hooks in dry-run mode")
	})

	t.Run("UpdateByID", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// WithDryRun should not actually update records
		originalName := u1.Name
		require.NoError(t, database.Database[*TestUser](context.Background()).WithDryRun().UpdateByID(u1.ID, types.Assign("name", "updated_name")))

		// Verify record is not updated
		uu := new(TestUser)
		require.NoError(t, database.Database[*TestUser](context.Background()).Get(uu, u1.ID))
		require.Equal(t, originalName, uu.Name, "name should not be updated in dry-run mode")
	})

	t.Run("List", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// WithDryRun should only build the SELECT statement without executing it.
		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).WithDryRun().List(&users))
		require.Empty(t, users, "List should not return results in dry-run mode")
	})

	t.Run("Get", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// WithDryRun should only build the SELECT statement without executing it.
		uu := new(TestUser)
		require.NoError(t, database.Database[*TestUser](context.Background()).WithDryRun().Get(uu, u1.ID))
		require.NotNil(t, uu)
		require.Empty(t, uu.ID, "Get should not return results in dry-run mode")
	})

	t.Run("GetIgnoresDestinationIDWhenBuildingSQL", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		const callbackName = "gst:test:dry_run_get_sql"
		_ = database.DB().Callback().Query().Remove(callbackName)
		var gotVars []any
		require.NoError(t, database.DB().Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
			gotVars = append([]any(nil), tx.Statement.Vars...)
		}))
		t.Cleanup(func() {
			require.NoError(t, database.DB().Callback().Query().Remove(callbackName))
		})

		existingID := u1.ID
		uu := &TestUser{Base: model.Base{ID: existingID}}
		require.NoError(t, database.Database[*TestUser](context.Background()).WithDryRun().Get(uu, u2.ID))
		require.Equal(t, existingID, uu.ID, "Get should leave destination values unchanged in dry-run mode")
		require.Equal(t, []any{u2.ID}, gotVars, "Get dry-run SQL should only use the requested id")
	})

	t.Run("Count", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// WithDryRun should only build the SELECT statement without executing it.
		count := new(int)
		require.NoError(t, database.Database[*TestUser](context.Background()).WithDryRun().Count(count))
		require.Equal(t, 0, *count, "Count should not execute in dry-run mode")
	})

	t.Run("First", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// WithDryRun should only build the SELECT statement without executing it.
		uu := new(TestUser)
		require.NoError(t, database.Database[*TestUser](context.Background()).WithDryRun().First(uu))
		require.NotNil(t, uu)
		require.Empty(t, uu.ID, "First should not return results in dry-run mode")
	})

	t.Run("Last", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// WithDryRun should only build the SELECT statement without executing it.
		uu := new(TestUser)
		require.NoError(t, database.Database[*TestUser](context.Background()).WithDryRun().Last(uu))
		require.NotNil(t, uu)
		require.Empty(t, uu.ID, "Last should not return results in dry-run mode")
	})

	t.Run("Take", func(t *testing.T) {
		defer cleanupTestData()
		setupTestData(t)

		// WithDryRun should only build the SELECT statement without executing it.
		uu := new(TestUser)
		require.NoError(t, database.Database[*TestUser](context.Background()).WithDryRun().Take(uu))
		require.NotNil(t, uu)
		require.Empty(t, uu.ID, "Take should not return results in dry-run mode")
	})
}

func TestDatabaseWithBuildSQL(t *testing.T) {
	t.Run("NilCollector", func(t *testing.T) {
		users := make([]*TestUser, 0)
		err := database.Database[*TestUser](context.Background()).WithBuildSQL(nil).List(&users)

		require.ErrorIs(t, err, database.ErrNilSQLBuilder)
	})

	t.Run("List", func(t *testing.T) {
		var stmts []types.SQLStatement
		users := make([]*TestUser, 0)

		err := database.Database[*TestUser](context.Background()).
			WithBuildSQL(&stmts).
			WithQuery(&TestUser{Name: u1.Name}).
			WithOrder(types.Desc("created_at")).
			List(&users)

		require.NoError(t, err)
		require.Len(t, stmts, 1)
		requireSQLContains(t, stmts[0], "SELECT", "FROM", "test_users", "WHERE", "ORDER BY")
		require.Contains(t, stmts[0].Args, u1.Name)
		require.Contains(t, stmts[0].RenderedSQL, u1.Name)
		require.Empty(t, users, "WithBuildSQL should not execute the query or fill the destination")
	})

	t.Run("ListWithModelQuery", func(t *testing.T) {
		t.Run("Query", func(t *testing.T) {
			var stmts []types.SQLStatement
			users := make([]*queryableTestUser, 0)
			cursorValue := "cursor-001"
			query := &queryableTestUser{
				Name: "queryable-user",
				Query: model.Query{
					Pagination: model.Pagination{
						Page: 2,
						Size: 10,
					},
					Cursor: model.Cursor{
						CursorValue: &cursorValue,
						CursorField: "id",
						CursorNext:  true,
					},
					SortBy: "created_at desc",
				},
			}

			err := database.Database[*queryableTestUser](context.Background()).
				WithBuildSQL(&stmts).
				WithQuery(query).
				List(&users)

			require.NoError(t, err)
			require.Len(t, stmts, 1)
			require.Contains(t, stmts[0].Args, query.Name)
			require.NotContains(t, stmts[0].Args, "2")
			require.NotContains(t, stmts[0].Args, "10")
			require.NotContains(t, stmts[0].Args, "1")
			require.NotContains(t, stmts[0].Args, *query.CursorValue)
			require.NotContains(t, stmts[0].Args, query.CursorField)
			require.NotContains(t, stmts[0].Args, query.SortBy)
			require.Empty(t, users, "WithBuildSQL should not execute the query or fill the destination")
		})

		t.Run("Pagination", func(t *testing.T) {
			var stmts []types.SQLStatement
			users := make([]*paginatableTestUser, 0)
			query := &paginatableTestUser{
				Name:       "paginatable-user",
				Pagination: model.Pagination{Page: 2, Size: 10},
			}

			err := database.Database[*paginatableTestUser](context.Background()).
				WithBuildSQL(&stmts).
				WithQuery(query).
				List(&users)

			require.NoError(t, err)
			require.Len(t, stmts, 1)
			require.Contains(t, stmts[0].Args, query.Name)
			require.NotContains(t, stmts[0].Args, "2")
			require.NotContains(t, stmts[0].Args, "10")
			require.Empty(t, users, "WithBuildSQL should not execute the query or fill the destination")
		})

		t.Run("Cursor", func(t *testing.T) {
			var stmts []types.SQLStatement
			users := make([]*cursorableTestUser, 0)
			cursorValue := "cursor-001"
			query := &cursorableTestUser{
				Name: "cursorable-user",
				Cursor: model.Cursor{
					CursorValue: &cursorValue,
					CursorField: "id",
					CursorNext:  true,
				},
			}

			err := database.Database[*cursorableTestUser](context.Background()).
				WithBuildSQL(&stmts).
				WithQuery(query).
				List(&users)

			require.NoError(t, err)
			require.Len(t, stmts, 1)
			require.Contains(t, stmts[0].Args, query.Name)
			require.NotContains(t, stmts[0].Args, *query.CursorValue)
			require.NotContains(t, stmts[0].Args, query.CursorField)
			require.NotContains(t, stmts[0].Args, "1")
			require.Empty(t, users, "WithBuildSQL should not execute the query or fill the destination")
		})
	})

	t.Run("CreateDoesNotExecute", func(t *testing.T) {
		defer cleanupTestData()

		var stmts []types.SQLStatement
		user := &TestUser{Name: "build-sql-create", Email: "build-sql-create@example.com"}
		err := database.Database[*TestUser](context.Background()).WithBuildSQL(&stmts).Create(user)

		require.NoError(t, err)
		require.Len(t, stmts, 1)
		requireSQLContains(t, stmts[0], "INSERT", "INTO", "test_users")
		require.Contains(t, stmts[0].Args, user.Name)
		require.Contains(t, stmts[0].RenderedSQL, user.Name)
		require.Empty(t, user.ID, "WithBuildSQL should not fill model IDs")
		require.True(t, user.CreatedAt.IsZero(), "WithBuildSQL should not fill created_at")
		require.True(t, user.UpdatedAt.IsZero(), "WithBuildSQL should not fill updated_at")
		require.Nil(t, user.Remark, "WithBuildSQL should not run model hooks")

		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(&TestUser{Name: user.Name}).
			List(&users))
		require.Empty(t, users, "WithBuildSQL should not create database rows")
	})

	t.Run("BatchCreate", func(t *testing.T) {
		var stmts []types.SQLStatement
		users := []*TestUser{
			{Name: "build-sql-batch-1", Email: "build-sql-batch-1@example.com"},
			{Name: "build-sql-batch-2", Email: "build-sql-batch-2@example.com"},
			{Name: "build-sql-batch-3", Email: "build-sql-batch-3@example.com"},
		}

		err := database.Database[*TestUser](context.Background()).
			WithBuildSQL(&stmts).
			WithBatchSize(2).
			Create(users...)

		require.NoError(t, err)
		require.Len(t, stmts, 2)
		requireSQLContains(t, stmts[0], "INSERT", "INTO", "test_users")
		requireSQLContains(t, stmts[1], "INSERT", "INTO", "test_users")
		require.Contains(t, stmts[0].Args, users[0].Name)
		require.Contains(t, stmts[0].Args, users[1].Name)
		require.Contains(t, stmts[1].Args, users[2].Name)
		require.Contains(t, stmts[0].RenderedSQL, users[0].Name)
		require.Contains(t, stmts[1].RenderedSQL, users[2].Name)
	})

	t.Run("GetIgnoresDestinationID", func(t *testing.T) {
		existingID := u1.ID
		requestedID := u2.ID
		dest := &TestUser{Base: model.Base{ID: existingID}}
		var stmts []types.SQLStatement

		err := database.Database[*TestUser](context.Background()).WithBuildSQL(&stmts).Get(dest, requestedID)

		require.NoError(t, err)
		require.Len(t, stmts, 1)
		requireSQLContains(t, stmts[0], "SELECT", "FROM", "test_users", "WHERE")
		require.Equal(t, []any{requestedID}, stmts[0].Args, "Get SQL should only use the requested id")
		require.Contains(t, stmts[0].RenderedSQL, requestedID)
		require.Equal(t, existingID, dest.ID, "WithBuildSQL should leave destination values unchanged")
	})

	t.Run("ResetsAfterAction", func(t *testing.T) {
		defer cleanupTestData()

		var stmts []types.SQLStatement
		user := &TestUser{Name: "build-sql-reset", Email: "build-sql-reset@example.com"}
		require.NoError(t, database.Database[*TestUser](context.Background()).WithBuildSQL(&stmts).Create(user))
		require.Len(t, stmts, 1)

		require.NoError(t, database.Database[*TestUser](context.Background()).Create(user))

		users := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](context.Background()).
			WithQuery(&TestUser{Name: user.Name}).
			List(&users))
		require.Len(t, users, 1, "normal actions after WithBuildSQL should execute database I/O")
	})
}

func requireSQLContains(t *testing.T, stmt types.SQLStatement, parts ...string) {
	t.Helper()

	sql := strings.ToUpper(stmt.Query)
	for _, part := range parts {
		require.Contains(t, sql, strings.ToUpper(part))
	}
}

type dryRunSoftDeleteUser struct {
	Name string `json:"name"`

	model.Base
}
