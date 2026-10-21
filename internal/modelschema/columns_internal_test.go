package modelschema

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	gormschema "gorm.io/gorm/schema"
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

// samplePart is embedded by name three times in sampleEmbedding, beside a
// top-level field of the same name.
type samplePart struct {
	Label string `json:"label"`
}

// sampleEmbedding embeds samplePart by name: gorm lifts the field of each into
// a column of its own, and only the path that reaches it tells them apart.
// Third is reached through a pointer, and Hidden is kept out of JSON as a
// whole.
type sampleEmbedding struct {
	SampleBase

	Label  string      `json:"label"`
	First  samplePart  `json:"first" gorm:"embedded;embeddedPrefix:first_"`
	Second samplePart  `json:"second_part" gorm:"embedded;embeddedPrefix:second_"`
	Third  *samplePart `json:"third" gorm:"embedded;embeddedPrefix:third_"`
	Hidden samplePart  `json:"-" gorm:"embedded;embeddedPrefix:hidden_"`
}

func (sampleEmbedding) TableName() string { return "sample_embeddings" }

func TestColumnsNameEmbeddedFieldsByPath(t *testing.T) {
	parsed, err := Columns(reflect.TypeFor[sampleEmbedding]())
	require.NoError(t, err)
	byDBName := make(map[string]Column, len(parsed))
	for _, col := range parsed {
		byDBName[col.DBName] = col
	}

	// The Go name is the selector path Go writes, the URL name the path the
	// field takes in JSON, each step named by its own rule; an anonymous
	// embedded struct adds no step.
	for dbName, want := range map[string]struct{ goName, queryName string }{
		"label":        {"Label", "label"},
		"first_label":  {"First.Label", "first.label"},
		"second_label": {"Second.Label", "second_part.label"},
		"third_label":  {"Third.Label", "third.label"},
		"id":           {"ID", "id"},
	} {
		require.Equal(t, want.goName, byDBName[dbName].GoName, dbName)
		require.Equal(t, want.queryName, byDBName[dbName].QueryName, dbName)
		require.True(t, byDBName[dbName].Filterable, dbName)
	}

	hidden := byDBName["hidden_label"]
	require.Equal(t, "Hidden.Label", hidden.GoName)
	require.False(t, hidden.Filterable, "json:\"-\" on the struct holding a field keeps the field out of JSON too")

	// The index path is the one reflect follows, a step through the embedded
	// pointer Third included, where gorm records -i-1.
	require.Equal(t, []int{4, 0}, byDBName["third_label"].Index)
	require.Equal(t, "Label", reflect.TypeFor[sampleEmbedding]().Field(4).Type.Elem().Field(0).Name)
}

func TestColumnsCarryGormDataTypes(t *testing.T) {
	// GORMDataType is what gorm reads from the Go type; DataType is the
	// column type it resolves, which a type tag replaces.
	type typed struct {
		ID  string    `gorm:"primaryKey"`
		At  time.Time `gorm:"type:datetime(6)"`
		Doc string    `gorm:"type:json"`
	}
	parsed, err := Columns(reflect.TypeFor[typed]())
	require.NoError(t, err)
	cols := indexByQueryName(parsed)
	require.Equal(t, gormschema.String, cols["id"].DataType)
	require.Equal(t, gormschema.String, cols["id"].GORMDataType)
	require.Equal(t, gormschema.DataType("datetime(6)"), cols["at"].DataType)
	require.Equal(t, gormschema.Time, cols["at"].GORMDataType)
	require.Equal(t, gormschema.DataType("json"), cols["doc"].DataType)
	require.Equal(t, gormschema.String, cols["doc"].GORMDataType)
}

func TestColumnsReportUpdatable(t *testing.T) {
	// An update writes the columns gorm writes on update: not one written on
	// insert only, nor a read-only one.
	type sealed struct {
		ID    string `gorm:"primaryKey"`
		Name  string
		Code  string `gorm:"<-:create"`
		Mark  string `gorm:"->"`
		Stamp string `gorm:"<-:update"`
	}
	parsed, err := Columns(reflect.TypeFor[sealed]())
	require.NoError(t, err)
	cols := indexByQueryName(parsed)
	require.True(t, cols["name"].Updatable)
	require.False(t, cols["code"].Updatable)
	require.False(t, cols["mark"].Updatable)
	require.True(t, cols["stamp"].Updatable)
}

func TestColumnsReportNotNull(t *testing.T) {
	// The schema forbids NULL on a NOT NULL column and on the primary key,
	// whatever the Go type could hold.
	type constrained struct {
		ID   string  `gorm:"primaryKey"`
		Code *string `gorm:"not null"`
		Note *string
	}
	parsed, err := Columns(reflect.TypeFor[constrained]())
	require.NoError(t, err)
	cols := indexByQueryName(parsed)
	require.True(t, cols["id"].NotNull)
	require.True(t, cols["code"].NotNull)
	require.False(t, cols["note"].NotNull)
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
