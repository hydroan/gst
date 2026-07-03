package controller

import (
	"testing"

	"github.com/hydroan/gst/model"
)

type listBenchmarkModel struct {
	Name string `schema:"name"`

	model.Base
}

type listBenchmarkQueryableModel struct {
	Name string `schema:"name"`

	model.Query
	model.Base
}

type listBenchmarkPaginatableModel struct {
	Name string `schema:"name"`

	model.Pagination
	model.Base
}

type listBenchmarkCursorableModel struct {
	Name string `schema:"name"`

	model.Cursor
	model.Base
}

var (
	listBenchmarkPlainQuery = map[string][]string{
		"name": {"alice"},
	}
	listBenchmarkFullQuery = map[string][]string{
		"name":           {"alice"},
		"_fuzzy":         {"true"},
		"_sortby":        {"created_at desc"},
		"page":           {"2"},
		"size":           {"10"},
		"_cursor_value":  {"0196a0b3-c9d1-713c-870e-adc76af9f857"},
		"_cursor_fields": {"id"},
		"_cursor_next":   {"true"},
	}
	listBenchmarkPaginationQuery = map[string][]string{
		"page": {"2"},
		"size": {"10"},
	}
	listBenchmarkCursorQuery = map[string][]string{
		"_cursor_value":  {"0196a0b3-c9d1-713c-870e-adc76af9f857"},
		"_cursor_fields": {"id"},
		"_cursor_next":   {"true"},
	}
	listBenchmarkRejectedQuery = map[string][]string{
		"_fuzzy": {"true"},
	}
)

func BenchmarkDecodeListQuery(b *testing.B) {
	b.Run("PlainModelField", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var m listBenchmarkModel
			if err := decodeListQuery(&m, listBenchmarkPlainQuery); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Query", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var m listBenchmarkQueryableModel
			if err := decodeListQuery(&m, listBenchmarkFullQuery); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Pagination", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var m listBenchmarkPaginatableModel
			if err := decodeListQuery(&m, listBenchmarkPaginationQuery); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Cursor", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var m listBenchmarkCursorableModel
			if err := decodeListQuery(&m, listBenchmarkCursorQuery); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("RejectFrameworkKey", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var m listBenchmarkModel
			if err := decodeListQuery(&m, listBenchmarkRejectedQuery); err == nil {
				b.Fatal("expected framework query key to be rejected")
			}
		}
	})
}
