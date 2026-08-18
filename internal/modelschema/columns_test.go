package modelschema

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
