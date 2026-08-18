package modelschema

import (
	"database/sql/driver"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// SampleBase mirrors an embedded framework base struct: its fields are lifted
// into the owning table as ordinary columns. The type name must be exported:
// an anonymous field of an unexported type is unexported itself, and gorm
// then skips the whole embedded struct.
type SampleBase struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at" query:"-"`
}

// sampleRelated is the target of an association: gorm resolves it through a
// foreign key, so the association field itself never becomes a column.
type sampleRelated struct {
	ID       string `gorm:"primaryKey"`
	RecordID string
}

// sampleAssociating declares its primary key directly rather than through an
// embedded struct. That is required here: gorm cannot resolve an association
// foreign key when the primary key is lifted from an embedded struct, and
// rejects the whole model instead. Models that embed a framework base
// therefore cannot carry association fields at all, which is why the
// embedded-base fixture below has none.
type sampleAssociating struct {
	ID      string          `json:"id" gorm:"primaryKey"`
	Name    string          `json:"name"`
	Related []sampleRelated `json:"related" gorm:"foreignKey:RecordID"`
}

func (sampleAssociating) TableName() string { return "sample_associating" }

type sampleRecord struct {
	SampleBase

	Name        string `json:"name"`
	GroupIDs    string `json:"group_ids"`
	MD5Hash     string `json:"md5_hash"`
	Renamed     string `json:"renamed" gorm:"column:custom_column"`
	QueryTagged string `json:"query_tagged" query:"alias"`
	Hidden      string `json:"-"`
	Skipped     string `json:"skipped" gorm:"-"`
	SkippedAll  string `json:"skipped_all" gorm:"-:all"`
	NoMigration string `json:"no_migration" gorm:"-:migration"`
	unexported  string //nolint:unused
}

func (sampleRecord) TableName() string { return "sample_records" }

// indexByQueryName indexes columns by URL parameter name for assertions.
func indexByQueryName(columns []Column) map[string]Column {
	indexed := make(map[string]Column, len(columns))
	for _, col := range columns {
		indexed[col.QueryName] = col
	}
	return indexed
}

func TestColumns(t *testing.T) {
	parsed, err := Columns(reflect.TypeFor[sampleRecord]())
	require.NoError(t, err)
	cols := indexByQueryName(parsed)

	t.Run("ResolvesDBNameThroughGorm", func(t *testing.T) {
		// gorm's commonInitialisms handling differs from a plain snake case
		// conversion, so these two names are the regression anchors.
		require.Equal(t, "group_ids", cols["group_ids"].DBName)
		require.Equal(t, "md5_hash", cols["md5_hash"].DBName)
	})

	t.Run("ExplicitColumnTagWins", func(t *testing.T) {
		require.Equal(t, "custom_column", cols["renamed"].DBName,
			"the column tag decides the database name")
		require.Equal(t, "renamed", cols["renamed"].QueryName,
			"the URL parameter name still comes from the json tag")
	})

	t.Run("QueryTagWinsForQueryName", func(t *testing.T) {
		col, ok := cols["alias"]
		require.True(t, ok, "the query tag names the URL parameter")
		require.Equal(t, "query_tagged", col.DBName)
	})

	t.Run("SkipsIgnoredAndUnexportedFields", func(t *testing.T) {
		for _, name := range []string{"skipped", "skipped_all", "unexported"} {
			_, ok := cols[name]
			require.False(t, ok, "field %q must not be a column", name)
		}
	})

	t.Run("KeepsMigrationIgnoredColumn", func(t *testing.T) {
		col, ok := cols["no_migration"]
		require.True(t, ok, `gorm:"-:migration" only skips migration, the column still exists`)
		require.Equal(t, "no_migration", col.DBName)
	})

	t.Run("LiftsEmbeddedBaseFields", func(t *testing.T) {
		require.Equal(t, "id", cols["id"].DBName)
		require.Equal(t, "created_at", cols["created_at"].DBName)
		require.Equal(t, reflect.TypeFor[time.Time](), cols["created_at"].Type)
	})

	t.Run("CarriesFieldType", func(t *testing.T) {
		require.Equal(t, reflect.TypeFor[string](), cols["name"].Type)
	})
}

// TestGoNameIndexResolvesOnceAndCaches pins both properties of the cached
// index: it carries the same content as deriving the by-Go-name transform from the parsed
// columns, and repeated calls hand back the very same map — what makes it
// safe to consult on every request without rebuilding.
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

func TestColumnsSkipsAssociationFields(t *testing.T) {
	parsed, err := Columns(reflect.TypeFor[sampleAssociating]())
	require.NoError(t, err)
	cols := indexByQueryName(parsed)

	require.Contains(t, cols, "id")
	require.Contains(t, cols, "name")
	_, ok := cols["related"]
	require.False(t, ok, "an association is resolved through a foreign key, not a column")
}

// sampleShadowing declares a column that also exists in the embedded base
// struct, which is how a model overrides the framework primary key.
type sampleShadowing struct {
	SampleBase

	ID int64 `json:"id" gorm:"primaryKey;autoIncrement:true"`
}

func (sampleShadowing) TableName() string { return "sample_shadowing" }

func TestColumnsDeduplicatesShadowedFields(t *testing.T) {
	parsed, err := Columns(reflect.TypeFor[sampleShadowing]())
	require.NoError(t, err)

	seen := 0
	var idColumn Column
	for _, col := range parsed {
		if col.DBName == "id" {
			seen++
			idColumn = col
		}
	}
	require.Equal(t, 1, seen, "a shadowed embedded field must not yield a second column")
	require.Equal(t, reflect.TypeFor[int64](), idColumn.Type,
		"the field declared on the model wins over the embedded one")
}

func TestColumnsRejectsNonStruct(t *testing.T) {
	_, err := Columns(reflect.TypeFor[string]())
	require.Error(t, err)
}

func TestClassifyColumn(t *testing.T) {
	type namedAmount int64
	type namedLabel string
	type namedTime time.Time

	t.Run("Numeric", func(t *testing.T) {
		for _, typ := range []reflect.Type{
			reflect.TypeFor[int](), reflect.TypeFor[int8](), reflect.TypeFor[int16](),
			reflect.TypeFor[int32](), reflect.TypeFor[int64](),
			reflect.TypeFor[uint](), reflect.TypeFor[uint8](), reflect.TypeFor[uint16](),
			reflect.TypeFor[uint32](), reflect.TypeFor[uint64](),
			reflect.TypeFor[float32](), reflect.TypeFor[float64](),
			// A named numeric type is still numeric: its kind is what SUM acts on.
			reflect.TypeFor[namedAmount](),
			// A pointer column aggregates its pointed-to value.
			reflect.TypeFor[*int64](),
		} {
			require.Equal(t, ColumnClassNumeric, ClassifyColumn(typ), typ.String())
		}
	})

	t.Run("Time", func(t *testing.T) {
		require.Equal(t, ColumnClassTime, ClassifyColumn(reflect.TypeFor[time.Time]()))
		require.Equal(t, ColumnClassTime, ClassifyColumn(reflect.TypeFor[*time.Time]()))
	})

	t.Run("Other", func(t *testing.T) {
		for _, typ := range []reflect.Type{
			reflect.TypeFor[string](), reflect.TypeFor[bool](),
			reflect.TypeFor[namedLabel](), reflect.TypeFor[[]byte](),
			// A named type whose underlying type is time.Time is not time.Time,
			// so a TimeColumn typed on it would not compile.
			reflect.TypeFor[namedTime](),
		} {
			require.Equal(t, ColumnClassOther, ClassifyColumn(typ), typ.String())
		}
		require.Equal(t, ColumnClassOther, ClassifyColumn(nil))
	})

	t.Run("RejectsValuerHeuristic", func(t *testing.T) {
		// Every type here implements driver.Valuer while being stored as text.
		// Classifying by that interface instead of by kind would call them
		// numeric, and MySQL and SQLite answer SUM over text with 0 rather than
		// an error, so the mistake would reach a report as a wrong number.
		for _, typ := range []reflect.Type{
			reflect.TypeFor[textValuer](), reflect.TypeFor[gorm.DeletedAt](),
		} {
			require.Equal(t, ColumnClassOther, ClassifyColumn(typ), typ.String())
		}
	})
}

// pointerDataType declares its gorm data type on the pointer receiver, the
// way a custom wrapper with pointer-based Scan/Value methods would.
type pointerDataType struct{}

func (*pointerDataType) GormDataType() string { return "JSONB" }

func TestIsJSONType(t *testing.T) {
	t.Run("JSON", func(t *testing.T) {
		for _, typ := range []reflect.Type{
			reflect.TypeFor[datatypes.JSON](),
			reflect.TypeFor[datatypes.JSONType[map[string]string]](),
			reflect.TypeFor[datatypes.JSONSlice[string]](),
			reflect.TypeFor[datatypes.JSONMap](),
			// A pointer column stores the pointed-to document.
			reflect.TypeFor[*datatypes.JSONSlice[string]](),
			// The declaration may hang off the pointer receiver, and the
			// spelling is case-insensitive.
			reflect.TypeFor[pointerDataType](),
		} {
			require.True(t, IsJSONType(typ), typ.String())
		}
	})

	t.Run("NotJSON", func(t *testing.T) {
		for _, typ := range []reflect.Type{
			reflect.TypeFor[string](), reflect.TypeFor[[]string](),
			reflect.TypeFor[[]byte](), reflect.TypeFor[gorm.DeletedAt](),
			reflect.TypeFor[time.Time](),
		} {
			require.False(t, IsJSONType(typ), typ.String())
		}
		require.False(t, IsJSONType(nil))
	})
}

// textValuer stands for the uuid, JSON and enum types that are stored as text
// and implement driver.Valuer.
type textValuer struct{ raw string }

func (v textValuer) Value() (driver.Value, error) { return v.raw, nil }

func TestTableName(t *testing.T) {
	// gorm's own TableName method decides the name when a model declares one.
	name, err := TableName(reflect.TypeFor[sampleRecord]())
	require.NoError(t, err)
	require.Equal(t, "sample_records", name)

	t.Run("DerivesFromNamingStrategy", func(t *testing.T) {
		// sampleEntry declares no table name at all, which is the case the
		// framework base models are in: their GetTableName returns "", so the
		// name has to come from gorm's naming strategy rather than from the
		// model.
		name, err := TableName(reflect.TypeFor[sampleEntry]())
		require.NoError(t, err)
		require.Equal(t, "sample_entries", name)
	})

	t.Run("AcceptsPointer", func(t *testing.T) {
		name, err := TableName(reflect.TypeFor[*sampleRecord]())
		require.NoError(t, err)
		require.Equal(t, "sample_records", name)
	})

	t.Run("RejectsNonStruct", func(t *testing.T) {
		_, err := TableName(reflect.TypeFor[string]())
		require.Error(t, err)
	})
}

// sampleEntry declares no table name, so gorm's naming strategy names it.
type sampleEntry struct {
	ID string `gorm:"primaryKey"`
}
