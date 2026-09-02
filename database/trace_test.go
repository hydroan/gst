package database_test

import (
	"context"
	"strings"
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	"github.com/stretchr/testify/require"
)

// TestTraceModelHookSpansOnlyOverriddenHooks verifies that a model hook gets a
// span of its own only when the model overrides it: the framework base's no-op
// still runs, but there is nothing to time, so it must not export a span.
func TestTraceModelHookSpansOnlyOverriddenHooks(t *testing.T) {
	oteltest.Enable(t)
	recorder := oteltest.Record(t)
	defer cleanupTestData()

	t.Run("an overridden hook gets a span and its no-op partner does not", func(t *testing.T) {
		defer cleanupTestData()
		// TestUser overrides CreateBefore and leaves CreateAfter to the framework base.
		user := &TestUser{Name: "trace-create", Email: "trace-create@example.com"}
		require.NoError(t, database.Database[*TestUser](context.Background()).Create(user))

		names := oteltest.EndedNames(recorder)
		require.Contains(t, names, "database.TestUser.Create")
		require.Contains(t, names, "model.TestUser.CreateBefore")
		require.NotContains(t, names, "model.TestUser.CreateAfter")
	})

	t.Run("a model without hook overrides gets no hook span", func(t *testing.T) {
		defer cleanupTestData()
		item := &TestItem{Name: "trace-get"}
		require.NoError(t, database.Database[*TestItem](context.Background()).Create(item))
		stored := new(TestItem)
		require.NoError(t, database.Database[*TestItem](context.Background()).Get(stored, item.ID))

		names := oteltest.EndedNames(recorder)
		require.Contains(t, names, "database.TestItem.Get")
		for _, name := range names {
			require.False(t, strings.HasPrefix(name, "model.TestItem."), "no-op hook must not export span %q", name)
		}
	})
}
