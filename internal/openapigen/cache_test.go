package openapigen

import (
	"reflect"
	"testing"
	"time"

	"github.com/hydroan/gst/types/consts"
)

// CacheTestUser represents a user with comments for testing
type CacheTestUser struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type CacheTestUserWithoutComment struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestParseStructCommentCache(t *testing.T) {
	// Clear cache before test
	structCommentMutex.Lock()
	structCommentCache = make(map[string]string)
	structCommentMutex.Unlock()

	// First call - should parse and cache
	start := time.Now()
	result1 := parseStructComment(&CacheTestUser{})
	duration1 := time.Since(start)

	// Second call - should use cache
	start = time.Now()
	result2 := parseStructComment(&CacheTestUser{})
	duration2 := time.Since(start)

	// Results should be the same
	if result1 != result2 {
		t.Errorf("Cache results don't match: %s vs %s", result1, result2)
	}

	// Second call should be faster (using cache)
	if duration2 > duration1 {
		t.Logf("Warning: Second call (%v) was not faster than first call (%v)", duration2, duration1)
	}

	// Check cache contains the result
	structCommentMutex.RLock()
	cacheKey := "github.com/hydroan/gst/internal/openapigen.CacheTestUser"
	cachedValue, exists := structCommentCache[cacheKey]
	structCommentMutex.RUnlock()

	if !exists {
		t.Error("Expected cache to contain the result")
	}

	if cachedValue != result1 {
		t.Errorf("Cached value doesn't match result: %s vs %s", cachedValue, result1)
	}

	t.Logf("First call duration: %v, Second call duration: %v", duration1, duration2)
	t.Logf("Parsed comment: %s", result1)
}

func TestSummaryWithStructComment(t *testing.T) {
	// Test with a struct that has comments
	typ := reflect.TypeFor[[]*CacheTestUser]()
	result := summary("/api/users", consts.List, typ)
	expected := "CacheTestUser represents a user with comments for testing"
	if result != expected {
		t.Errorf("Expected summary to use struct comment: '%s', got: '%s'", expected, result)
	}

	// Test with a struct without comments - should fallback to original logic
	typ2 := reflect.TypeFor[[]*CacheTestUserWithoutComment]()
	result2 := summary("/api/users", consts.List, typ2)
	expected2 := "list CacheTestUserWithoutComments" // pluralized
	if result2 != expected2 {
		t.Errorf("Expected summary fallback: '%s', got: '%s'", expected2, result2)
	}

	// Test with non-slice type
	typ3 := reflect.TypeFor[CacheTestUser]()
	result3 := summary("/api/users", consts.Create, typ3)
	expected3 := "CacheTestUser represents a user with comments for testing"
	if result3 != expected3 {
		t.Errorf("Expected summary for non-slice type: '%s', got: '%s'", expected3, result3)
	}
}
