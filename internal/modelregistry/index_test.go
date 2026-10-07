package modelregistry_test

import (
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
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

func (*IndexedSample) TableName() string { return "indexed_samples" }

func (*IndexedSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{
		{Fields: []string{"Kind", "CreatedAt"}},
		{Fields: []string{"Code", "Kind"}, Unique: true},
	}
}

// PlainSample does not implement the indexer capability.
type PlainSample struct {
	Name string `json:"name"`

	modelregistry.Base
}

// EmptyFieldsSample declares an index without fields.
type EmptyFieldsSample struct {
	Kind string

	modelregistry.Base
}

func (*EmptyFieldsSample) TableName() string { return "empty_fields_samples" }

func (*EmptyFieldsSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{}}
}

// UnknownFieldSample references a field that does not exist on the model.
type UnknownFieldSample struct {
	Kind string

	modelregistry.Base
}

func (*UnknownFieldSample) TableName() string { return "unknown_field_samples" }

func (*UnknownFieldSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Missing"}}}
}

// RepeatedColumnSample repeats the same column inside one index.
type RepeatedColumnSample struct {
	Kind string

	modelregistry.Base
}

func (*RepeatedColumnSample) TableName() string { return "repeated_column_samples" }

func (*RepeatedColumnSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Kind", "Kind"}}}
}

// DuplicateDeclSample declares two indexes with the same column sequence.
type DuplicateDeclSample struct {
	Kind string

	modelregistry.Base
}

func (*DuplicateDeclSample) TableName() string { return "duplicate_decl_samples" }

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

func (*TagNameConflictSample) TableName() string { return "tag_name_conflict_samples" }

func (*TagNameConflictSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Kind"}}}
}

// TagColumnsConflictSample owns a struct tag index with the same column
// sequence as the declaration but under a different name.
type TagColumnsConflictSample struct {
	Code string `gorm:"index:custom_code_idx"`

	modelregistry.Base
}

func (*TagColumnsConflictSample) TableName() string { return "tag_columns_conflict_samples" }

func (*TagColumnsConflictSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Code"}}}
}

// UniqueTagConflictSample marks a column unique through the bare unique tag
// and declares a unique index on the same single column, which would create
// two unique indexes over one column.
type UniqueTagConflictSample struct {
	Code string `gorm:"unique"`

	modelregistry.Base
}

func (*UniqueTagConflictSample) TableName() string { return "unique_tag_conflict_samples" }

func (*UniqueTagConflictSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Code"}, Unique: true}}
}

// NoTableNameSample implements indexer but never declares its table name.
type NoTableNameSample struct {
	Kind string

	modelregistry.Base
}

func (*NoTableNameSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Kind"}}}
}

// ColumnNameSample references its column by database name instead of the Go
// field name, which the declaration contract rejects.
type ColumnNameSample struct {
	Kind string

	modelregistry.Base
}

func (*ColumnNameSample) TableName() string { return "column_name_samples" }

func (*ColumnNameSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"kind"}}}
}

func TestParseIndexPlans(t *testing.T) {
	db := newSchemaDB(t)

	t.Run("value and pointer models resolve identically", func(t *testing.T) {
		want := []modelregistry.IndexPlan{
			{Name: "idx_indexed_samples_kind_created_at", Table: "indexed_samples", Columns: []string{"kind", "created_at"}},
			{Name: "uniq_indexed_samples_code_kind", Table: "indexed_samples", Columns: []string{"code", "kind"}, Unique: true},
		}

		plans, err := modelregistry.ParseIndexPlans(db, &IndexedSample{})
		require.NoError(t, err)
		require.Equal(t, want, plans)

		plans, err = modelregistry.ParseIndexPlans(db, IndexedSample{})
		require.NoError(t, err)
		require.Equal(t, want, plans)
	})

	t.Run("models without the capability yield no plans", func(t *testing.T) {
		plans, err := modelregistry.ParseIndexPlans(db, &PlainSample{})
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
		{"unique tag columns conflict", &UniqueTagConflictSample{}, `custom index on column "code" duplicates the unique struct tag`},
		{"missing table name", &NoTableNameSample{}, "must declare an explicit table name"},
		{"column name instead of field name", &ColumnNameSample{}, `unknown field "kind"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := modelregistry.ParseIndexPlans(db, tt.model)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

// LongTableNameSample carries a table name long enough to push generated
// index names past the identifier limit.
type LongTableNameSample struct {
	Code string
	Kind string

	modelregistry.Base
}

func (*LongTableNameSample) TableName() string {
	return "an_extremely_long_table_name_used_to_exercise_truncation"
}

func (*LongTableNameSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Kind", "CreatedAt"}}}
}

func TestParseIndexPlansTruncatesLongNames(t *testing.T) {
	db := newSchemaDB(t)
	table := (&LongTableNameSample{}).TableName()

	plans, err := modelregistry.ParseIndexPlans(db, &LongTableNameSample{})
	require.NoError(t, err)
	require.Len(t, plans, 1)
	require.Len(t, plans[0].Name, 64)
	require.True(t, strings.HasPrefix(plans[0].Name, "idx_"+table[:20]))

	// Truncation must stay deterministic across runs.
	again, err := modelregistry.ParseIndexPlans(db, &LongTableNameSample{})
	require.NoError(t, err)
	require.Equal(t, plans[0].Name, again[0].Name)
}

// LimitNameSample and OverLimitNameSample sit on either side of the
// PostgreSQL index name limit: idx_<table>_kind is 63 characters for the
// first and 64 for the second.
type (
	LimitNameSample struct {
		Kind string
		modelregistry.Base
	}
	OverLimitNameSample struct {
		Kind string
		modelregistry.Base
	}
)

func (*LimitNameSample) TableName() string {
	return "limit_name_samples_of_fifty_four_characters_in_allxxxx"
}

func (*LimitNameSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Kind"}}}
}

func (*OverLimitNameSample) TableName() string {
	return "over_limit_name_samples_of_fifty_five_characters_in_all"
}

func (*OverLimitNameSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Kind"}}}
}

// postgresDialector is a dialector named like PostgreSQL's, for the plans
// to be read under its identifier limit without a server.
type postgresDialector struct{ tests.DummyDialector }

func (postgresDialector) Name() string { return "postgres" }

// TestParseIndexPlansHoldsNamesToTheDialectsLimit pins the limit the names
// are held to on each database: 63 characters on PostgreSQL, the most it
// stores of an identifier, where a name of exactly 63 stays as it is and
// one of 64 is cut to 63 with the hash, since PostgreSQL would otherwise
// cut it itself and never find the index under its full name again; 64
// elsewhere, where the 64-character name stays as it is.
func TestParseIndexPlansHoldsNamesToTheDialectsLimit(t *testing.T) {
	limitName := "idx_" + (&LimitNameSample{}).TableName() + "_kind"
	overName := "idx_" + (&OverLimitNameSample{}).TableName() + "_kind"
	require.Len(t, limitName, 63)
	require.Len(t, overName, 64)

	postgres, err := gorm.Open(postgresDialector{}, &gorm.Config{DryRun: true})
	require.NoError(t, err)
	limit, err := modelregistry.ParseIndexPlans(postgres, &LimitNameSample{})
	require.NoError(t, err)
	require.Equal(t, limitName, limit[0].Name)
	over, err := modelregistry.ParseIndexPlans(postgres, &OverLimitNameSample{})
	require.NoError(t, err)
	require.Len(t, over[0].Name, 63)
	require.True(t, strings.HasPrefix(over[0].Name, "idx_over_limit_name_samples"))

	other, err := modelregistry.ParseIndexPlans(newSchemaDB(t), &OverLimitNameSample{})
	require.NoError(t, err)
	require.Equal(t, overName, other[0].Name)
}

func TestCheckCrossModelIndexPlanConflicts(t *testing.T) {
	plan := func(table, name string, unique bool, columns ...string) modelregistry.IndexPlan {
		return modelregistry.IndexPlan{Name: name, Table: table, Columns: columns, Unique: unique}
	}

	t.Run("same column sequence on one table conflicts whatever the uniqueness", func(t *testing.T) {
		err := modelregistry.CheckCrossModelIndexPlanConflicts([]modelregistry.ModelIndexPlans{
			{Model: "pkg.SampleA", Plans: []modelregistry.IndexPlan{plan("samples", "idx_samples_kind", false, "kind")}},
			{Model: "pkg.SampleB", Plans: []modelregistry.IndexPlan{plan("samples", "uniq_samples_kind", true, "kind")}},
		})
		require.ErrorContains(t, err, `conflict on table "samples"`)
		require.ErrorContains(t, err, "pkg.SampleA")
		require.ErrorContains(t, err, "pkg.SampleB")
		require.ErrorContains(t, err, "(kind)")
	})

	t.Run("same generated name for different definitions conflicts", func(t *testing.T) {
		err := modelregistry.CheckCrossModelIndexPlanConflicts([]modelregistry.ModelIndexPlans{
			{Model: "pkg.SampleA", Plans: []modelregistry.IndexPlan{plan("samples", "idx_samples_collision", false, "code")}},
			{Model: "pkg.SampleB", Plans: []modelregistry.IndexPlan{plan("samples", "idx_samples_collision", false, "kind")}},
		})
		require.ErrorContains(t, err, `same index name "idx_samples_collision"`)
	})

	t.Run("same columns on different tables do not conflict", func(t *testing.T) {
		require.NoError(t, modelregistry.CheckCrossModelIndexPlanConflicts([]modelregistry.ModelIndexPlans{
			{Model: "pkg.SampleA", Plans: []modelregistry.IndexPlan{plan("samples", "idx_samples_kind", false, "kind")}},
			{Model: "pkg.RecordB", Plans: []modelregistry.IndexPlan{plan("records", "idx_records_kind", false, "kind")}},
		}))
	})

	t.Run("one declaring model per table passes", func(t *testing.T) {
		require.NoError(t, modelregistry.CheckCrossModelIndexPlanConflicts([]modelregistry.ModelIndexPlans{
			{Model: "pkg.SampleA", Plans: []modelregistry.IndexPlan{
				plan("samples", "idx_samples_kind", false, "kind"),
				plan("samples", "uniq_samples_code", true, "code"),
			}},
		}))
	})
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

// UnsizedIndexSample indexes a string field that declares no size, which
// gorm's MySQL dialect stores as longtext.
type UnsizedIndexSample struct {
	Owner string `json:"owner"`

	modelregistry.Base
}

func (*UnsizedIndexSample) TableName() string { return "unsized_index_samples" }

func (*UnsizedIndexSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Owner"}}}
}

// SizedIndexSample indexes the same field with a size, a varchar on MySQL.
type SizedIndexSample struct {
	Owner string `json:"owner" gorm:"size:36"`

	modelregistry.Base
}

func (*SizedIndexSample) TableName() string { return "sized_index_samples" }

func (*SizedIndexSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Owner"}}}
}

// JSONIndexSample indexes a JSON column, which MySQL does not index at all,
// so no size would help.
type JSONIndexSample struct {
	Attrs datatypes.JSON `json:"attrs"`

	modelregistry.Base
}

func (*JSONIndexSample) TableName() string { return "json_index_samples" }

func (*JSONIndexSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Attrs"}}}
}

// TestParseIndexPlansRefusesAColumnMySQLCannotIndex pins the column rule of
// the MySQL dialect: a string field without a size is a longtext column,
// which MySQL cannot index without a key length, so the declaration is
// refused with the fix before any DDL runs; the sized field passes, and
// PostgreSQL, which indexes text columns, is not held to the rule.
func TestParseIndexPlansRefusesAColumnMySQLCannotIndex(t *testing.T) {
	mysqlDB := newMySQLSchemaDB(t)
	_, err := modelregistry.ParseIndexPlans(mysqlDB, &UnsizedIndexSample{})
	require.ErrorContains(t, err, `model UnsizedIndexSample: index field Owner is a longtext column, which MySQL cannot index without a key length; give it a size (gorm:"size:191") or a bounded type`)

	_, err = modelregistry.ParseIndexPlans(mysqlDB, &JSONIndexSample{})
	require.ErrorContains(t, err, `model JSONIndexSample: index field Attrs is a json column, which MySQL does not index; index a bounded column holding the value or a generated column instead`)

	plans, err := modelregistry.ParseIndexPlans(mysqlDB, &SizedIndexSample{})
	require.NoError(t, err)
	require.Len(t, plans, 1)

	postgres, err := gorm.Open(postgresDialector{}, &gorm.Config{DryRun: true})
	require.NoError(t, err)
	_, err = modelregistry.ParseIndexPlans(postgres, &UnsizedIndexSample{})
	require.NoError(t, err)
}

// newMySQLSchemaDB opens a dry-run handle on the MySQL dialect without a
// server, for the column types the dialect gives.
func newMySQLSchemaDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(mysql.New(mysql.Config{DSN: "/", SkipInitializeWithVersion: true}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	require.NoError(t, err)
	return db
}
