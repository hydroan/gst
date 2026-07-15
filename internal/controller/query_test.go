package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/model"
	"github.com/stretchr/testify/require"
)

type listQueryableTestModel struct {
	Name string `query:"name"`

	model.Query
	model.Base
}

type listUnsafeQueryableTestModel struct {
	Name string `query:"name"`

	model.Query
	model.UnsafeQuery
	model.Base
}

func TestDecodeListQueryGatesUnsafeQueryKeys(t *testing.T) {
	queryKeys := map[string][]string{
		"name":         {"alice"},
		"_fuzzy":       {"true"},
		"_sort_by":     {"created_at desc"},
		"_time_column": {"created_at"},
		"_start_time":  {"2025-01-01 00:00:00"},
		"_end_time":    {"2025-01-02 00:00:00"},
	}

	t.Run("QueryAcceptsRegularKeys", func(t *testing.T) {
		var m listQueryableTestModel
		require.NoError(t, decodeListQuery(&m, queryKeys))
	})

	t.Run("QueryRejectsUnsafeKeys", func(t *testing.T) {
		for _, key := range []string{"_or", "_index", "_select", "_no_cache", "_no_total"} {
			var m listQueryableTestModel
			err := decodeListQuery(&m, map[string][]string{key: {"true"}})
			require.Error(t, err, "unsafe query key %q must be rejected without model.UnsafeQuery", key)
		}
	})

	t.Run("UnsafeQueryAcceptsUnsafeKeys", func(t *testing.T) {
		var m listUnsafeQueryableTestModel
		require.NoError(t, decodeListQuery(&m, map[string][]string{
			"_or":       {"true"},
			"_index":    {"idx_test"},
			"_select":   {"name"},
			"_no_cache": {"true"},
			"_no_total": {"true"},
		}))
	})

	t.Run("UnsafeQueryAloneRejectsRegularKeys", func(t *testing.T) {
		type unsafeOnlyModel struct {
			model.UnsafeQuery
			model.Base
		}
		var m unsafeOnlyModel
		require.Error(t, decodeListQuery(&m, map[string][]string{"_fuzzy": {"true"}}))
	})
}

func TestPresentQueryFields(t *testing.T) {
	t.Run("CollectsExplicitModelKeys", func(t *testing.T) {
		present := presentQueryFields(map[string][]string{
			"is_active": {"false"},
			"age":       {"0"},
			"isLocked":  {"true"},
			"size":      {"3"},
		})
		require.Equal(t, map[string]struct{}{
			"is_active": {},
			"age":       {},
			"is_locked": {},
			"size":      {},
		}, present, "camel case keys should normalize to snake case column names, and bare names like size are model filter columns")
	})

	t.Run("ExcludesFrameworkKeys", func(t *testing.T) {
		present := presentQueryFields(map[string][]string{
			"_page":         {"1"},
			"_size":         {"10"},
			"_limit":        {"100"},
			"_fuzzy":        {"true"},
			"_sort_by":      {"created_at desc"},
			"_no_total":     {"true"},
			"_cursor_value": {"abc"},
		})
		require.Empty(t, present, "framework parameters live in the underscore namespace and are not model filter columns")
	})

	t.Run("ExcludesKeysWithoutValues", func(t *testing.T) {
		present := presentQueryFields(map[string][]string{
			"is_active": {""},
			"remark":    {"", ""},
		})
		require.Empty(t, present, "an empty value means the caller is not filtering by that key")
	})
}

func TestParseQueryTime(t *testing.T) {
	t.Run("DateTimeLayout", func(t *testing.T) {
		got, err := parseQueryTime("2026-07-01 08:30:15", false)
		require.NoError(t, err)
		require.Equal(t, time.Date(2026, 7, 1, 8, 30, 15, 0, time.Local), got)
	})

	t.Run("DateTimeLocalLayoutWithSeconds", func(t *testing.T) {
		got, err := parseQueryTime("2026-07-01T08:30:15", false)
		require.NoError(t, err)
		require.Equal(t, time.Date(2026, 7, 1, 8, 30, 15, 0, time.Local), got)
	})

	t.Run("DateTimeLocalLayoutWithoutSeconds", func(t *testing.T) {
		got, err := parseQueryTime("2026-07-01T08:30", false)
		require.NoError(t, err)
		require.Equal(t, time.Date(2026, 7, 1, 8, 30, 0, 0, time.Local), got)
	})

	t.Run("DateOnlyStartIsBeginOfDay", func(t *testing.T) {
		got, err := parseQueryTime("2026-07-01", false)
		require.NoError(t, err)
		require.Equal(t, time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local), got)
	})

	t.Run("DateOnlyEndCoversWholeDay", func(t *testing.T) {
		got, err := parseQueryTime("2026-07-01", true)
		require.NoError(t, err)
		require.Equal(t, time.Date(2026, 7, 2, 0, 0, 0, 0, time.Local).Add(-time.Nanosecond), got)
	})

	t.Run("RFC3339KeepsExplicitOffset", func(t *testing.T) {
		got, err := parseQueryTime("2026-07-01T08:30:15+08:00", false)
		require.NoError(t, err)
		require.True(t, got.Equal(time.Date(2026, 7, 1, 8, 30, 15, 0, time.FixedZone("", 8*3600))))
	})

	t.Run("RFC3339EndWithTimeOfDayIsNotExtended", func(t *testing.T) {
		got, err := parseQueryTime("2026-07-01T00:00:00Z", true)
		require.NoError(t, err)
		require.True(t, got.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)))
	})

	t.Run("UnixSeconds", func(t *testing.T) {
		got, err := parseQueryTime("1751328000", false)
		require.NoError(t, err)
		require.True(t, got.Equal(time.Unix(1751328000, 0)))
	})

	t.Run("UnixMilliseconds", func(t *testing.T) {
		got, err := parseQueryTime("1751328000123", false)
		require.NoError(t, err)
		require.True(t, got.Equal(time.UnixMilli(1751328000123)))
	})

	t.Run("UnsupportedFormatFails", func(t *testing.T) {
		_, err := parseQueryTime("07/01/2026", false)
		require.Error(t, err)
	})
}

func TestParseTimeRangeQuery(t *testing.T) {
	t.Run("BothBounds", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_start_time=2026-07-01&_end_time=2026-07-02")
		start, end, err := parseTimeRangeQuery(c)
		require.NoError(t, err)
		require.Equal(t, time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local), start)
		require.Equal(t, time.Date(2026, 7, 3, 0, 0, 0, 0, time.Local).Add(-time.Nanosecond), end)
	})

	t.Run("EmptyValuesMeanNoBound", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_start_time=&_end_time=")
		start, end, err := parseTimeRangeQuery(c)
		require.NoError(t, err)
		require.True(t, start.IsZero())
		require.True(t, end.IsZero())
	})

	t.Run("InvalidStartTimeFails", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_start_time=not-a-time")
		_, _, err := parseTimeRangeQuery(c)
		require.Error(t, err)
	})

	t.Run("InvalidEndTimeFails", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_end_time=not-a-time")
		_, _, err := parseTimeRangeQuery(c)
		require.Error(t, err)
	})
}

type expandQueryTestModel struct {
	Children []*expandQueryTestModel
	Parent   *expandQueryTestModel

	model.Base
}

func (*expandQueryTestModel) Expands() []string { return []string{"Children", "Parent"} }

func TestParseExpandQuery(t *testing.T) {
	t.Run("DepthRepeatsSliceExpand", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=Children&_depth=3")
		require.Equal(t, []string{"Children.Children.Children"}, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("NonSliceExpandIgnoresDepth", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=Parent&_depth=3")
		require.Equal(t, []string{"Parent"}, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("AllSelectsEveryModelExpand", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=all")
		require.Equal(t, []string{"Children", "Parent"}, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("ExpandMatchesCaseInsensitively", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=children")
		require.Equal(t, []string{"Children"}, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("UnknownExpandDropped", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=Bogus")
		require.Empty(t, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("OutOfRangeDepthFallsBackToOne", func(t *testing.T) {
		c := newTestGetContext(t, "/items?_expand=Children&_depth=100")
		require.Equal(t, []string{"Children"}, parseExpandQuery(c, &expandQueryTestModel{}))
	})

	t.Run("NoExpandParameterReturnsNothing", func(t *testing.T) {
		c := newTestGetContext(t, "/items")
		require.Empty(t, parseExpandQuery(c, &expandQueryTestModel{}))
	})
}

// newTestGetContext builds a gin context carrying a GET request with the given
// target URL, for exercising query-parameter parsing helpers.
func newTestGetContext(t *testing.T, target string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return c
}
