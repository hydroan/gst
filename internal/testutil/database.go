package testutil

import (
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/tenant"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

// RequireGet asserts that the row with the given id exists and returns it.
func RequireGet[T any, M interface {
	types.Model
	*T
}](t *testing.T, id string) M {
	t.Helper()

	dest := M(new(T))
	require.NoError(t, database.Database[M](t.Context()).Get(dest, id))
	return dest
}

// RequireFirst asserts that at least one row matches the query and returns
// the first match. A nil or zero-value query matches no rows: WithQuery keeps
// the framework's empty-query safety check, so a deliberate full-table read
// stays on the database chain.
func RequireFirst[T any, M interface {
	types.Model
	*T
}](t *testing.T, query M) M {
	t.Helper()

	dest := M(new(T))
	require.NoError(t, database.Database[M](t.Context()).WithQuery(query).First(dest))
	return dest
}

// RequireList asserts that listing the rows matching the query succeeds and
// returns them, sorted by orders when given. An empty result is a valid
// return; length assertions stay with the caller. A nil or zero-value query
// matches no rows: WithQuery keeps the framework's empty-query safety check,
// so a deliberate full-table read stays on the database chain.
func RequireList[T any, M interface {
	types.Model
	*T
}](t *testing.T, query M, orders ...types.Order) []M {
	t.Helper()

	chain := database.Database[M](t.Context()).WithQuery(query)
	if len(orders) > 0 {
		chain = chain.WithOrder(orders...)
	}
	rows := make([]M, 0)
	require.NoError(t, chain.List(&rows))
	return rows
}

// RequireCount asserts that counting the rows matching the query succeeds
// and returns the count. A nil or zero-value query matches no rows: WithQuery
// keeps the framework's empty-query safety check, so a deliberate full-table
// count stays on the database chain.
func RequireCount[T any, M interface {
	types.Model
	*T
}](t *testing.T, query M) int {
	t.Helper()

	count := 0
	require.NoError(t, database.Database[M](t.Context()).WithQuery(query).Count(&count))
	return count
}

// RequireNoRow asserts that no row with the given id is visible through the
// regular query path. A hard-deleted and a soft-deleted row both satisfy it;
// RequireSoftDeleted additionally pins that a soft-deleted row is kept.
func RequireNoRow[T any, M interface {
	types.Model
	*T
}](t *testing.T, id string) {
	t.Helper()

	require.ErrorIs(t, database.Database[M](t.Context()).Get(M(new(T)), id), database.ErrRecordNotFound)
}

// RequireSoftDeleted asserts that the row with the given id is soft-deleted:
// gone from the regular query path while WithDeleted still finds it. Both
// reads span every tenant, so a row filed under another tenant is judged by
// its deletion rather than kept out of sight by the tenant scope, and gorm's
// own soft-delete scope decides what counts as deleted, whatever column and
// zero value the model declares for it.
func RequireSoftDeleted[T any, M interface {
	types.Model
	*T
}](t *testing.T, id string) {
	t.Helper()

	ctx := tenant.Across(t.Context())
	require.ErrorIs(t, database.Database[M](ctx).Get(M(new(T)), id), database.ErrRecordNotFound,
		"the row should be gone from the regular query path of every tenant")
	require.NoError(t, database.Database[M](ctx).WithDeleted().Get(M(new(T)), id),
		"the soft-deleted row should stay in %s", M(new(T)).TableName())
}
