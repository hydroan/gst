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

// sampleTreeRecord declares the two associations a model preloads, a has-many
// and a belongs-to, beside its own columns.
type sampleTreeRecord struct {
	ID       string              `json:"id" gorm:"primaryKey"`
	Name     string              `json:"name"`
	ParentID string              `json:"parent_id"`
	Children []*sampleTreeRecord `json:"children" gorm:"foreignKey:ParentID"`
	Parent   *sampleTreeRecord   `json:"parent" gorm:"foreignKey:ParentID;references:ID"`
}

func (sampleTreeRecord) TableName() string { return "sample_tree_records" }

// sampleOwnerRecord holds sampleOwnedRecord rows through a has-many, which
// gorm registers on sampleOwnedRecord's schema as well, under a hidden name,
// once it parses the owner.
type sampleOwnerRecord struct {
	ID      string               `json:"id" gorm:"primaryKey"`
	Entries []*sampleOwnedRecord `json:"entries" gorm:"foreignKey:OwnerID"`
}

func (sampleOwnerRecord) TableName() string { return "sample_owner_records" }

// sampleOwnedRecord is held by a sampleOwnerRecord and declares no
// association of its own.
type sampleOwnedRecord struct {
	ID      string `json:"id" gorm:"primaryKey"`
	OwnerID string `json:"owner_id"`
}

func (sampleOwnedRecord) TableName() string { return "sample_owned_records" }

// TestAssociationFieldsListsTheRelationshipFields pins the field paths of a
// model's associations — the relationships it declares itself, not the ones
// gorm registers on it for a model holding it — the per-type caching, and
// the nil-on-failure contract.
func TestAssociationFieldsListsTheRelationshipFields(t *testing.T) {
	paths := AssociationFields(reflect.TypeFor[*sampleTreeRecord]())
	require.Equal(t, [][]int{{3}, {4}}, paths)

	second := AssociationFields(reflect.TypeFor[sampleTreeRecord]())
	require.Equal(t, reflect.ValueOf(paths).Pointer(), reflect.ValueOf(second).Pointer(),
		"pointer and struct type must share one cached list")

	// Resolving the owner first registers its has-many on the owned model's
	// schema; that entry describes the owner's field.
	require.Equal(t, [][]int{{1}}, AssociationFields(reflect.TypeFor[sampleOwnerRecord]()))
	require.Empty(t, AssociationFields(reflect.TypeFor[sampleOwnedRecord]()), "the owner's has-many is no association of the owned model")

	require.Empty(t, AssociationFields(reflect.TypeFor[sampleDocumentRecord]()), "a model without associations has none")
	require.Nil(t, AssociationFields(reflect.TypeFor[string]()),
		"a type without resolvable columns yields nil, not an error")
}

// sampleHolderRecord holds sampleHeldRecord rows through a has-many, so
// parsing it writes into sampleHeldRecord's cached schema.
type sampleHolderRecord struct {
	ID      string              `json:"id" gorm:"primaryKey"`
	Entries []*sampleHeldRecord `json:"entries" gorm:"foreignKey:HolderID"`
}

func (sampleHolderRecord) TableName() string { return "sample_holder_records" }

// sampleHeldRecord is held by a sampleHolderRecord.
type sampleHeldRecord struct {
	ID       string `json:"id" gorm:"primaryKey"`
	HolderID string `json:"holder_id"`
}

func (sampleHeldRecord) TableName() string { return "sample_held_records" }

// TestAssociationFieldsWalksUnderTheSchemaLock pins that the walk over a
// type's relationships takes the lock gorm writes them under: parsing a
// holder registers its has-many on the held type's schema, and here that
// parse runs while another goroutine keeps resolving the held type. The race
// detector, which make test runs under, reports a walk without the lock.
func TestAssociationFieldsWalksUnderTheSchemaLock(t *testing.T) {
	held := reflect.TypeFor[sampleHeldRecord]()
	require.Empty(t, AssociationFields(held))

	stop := make(chan struct{})
	walked := make(chan struct{})
	go func() {
		defer close(walked)
		for {
			select {
			case <-stop:
				return
			default:
			}
			associationFieldsCache.Delete(held)
			AssociationFields(held)
		}
	}()
	require.Equal(t, [][]int{{1}}, AssociationFields(reflect.TypeFor[sampleHolderRecord]()))
	time.Sleep(50 * time.Millisecond)
	close(stop)
	<-walked
}
