package database

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
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

type presenceQueryItem struct {
	Enabled bool    `json:"enabled"`
	Count   int     `json:"count"`
	Note    *string `json:"note"`
	Legacy  bool    `json:"legacy_json" query:"flag"`

	modelregistry.Base
}

func TestStructFieldToMap(t *testing.T) {
	toMap := func(m any, q map[string]any, present map[string]struct{}) map[string]any {
		if q == nil {
			q = make(map[string]any)
		}
		typ := reflect.TypeOf(m).Elem()
		val := reflect.ValueOf(m).Elem()
		goNames, err := modelschema.GoNameIndex(typ)
		require.NoError(t, err)
		structFieldToMap(context.Background(), typ, val, q, present, goNames)
		return q
	}

	t.Run("base lifts framework fields", func(t *testing.T) {
		item := &baseQueryItem{Code: "c1"}
		item.ID = "id1"
		item.CreatedBy = "creator"
		item.UpdatedBy = "updater"

		q := toMap(item, nil, nil)
		require.Equal(t, "c1", q["code"])
		require.Equal(t, "id1", q["id"])
		require.Equal(t, "creator", q["created_by"])
		require.Equal(t, "updater", q["updated_by"])
	})

	t.Run("base keeps values set by the outer model", func(t *testing.T) {
		item := &baseQueryItem{}
		item.ID = "id1"

		q := toMap(item, map[string]any{"id": "outer"}, nil)
		require.Equal(t, "outer", q["id"], "outer model value should have higher priority")
	})

	t.Run("auto base lifts framework fields", func(t *testing.T) {
		item := &autoBaseQueryItem{Code: "c1"}
		item.ID = 123
		item.CreatedBy = "creator"
		item.UpdatedBy = "updater"

		q := toMap(item, nil, nil)
		require.Equal(t, "c1", q["code"])
		require.Equal(t, "123", q["id"], "auto increment id should be lifted in decimal form")
		require.Equal(t, "creator", q["created_by"])
		require.Equal(t, "updater", q["updated_by"])
	})

	t.Run("auto base keeps values set by the outer model", func(t *testing.T) {
		item := &autoBaseQueryItem{}
		item.ID = 123

		q := toMap(item, map[string]any{"id": "outer"}, nil)
		require.Equal(t, "outer", q["id"], "outer model value should have higher priority")
	})

	t.Run("auto base ignores unset id", func(t *testing.T) {
		item := &autoBaseQueryItem{Code: "c1"}

		q := toMap(item, nil, nil)
		require.NotContains(t, q, "id", "unset auto increment id should not become a condition")
	})

	t.Run("auto base ignores deleted at", func(t *testing.T) {
		item := &autoBaseQueryItem{Code: "c1"}
		item.DeletedAt = gorm.DeletedAt{Time: time.Now(), Valid: true}

		q := toMap(item, nil, nil)
		require.Equal(t, "c1", q["code"])
		require.NotContains(t, q, "valid", "gorm.DeletedAt internals should not leak into conditions")
		require.NotContains(t, q, "deleted_at", "deleted_at is managed by soft delete, not query mapping")
	})

	t.Run("zero values without presence stay ignored", func(t *testing.T) {
		q := toMap(&presenceQueryItem{}, nil, nil)
		require.NotContains(t, q, "enabled")
		require.NotContains(t, q, "count")
	})

	t.Run("present zero bool becomes a condition", func(t *testing.T) {
		q := toMap(&presenceQueryItem{}, nil, map[string]struct{}{"enabled": {}})
		require.Equal(t, "0", q["enabled"], "explicitly provided false should filter instead of being dropped")
		require.NotContains(t, q, "count", "fields without presence keep the zero-value skip")
	})

	t.Run("present zero int becomes a condition", func(t *testing.T) {
		q := toMap(&presenceQueryItem{}, nil, map[string]struct{}{"count": {}})
		require.Equal(t, "0", q["count"])
		require.NotContains(t, q, "enabled")
	})

	t.Run("presence matches the query tag while gorm names the column", func(t *testing.T) {
		q := toMap(&presenceQueryItem{}, nil, map[string]struct{}{"flag": {}})
		// The query tag names the URL parameter presence is keyed by, but the
		// condition column is whatever gorm derives from the field name.
		// Using the tag as a column name would emit a column no table has.
		require.Equal(t, "0", q["legacy"])
		require.NotContains(t, q, "flag")
		require.NotContains(t, q, "legacy_json")
	})

	t.Run("present nil pointer stays ignored", func(t *testing.T) {
		q := toMap(&presenceQueryItem{}, nil, map[string]struct{}{"note": {}})
		require.NotContains(t, q, "note", "a nil pointer carries no value to filter by")
	})

	t.Run("number slices join their elements like string slices", func(t *testing.T) {
		q := toMap(&sliceQueryItem{
			Codes:   datatypes.NewJSONSlice([]string{"a", "b"}),
			Scores:  datatypes.NewJSONSlice([]int{1, -2}),
			Ranks:   datatypes.NewJSONSlice([]int8{3}),
			Weights: datatypes.NewJSONSlice([]float64{0.5, 2}),
		}, nil, nil)
		require.Equal(t, map[string]any{"codes": "a,b", "scores": "1,-2", "ranks": "3", "weights": "0.5,2"}, q)
	})

	t.Run("a byte slice is one binary value", func(t *testing.T) {
		q := toMap(&sliceQueryItem{Digest: []byte{0x00, 0xff, 0x10}}, nil, nil)
		require.Equal(t, map[string]any{"digest": []byte{0x00, 0xff, 0x10}}, q)
	})

	t.Run("empty slices add an empty value", func(t *testing.T) {
		q := toMap(&sliceQueryItem{
			Codes:   datatypes.NewJSONSlice([]string{}),
			Scores:  datatypes.NewJSONSlice([]int{}),
			Weights: datatypes.NewJSONSlice([]float64{}),
			Digest:  []byte{},
		}, nil, nil)
		require.Equal(t, map[string]any{"codes": "", "scores": "", "weights": "", "digest": ""}, q)
	})
}
