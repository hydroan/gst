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

func TestStructFieldToMap(t *testing.T) {
	toMap := func(m any, q map[string]string) map[string]string {
		if q == nil {
			q = make(map[string]string)
		}
		typ := reflect.TypeOf(m).Elem()
		val := reflect.ValueOf(m).Elem()
		structFieldToMap(context.Background(), typ, val, q)
		return q
	}

	t.Run("base lifts framework fields", func(t *testing.T) {
		item := &baseQueryItem{Code: "c1"}
		item.ID = "id1"
		item.CreatedBy = "creator"
		item.UpdatedBy = "updater"

		q := toMap(item, nil)
		require.Equal(t, "c1", q["code"])
		require.Equal(t, "id1", q["id"])
		require.Equal(t, "creator", q["created_by"])
		require.Equal(t, "updater", q["updated_by"])
	})

	t.Run("base keeps values set by the outer model", func(t *testing.T) {
		item := &baseQueryItem{}
		item.ID = "id1"

		q := toMap(item, map[string]string{"id": "outer"})
		require.Equal(t, "outer", q["id"], "outer model value should have higher priority")
	})

	t.Run("auto base lifts framework fields", func(t *testing.T) {
		item := &autoBaseQueryItem{Code: "c1"}
		item.ID = 123
		item.CreatedBy = "creator"
		item.UpdatedBy = "updater"

		q := toMap(item, nil)
		require.Equal(t, "c1", q["code"])
		require.Equal(t, "123", q["id"], "auto increment id should be lifted in decimal form")
		require.Equal(t, "creator", q["created_by"])
		require.Equal(t, "updater", q["updated_by"])
	})

	t.Run("auto base keeps values set by the outer model", func(t *testing.T) {
		item := &autoBaseQueryItem{}
		item.ID = 123

		q := toMap(item, map[string]string{"id": "outer"})
		require.Equal(t, "outer", q["id"], "outer model value should have higher priority")
	})

	t.Run("auto base ignores unset id", func(t *testing.T) {
		item := &autoBaseQueryItem{Code: "c1"}

		q := toMap(item, nil)
		require.NotContains(t, q, "id", "unset auto increment id should not become a condition")
	})

	t.Run("auto base ignores deleted at", func(t *testing.T) {
		item := &autoBaseQueryItem{Code: "c1"}
		item.DeletedAt = gorm.DeletedAt{Time: time.Now(), Valid: true}

		q := toMap(item, nil)
		require.Equal(t, "c1", q["code"])
		require.NotContains(t, q, "valid", "gorm.DeletedAt internals should not leak into conditions")
		require.NotContains(t, q, "deleted_at", "deleted_at is managed by soft delete, not query mapping")
	})
}
