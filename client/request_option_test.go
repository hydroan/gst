package client

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRequestOptionsEncodeFrameworkParams(t *testing.T) {
	cfg := newRequestConfig([]RequestOption{
		WithPage(2, 20),
		WithSortBy("created_at desc"),
		WithExpand("children", 3),
		WithCursor("id", "cursor-token", true),
	})

	encoded, err := cfg.encode()
	require.NoError(t, err)
	// Parameter names come from the url tags on model.Query.
	require.Contains(t, encoded, "_page=2")
	require.Contains(t, encoded, "_size=20")
	require.Contains(t, encoded, "_sort_by=created_at+desc")
	require.Contains(t, encoded, "_expand=children")
	require.Contains(t, encoded, "_depth=3")
	require.Contains(t, encoded, "_cursor_field=id")
	require.Contains(t, encoded, "_cursor_value=cursor-token")
	require.Contains(t, encoded, "_cursor_next=true")
}

func TestWithQueryEncodesFreeFormPairs(t *testing.T) {
	cfg := newRequestConfig([]RequestOption{
		WithQuery("kind", "sample", "count", 3, "enabled", true, "name[like]", "reco"),
	})

	encoded, err := cfg.encode()
	require.NoError(t, err)
	require.Contains(t, encoded, "kind=sample")
	require.Contains(t, encoded, "count=3")
	require.Contains(t, encoded, "enabled=true")
	require.Contains(t, encoded, "name%5Blike%5D=reco")
}

func TestWithQueryKeepsEmptyValuesAligned(t *testing.T) {
	// An empty value must not shift later pairs; the old implementation
	// dropped empty strings and mispaired everything after them.
	cfg := newRequestConfig([]RequestOption{WithQuery("a", "", "b", "2")})

	encoded, err := cfg.encode()
	require.NoError(t, err)
	require.Contains(t, encoded, "a=")
	require.Contains(t, encoded, "b=2")
}

func TestWithQueryDropsTrailingKeyWithoutValue(t *testing.T) {
	cfg := newRequestConfig([]RequestOption{WithQuery("a", "1", "orphan")})

	encoded, err := cfg.encode()
	require.NoError(t, err)
	require.Contains(t, encoded, "a=1")
	require.NotContains(t, encoded, "orphan")
}

func TestRequestConfigEncodesNothingByDefault(t *testing.T) {
	encoded, err := newRequestConfig(nil).encode()
	require.NoError(t, err)
	require.Empty(t, encoded)
}

func TestWithTimeRangeEncodesBothBoundsAsRFC3339(t *testing.T) {
	from := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cfg := newRequestConfig([]RequestOption{WithTimeRange("created_at", from, from.Add(48*time.Hour))})

	encoded, err := cfg.encode()
	require.NoError(t, err)
	require.Contains(t, encoded, "created_at%5Bgte%5D=2026-01-02T03%3A04%3A05Z")
	require.Contains(t, encoded, "created_at%5Blte%5D=2026-01-04T03%3A04%3A05Z")
}

func TestWithTimeRangeLeavesAZeroBoundUnset(t *testing.T) {
	moment := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	fromOnly, err := newRequestConfig([]RequestOption{WithTimeRange("created_at", moment, time.Time{})}).encode()
	require.NoError(t, err)
	require.Contains(t, fromOnly, "created_at%5Bgte%5D=")
	require.NotContains(t, fromOnly, "%5Blte%5D")

	toOnly, err := newRequestConfig([]RequestOption{WithTimeRange("created_at", time.Time{}, moment)}).encode()
	require.NoError(t, err)
	require.Contains(t, toOnly, "created_at%5Blte%5D=")
	require.NotContains(t, toOnly, "%5Bgte%5D")
}

func TestWithTimeRangeAddsNothingWithoutColumnOrBounds(t *testing.T) {
	moment := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	blankColumn, err := newRequestConfig([]RequestOption{WithTimeRange("  ", moment, moment)}).encode()
	require.NoError(t, err)
	require.Empty(t, blankColumn)

	zeroBounds, err := newRequestConfig([]RequestOption{WithTimeRange("created_at", time.Time{}, time.Time{})}).encode()
	require.NoError(t, err)
	require.Empty(t, zeroBounds)
}
