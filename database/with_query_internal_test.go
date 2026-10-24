package database

import (
	"database/sql"
	"database/sql/driver"
	"reflect"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/utils/tests"
)

type baseQueryItem struct {
	Code string `json:"code"`

	modelregistry.Base
}

type autoBaseQueryItem struct {
	Code string `json:"code"`

	modelregistry.AutoBase
}

type sliceQueryItem struct {
	Codes   datatypes.JSONSlice[string]  `json:"codes"`
	Scores  datatypes.JSONSlice[int]     `json:"scores"`
	Ranks   datatypes.JSONSlice[int8]    `json:"ranks"`
	Weights datatypes.JSONSlice[float64] `json:"weights"`
	Digest  []byte                       `json:"digest"`

	modelregistry.Base
}

// valuedQueryItem carries the columns that are one struct or map each: an
// instant, a day, a nullable string and a JSON document.
type valuedQueryItem struct {
	At    time.Time         `json:"at"`
	Day   datatypes.Date    `json:"day"`
	Grade sql.NullString    `json:"grade"`
	Meta  datatypes.JSONMap `json:"meta"`

	modelregistry.Base
}

type presenceQueryItem struct {
	Enabled bool    `json:"enabled"`
	Count   int     `json:"count"`
	Note    *string `json:"note"`
	Legacy  bool    `json:"legacy_json" query:"flag"`

	modelregistry.Base
}

// serializedQueryItem stores a slice through gorm's JSON serializer.
type serializedQueryItem struct {
	Tags []string `json:"tags" gorm:"serializer:json"`

	modelregistry.Base
}

// queryPart is embedded twice in pairedQueryItem, under two prefixes, beside
// a top-level field of the same Go name.
type queryPart struct {
	Label string `json:"label"`
}

type pairedQueryItem struct {
	Label  string    `json:"label"`
	First  queryPart `json:"first" gorm:"embedded;embeddedPrefix:first_"`
	Second queryPart `json:"second" gorm:"embedded;embeddedPrefix:second_"`

	modelregistry.Base
}

func TestStructConditionsMapTheQueriedFieldsToTheirColumns(t *testing.T) {
	db, err := gorm.Open(tests.DummyDialector{}, &gorm.Config{})
	require.NoError(t, err)
	conditions := func(m any, present map[string]struct{}) map[string]any {
		columns, err := modelschema.Columns(reflect.TypeOf(m))
		require.NoError(t, err)
		return structConditions(db.Statement, m, columns, present)
	}

	t.Run("every field names its own column", func(t *testing.T) {
		require.Equal(t, map[string]any{"label": "l"}, conditions(&pairedQueryItem{Label: "l"}, nil))
		require.Equal(t, map[string]any{"first_label": "f"}, conditions(&pairedQueryItem{First: queryPart{Label: "f"}}, nil))
		require.Equal(t, map[string]any{"second_label": "s"}, conditions(&pairedQueryItem{Second: queryPart{Label: "s"}}, nil))
	})

	t.Run("base lifts framework fields", func(t *testing.T) {
		item := &baseQueryItem{Code: "c1"}
		item.ID = "id1"
		item.CreatedBy = "creator"
		item.UpdatedBy = "updater"

		require.Equal(t, map[string]any{"code": "c1", "id": "id1", "created_by": "creator", "updated_by": "updater"}, conditions(item, nil))
	})

	t.Run("auto base lifts framework fields", func(t *testing.T) {
		item := &autoBaseQueryItem{Code: "c1"}
		item.ID = 123
		item.CreatedBy = "creator"
		item.UpdatedBy = "updater"

		require.Equal(t, map[string]any{"code": "c1", "id": uint64(123), "created_by": "creator", "updated_by": "updater"}, conditions(item, nil),
			"the auto increment id binds as the number it is")
	})

	t.Run("auto base ignores unset id", func(t *testing.T) {
		require.NotContains(t, conditions(&autoBaseQueryItem{Code: "c1"}, nil), "id", "unset auto increment id should not become a condition")
	})

	t.Run("timestamps are no condition", func(t *testing.T) {
		item := &autoBaseQueryItem{Code: "c1"}
		item.CreatedAt = time.Now()
		item.UpdatedAt = time.Now()
		item.DeletedAt = gorm.DeletedAt{Time: time.Now(), Valid: true}

		require.Equal(t, map[string]any{"code": "c1"}, conditions(item, nil), "the timestamps a fetched record carries never narrow a query")
	})

	t.Run("an instant binds in UTC", func(t *testing.T) {
		at := time.Date(2024, 3, 1, 8, 30, 0, 0, time.FixedZone("", 8*3600))
		require.Equal(t, map[string]any{"at": at.UTC()}, conditions(&valuedQueryItem{At: at}, nil))
	})

	t.Run("a day binds as the UTC day it names", func(t *testing.T) {
		day := datatypes.Date(time.Date(2024, 1, 10, 23, 30, 0, 0, time.FixedZone("", 8*3600)))
		require.Equal(t, map[string]any{"day": time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)}, conditions(&valuedQueryItem{Day: day}, nil))
	})

	t.Run("a nullable string binds as it is", func(t *testing.T) {
		require.Equal(t, map[string]any{"grade": sql.NullString{String: "a", Valid: true}},
			conditions(&valuedQueryItem{Grade: sql.NullString{String: "a", Valid: true}}, nil))
	})

	t.Run("a json document binds for the json check to fail closed on", func(t *testing.T) {
		require.Equal(t, map[string]any{"meta": datatypes.JSONMap{"k": "v"}}, conditions(&valuedQueryItem{Meta: datatypes.JSONMap{"k": "v"}}, nil))
	})

	t.Run("zero values without presence stay ignored", func(t *testing.T) {
		q := conditions(&presenceQueryItem{}, nil)
		require.NotContains(t, q, "enabled")
		require.NotContains(t, q, "count")
	})

	t.Run("present zero bool becomes a condition", func(t *testing.T) {
		q := conditions(&presenceQueryItem{}, map[string]struct{}{"enabled": {}})
		require.Equal(t, false, q["enabled"], "explicitly provided false should filter instead of being dropped")
		require.NotContains(t, q, "count", "fields without presence keep the zero-value skip")
	})

	t.Run("present zero int becomes a condition", func(t *testing.T) {
		q := conditions(&presenceQueryItem{}, map[string]struct{}{"count": {}})
		require.Equal(t, 0, q["count"])
		require.NotContains(t, q, "enabled")
	})

	t.Run("presence matches the query tag while gorm names the column", func(t *testing.T) {
		q := conditions(&presenceQueryItem{}, map[string]struct{}{"flag": {}})
		// The query tag names the URL parameter presence is keyed by, but the
		// condition column is whatever gorm derives from the field name.
		// Using the tag as a column name would emit a column no table has.
		require.Equal(t, false, q["legacy"])
		require.NotContains(t, q, "flag")
		require.NotContains(t, q, "legacy_json")
	})

	t.Run("present nil pointer stays ignored", func(t *testing.T) {
		require.NotContains(t, conditions(&presenceQueryItem{}, map[string]struct{}{"note": {}}), "note", "a nil pointer carries no value to filter by")
	})

	t.Run("a present framework column keeps its zero value out", func(t *testing.T) {
		require.Empty(t, conditions(&autoBaseQueryItem{}, map[string]struct{}{"id": {}, "created_by": {}}))
	})

	t.Run("slices bind as their type stores them", func(t *testing.T) {
		codes := datatypes.NewJSONSlice([]string{"a", "b"})
		scores := datatypes.NewJSONSlice([]int{1, -2})
		ranks := datatypes.NewJSONSlice([]int8{3})
		weights := datatypes.NewJSONSlice([]float64{0.5, 2})
		require.Equal(t, map[string]any{"codes": codes, "scores": scores, "ranks": ranks, "weights": weights},
			conditions(&sliceQueryItem{Codes: codes, Scores: scores, Ranks: ranks, Weights: weights}, nil))
	})

	t.Run("a serializer binds the form its column stores", func(t *testing.T) {
		valuer, ok := conditions(&serializedQueryItem{Tags: []string{"a", "b"}}, nil)["tags"].(driver.Valuer)
		require.True(t, ok, "the condition binds through the field's serializer")
		stored, err := valuer.Value()
		require.NoError(t, err)
		require.Equal(t, `["a","b"]`, stored)
	})

	t.Run("a byte slice is one binary value", func(t *testing.T) {
		require.Equal(t, map[string]any{"digest": []byte{0x00, 0xff, 0x10}}, conditions(&sliceQueryItem{Digest: []byte{0x00, 0xff, 0x10}}, nil))
	})

	t.Run("empty slices keep their empty value for WithQuery to skip", func(t *testing.T) {
		require.Equal(t, map[string]any{
			"codes":   datatypes.NewJSONSlice([]string{}),
			"scores":  datatypes.NewJSONSlice([]int{}),
			"weights": datatypes.NewJSONSlice([]float64{}),
			"digest":  []byte{},
		}, conditions(&sliceQueryItem{
			Codes:   datatypes.NewJSONSlice([]string{}),
			Scores:  datatypes.NewJSONSlice([]int{}),
			Weights: datatypes.NewJSONSlice([]float64{}),
			Digest:  []byte{},
		}, nil))
	})
}
