package database

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/hydroan/gst/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type baseQueryItem struct {
	Code string `json:"code"`

	model.Base
}

type autoBaseQueryItem struct {
	Code string `json:"code"`

	model.AutoBase
}

type presenceQueryItem struct {
	Enabled bool    `json:"enabled"`
	Count   int     `json:"count"`
	Note    *string `json:"note"`
	Legacy  bool    `json:"legacy_json" query:"flag"`

	model.Base
}

func TestStructFieldToMap(t *testing.T) {
	toMap := func(m any, q map[string]string, present map[string]struct{}) map[string]string {
		if q == nil {
			q = make(map[string]string)
		}
		typ := reflect.TypeOf(m).Elem()
		val := reflect.ValueOf(m).Elem()
		structFieldToMap(context.Background(), typ, val, q, present)
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

		q := toMap(item, map[string]string{"id": "outer"}, nil)
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

		q := toMap(item, map[string]string{"id": "outer"}, nil)
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

	t.Run("presence matches the query tag over the json tag", func(t *testing.T) {
		q := toMap(&presenceQueryItem{}, nil, map[string]struct{}{"flag": {}})
		require.Equal(t, "0", q["flag"], "the query tag decides the condition column")
		require.NotContains(t, q, "legacy_json")
	})

	t.Run("present nil pointer stays ignored", func(t *testing.T) {
		q := toMap(&presenceQueryItem{}, nil, map[string]struct{}{"note": {}})
		require.NotContains(t, q, "note", "a nil pointer carries no value to filter by")
	})
}
