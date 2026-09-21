package modelschema

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// TestGoNameIndexResolvesOnceAndCaches pins both properties of the cached
// index: it carries the same content as deriving the by-Go-name transform
// from the parsed columns, and repeated calls hand back the very same map —
// what makes it safe to consult on every request without rebuilding.
func TestGoNameIndexResolvesOnceAndCaches(t *testing.T) {
	first, err := GoNameIndex(reflect.TypeFor[*sampleRecord]())
	require.NoError(t, err)

	parsed, err := Columns(reflect.TypeFor[sampleRecord]())
	require.NoError(t, err)
	require.Equal(t, byGoName(parsed), first)

	second, err := GoNameIndex(reflect.TypeFor[sampleRecord]())
	require.NoError(t, err)
	require.Equal(t, reflect.ValueOf(first).Pointer(), reflect.ValueOf(second).Pointer(),
		"pointer and struct type must share one cached index")
}

// TestFilterableIndexKeepsOnlyClientFilterableColumns pins the index's two
// jobs: keying by the URL parameter name and keeping json-hidden columns out
// of the client-filterable set, plus the same per-type caching as above.
func TestFilterableIndexKeepsOnlyClientFilterableColumns(t *testing.T) {
	index, err := FilterableIndex(reflect.TypeFor[*sampleRecord]())
	require.NoError(t, err)

	require.Equal(t, "query_tagged", index["alias"].DBName, "the index is keyed by URL parameter name")
	_, hidden := index["hidden"]
	require.False(t, hidden, "a json-hidden column must not be client-filterable")

	second, err := FilterableIndex(reflect.TypeFor[sampleRecord]())
	require.NoError(t, err)
	require.Equal(t, reflect.ValueOf(index).Pointer(), reflect.ValueOf(second).Pointer(),
		"pointer and struct type must share one cached index")
}

// sampleDocumentRecord carries one column of each type the set derivations
// look for, plus a plain column that must stay out of both sets.
type sampleDocumentRecord struct {
	ID        string         `json:"id" gorm:"primaryKey"`
	Payload   datatypes.JSON `json:"payload"`
	Note      string         `json:"note"`
	CreatedAt time.Time      `json:"created_at"`
}

func (sampleDocumentRecord) TableName() string { return "sample_document_records" }

// TestTimeColumnSetReportsTimeColumns pins the set's content by database
// name, the per-type caching, and the nil-on-failure contract.
func TestTimeColumnSetReportsTimeColumns(t *testing.T) {
	set := TimeColumnSet(reflect.TypeFor[*sampleDocumentRecord]())
	require.Contains(t, set, "created_at")
	require.NotContains(t, set, "note")
	require.NotContains(t, set, "payload")

	second := TimeColumnSet(reflect.TypeFor[sampleDocumentRecord]())
	require.Equal(t, reflect.ValueOf(set).Pointer(), reflect.ValueOf(second).Pointer(),
		"pointer and struct type must share one cached set")

	require.Nil(t, TimeColumnSet(reflect.TypeFor[string]()),
		"a type without resolvable columns yields nil, not an error")
}

// TestJSONColumnSetReportsJSONColumns pins the same three properties for the
// JSON-typed column set.
func TestJSONColumnSetReportsJSONColumns(t *testing.T) {
	set := JSONColumnSet(reflect.TypeFor[*sampleDocumentRecord]())
	require.Contains(t, set, "payload")
	require.NotContains(t, set, "note")
	require.NotContains(t, set, "created_at")

	second := JSONColumnSet(reflect.TypeFor[sampleDocumentRecord]())
	require.Equal(t, reflect.ValueOf(set).Pointer(), reflect.ValueOf(second).Pointer(),
		"pointer and struct type must share one cached set")

	require.Nil(t, JSONColumnSet(reflect.TypeFor[string]()),
		"a type without resolvable columns yields nil, not an error")
}
