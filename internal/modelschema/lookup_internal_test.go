package modelschema

import (
	"database/sql"
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

// TestIndexesTellEmbeddedFieldsApart pins that both indexes key a column of
// a struct embedded by name by its path, so a field sharing its name with the
// model's own, or with another embedding's, takes none of their columns.
func TestIndexesTellEmbeddedFieldsApart(t *testing.T) {
	filterable, err := FilterableIndex(reflect.TypeFor[sampleEmbedding]())
	require.NoError(t, err)
	require.Equal(t, "label", filterable["label"].DBName)
	require.Equal(t, "first_label", filterable["first.label"].DBName)
	require.Equal(t, "second_label", filterable["second_part.label"].DBName)

	byGoName, err := GoNameIndex(reflect.TypeFor[sampleEmbedding]())
	require.NoError(t, err)
	require.Equal(t, "label", byGoName["Label"].DBName)
	require.Equal(t, "first_label", byGoName["First.Label"].DBName)
	require.Equal(t, "second_label", byGoName["Second.Label"].DBName)
}

// sampleDocumentRecord carries one column of each type the set derivations
// look for, plus a plain column that must stay out of every set. Settled,
// Tally and Doc store a time, a number and a JSON document that only gorm's
// reading of the field tells, and Clock is a time of day gorm gives the time
// data type of an instant.
type sampleDocumentRecord struct {
	ID        string         `json:"id" gorm:"primaryKey"`
	Payload   datatypes.JSON `json:"payload"`
	Note      string         `json:"note"`
	Count     int            `json:"count"`
	CreatedAt time.Time      `json:"created_at"`
	Day       datatypes.Date `json:"day"`
	Settled   sql.NullTime   `json:"settled"`
	Tally     sql.NullInt64  `json:"tally"`
	Doc       string         `json:"doc" gorm:"type:json"`
	Clock     datatypes.Time `json:"clock"`
}

func (sampleDocumentRecord) TableName() string { return "sample_document_records" }

// TestTimeColumnSetReportsTimeColumns pins the set's content by database
// name, the per-type caching, and the nil-on-failure contract.
func TestTimeColumnSetReportsTimeColumns(t *testing.T) {
	set := TimeColumnSet(reflect.TypeFor[*sampleDocumentRecord]())
	require.Contains(t, set, "created_at")
	require.Contains(t, set, "day", "a date column compares as a time")
	require.Contains(t, set, "settled", "gorm stores the time sql.NullTime holds")
	require.NotContains(t, set, "clock", "a time of day is no instant")
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
	require.Contains(t, set, "doc", "the type tag declares a JSON column")
	require.NotContains(t, set, "note")
	require.NotContains(t, set, "created_at")

	second := JSONColumnSet(reflect.TypeFor[sampleDocumentRecord]())
	require.Equal(t, reflect.ValueOf(set).Pointer(), reflect.ValueOf(second).Pointer(),
		"pointer and struct type must share one cached set")

	require.Nil(t, JSONColumnSet(reflect.TypeFor[string]()),
		"a type without resolvable columns yields nil, not an error")
}

// TestNumericColumnSetReportsNumericColumns pins the same three properties
// for the numeric column set.
func TestNumericColumnSetReportsNumericColumns(t *testing.T) {
	set := NumericColumnSet(reflect.TypeFor[*sampleDocumentRecord]())
	require.Contains(t, set, "count")
	require.Contains(t, set, "tally", "gorm stores the integer sql.NullInt64 holds")
	require.NotContains(t, set, "clock", "a time of day is no number, whatever its Go kind")
	require.NotContains(t, set, "note")
	require.NotContains(t, set, "payload")
	require.NotContains(t, set, "created_at")

	second := NumericColumnSet(reflect.TypeFor[sampleDocumentRecord]())
	require.Equal(t, reflect.ValueOf(set).Pointer(), reflect.ValueOf(second).Pointer(),
		"pointer and struct type must share one cached set")

	require.Nil(t, NumericColumnSet(reflect.TypeFor[string]()),
		"a type without resolvable columns yields nil, not an error")
}
