package modelschema

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The fingerprintSample variants below each differ from fingerprintSample in
// a single declaration, so a declaration that failed to reach the rendering
// is the only thing that could make two of them read the same. They map to
// one table on purpose, all but fingerprintSampleOtherTable, whose one
// differing declaration is the table name.

type fingerprintSample struct {
	Name string `gorm:"size:64"`

	SampleBase
}

func (*fingerprintSample) TableName() string { return "fingerprint_samples" }

func (*fingerprintSample) Indexes() []Index {
	return []Index{{Fields: []string{"Name"}}}
}

// fingerprintSampleWithoutField drops a column. It is the case a migration
// cannot repair afterwards: it never drops a column a model stopped
// declaring, so the fingerprint has to be what notices.
type fingerprintSampleWithoutField struct {
	SampleBase
}

func (*fingerprintSampleWithoutField) TableName() string { return "fingerprint_samples" }

func (*fingerprintSampleWithoutField) Indexes() []Index {
	return []Index{{Fields: []string{"Name"}}}
}

// fingerprintSampleWiderField keeps the column and moves its definition.
type fingerprintSampleWiderField struct {
	Name string `gorm:"size:128"`

	SampleBase
}

func (*fingerprintSampleWiderField) TableName() string { return "fingerprint_samples" }

func (*fingerprintSampleWiderField) Indexes() []Index {
	return []Index{{Fields: []string{"Name"}}}
}

// fingerprintSampleWithoutIndex drops an index, the other change a migration
// leaves in place once it exists.
type fingerprintSampleWithoutIndex struct {
	Name string `gorm:"size:64"`

	SampleBase
}

func (*fingerprintSampleWithoutIndex) TableName() string { return "fingerprint_samples" }

// fingerprintSampleUniqueIndex keeps the index and makes it unique.
type fingerprintSampleUniqueIndex struct {
	Name string `gorm:"size:64"`

	SampleBase
}

func (*fingerprintSampleUniqueIndex) TableName() string { return "fingerprint_samples" }

func (*fingerprintSampleUniqueIndex) Indexes() []Index {
	return []Index{{Fields: []string{"Name"}, Unique: true}}
}

// fingerprintSampleOtherTable changes nothing but the table it maps to.
type fingerprintSampleOtherTable struct {
	Name string `gorm:"size:64"`

	SampleBase
}

func (*fingerprintSampleOtherTable) TableName() string { return "other_fingerprint_samples" }

func (*fingerprintSampleOtherTable) Indexes() []Index {
	return []Index{{Fields: []string{"Name"}}}
}

// fingerprintSpan is the struct fingerprintSpanSample embeds by tag: gorm
// expands its fields into columns of the model just as it does an anonymous
// embedding.
type fingerprintSpan struct {
	Start int64
	End   int64 `gorm:"column:ends_at"`
}

// fingerprintSpanSample embeds a struct both ways gorm expands one into
// columns: SampleBase anonymously, the span by tag.
type fingerprintSpanSample struct {
	Name string          `gorm:"size:64"`
	Span fingerprintSpan `gorm:"embedded;embeddedPrefix:span_"`

	SampleBase
}

func (*fingerprintSpanSample) TableName() string { return "fingerprint_span_samples" }

func TestModelDeclarationCoversEveryTableDeclaration(t *testing.T) {
	baseline := modelDeclaration(&fingerprintSample{})

	t.Run("TheSameModelDeclaresTheSameThingEveryTime", func(t *testing.T) {
		require.Equal(t, baseline, modelDeclaration(&fingerprintSample{}))
	})

	t.Run("EveryDeclarationAMigrationCannotRepairShowsUp", func(t *testing.T) {
		for name, model := range map[string]any{
			"a dropped field":      &fingerprintSampleWithoutField{},
			"a dropped index":      &fingerprintSampleWithoutIndex{},
			"a widened field":      &fingerprintSampleWiderField{},
			"an index made unique": &fingerprintSampleUniqueIndex{},
			"another table":        &fingerprintSampleOtherTable{},
		} {
			require.NotEqual(t, baseline, modelDeclaration(model), name)
		}
	})

	t.Run("EveryFieldGormExpandsIntoAColumnIsDeclared", func(t *testing.T) {
		// gorm promotes the columns of a struct it expands, embedded
		// anonymously like SampleBase, which carries the primary key and a
		// timestamp, or by the embedded tag like the span; a declaration
		// blind to them would let a change to them pass for no change at all.
		declaration := modelDeclaration(&fingerprintSpanSample{})
		columns, err := Columns(reflect.TypeFor[fingerprintSpanSample]())
		require.NoError(t, err)
		require.NotEmpty(t, columns)
		for _, column := range columns {
			field := column.GoName[strings.LastIndex(column.GoName, ".")+1:]
			require.Contains(t, declaration, " field="+field+" ", column.DBName)
		}
	})

	t.Run("TheTableNameIsDeclared", func(t *testing.T) {
		require.Contains(t, baseline, "table=fingerprint_samples")
	})
}
