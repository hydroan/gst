package modelregistry_test

import (
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/utils/tests"
)

// IndexedSample declares custom indexes that cover embedded Base columns.
type IndexedSample struct {
	Code string `json:"code" gorm:"index:idx_indexed_samples_code"`
	Kind string `json:"kind"`

	modelregistry.Base
}

func (*IndexedSample) GetTableName() string { return "indexed_samples" }

func (*IndexedSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{
		{Fields: []string{"Kind", "CreatedAt"}},
		{Fields: []string{"Code", "Kind"}, Unique: true},
	}
}

// PlainSample does not implement the Indexer capability.
type PlainSample struct {
	Name string `json:"name"`

	modelregistry.Base
}

// EmptyFieldsSample declares an index without fields.
type EmptyFieldsSample struct {
	Kind string

	modelregistry.Base
}

func (*EmptyFieldsSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{}}
}

// UnknownFieldSample references a field that does not exist on the model.
type UnknownFieldSample struct {
	Kind string

	modelregistry.Base
}

func (*UnknownFieldSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Missing"}}}
}

// RepeatedColumnSample repeats the same column inside one index.
type RepeatedColumnSample struct {
	Kind string

	modelregistry.Base
}

func (*RepeatedColumnSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Kind", "Kind"}}}
}

// DuplicateDeclSample declares two indexes with the same column sequence.
type DuplicateDeclSample struct {
	Kind string

	modelregistry.Base
}

func (*DuplicateDeclSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{
		{Fields: []string{"Kind"}},
		{Fields: []string{"Kind"}, Unique: true},
	}
}

// TagNameConflictSample owns a struct tag index whose explicit name collides
// with the name the framework generates for the declaration.
type TagNameConflictSample struct {
	Code string `gorm:"index:idx_tag_name_conflict_samples_kind"`
	Kind string

	modelregistry.Base
}

func (*TagNameConflictSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Kind"}}}
}

// TagColumnsConflictSample owns a struct tag index with the same column
// sequence as the declaration but under a different name.
type TagColumnsConflictSample struct {
	Code string `gorm:"index:custom_code_idx"`

	modelregistry.Base
}

func (*TagColumnsConflictSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Code"}}}
}

func TestParseIndexPlans(t *testing.T) {
	db := newSchemaDB(t)

	t.Run("value and pointer models resolve identically", func(t *testing.T) {
		want := []modelregistry.IndexPlan{
			{Name: "idx_indexed_samples_kind_created_at", Table: "indexed_samples", Columns: []string{"kind", "created_at"}},
			{Name: "uniq_indexed_samples_code_kind", Table: "indexed_samples", Columns: []string{"code", "kind"}, Unique: true},
		}

		plans, err := modelregistry.ParseIndexPlans(db, &IndexedSample{}, "indexed_samples")
		require.NoError(t, err)
		require.Equal(t, want, plans)

		plans, err = modelregistry.ParseIndexPlans(db, IndexedSample{}, "indexed_samples")
		require.NoError(t, err)
		require.Equal(t, want, plans)
	})

	t.Run("empty table name falls back to the schema table", func(t *testing.T) {
		plans, err := modelregistry.ParseIndexPlans(db, &IndexedSample{}, "")
		require.NoError(t, err)
		require.Len(t, plans, 2)
		require.Equal(t, "indexed_samples", plans[0].Table)
	})

	t.Run("models without the capability yield no plans", func(t *testing.T) {
		plans, err := modelregistry.ParseIndexPlans(db, &PlainSample{}, "plain_samples")
		require.NoError(t, err)
		require.Nil(t, plans)
	})
}

func TestParseIndexPlansValidation(t *testing.T) {
	db := newSchemaDB(t)

	for _, tt := range []struct {
		name  string
		model any
		want  string
	}{
		{"empty fields", &EmptyFieldsSample{}, "at least one field"},
		{"unknown field", &UnknownFieldSample{}, `unknown field "Missing"`},
		{"repeated column", &RepeatedColumnSample{}, `repeats column "kind"`},
		{"duplicate declaration", &DuplicateDeclSample{}, "duplicate custom index"},
		{"tag index name conflict", &TagNameConflictSample{}, "conflicts with struct tag index"},
		{"tag index columns conflict", &TagColumnsConflictSample{}, `duplicates struct tag index "custom_code_idx"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := modelregistry.ParseIndexPlans(db, tt.model, "")
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestParseIndexPlansTruncatesLongNames(t *testing.T) {
	db := newSchemaDB(t)
	table := "an_extremely_long_table_name_used_to_exercise_truncation"

	plans, err := modelregistry.ParseIndexPlans(db, &IndexedSample{}, table)
	require.NoError(t, err)
	require.Len(t, plans, 2)
	require.Len(t, plans[0].Name, 64)
	require.True(t, strings.HasPrefix(plans[0].Name, "idx_"+table[:20]))

	// Truncation must stay deterministic across runs.
	again, err := modelregistry.ParseIndexPlans(db, &IndexedSample{}, table)
	require.NoError(t, err)
	require.Equal(t, plans[0].Name, again[0].Name)
}

func TestIndexPlanCreateSQL(t *testing.T) {
	plan := modelregistry.IndexPlan{
		Name:    "idx_samples_kind_created_at",
		Table:   "samples",
		Columns: []string{"kind", "created_at"},
	}
	require.Equal(t,
		"CREATE INDEX `idx_samples_kind_created_at` ON `samples` (`kind`,`created_at`)",
		plan.CreateSQL(mysql.New(mysql.Config{})))

	unique := modelregistry.IndexPlan{
		Name:    "uniq_samples_code",
		Table:   "samples",
		Columns: []string{"code"},
		Unique:  true,
	}
	require.Equal(t,
		`CREATE UNIQUE INDEX "uniq_samples_code" ON "samples" ("code")`,
		unique.CreateSQL(postgres.New(postgres.Config{})))
}

// newSchemaDB opens a dry-run gorm handle that only serves schema parsing.
func newSchemaDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(tests.DummyDialector{}, &gorm.Config{DryRun: true})
	require.NoError(t, err)
	return db
}
