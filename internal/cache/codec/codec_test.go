package codec_test

import (
	"testing"

	"github.com/hydroan/gst/internal/cache/codec"
)

type sample struct {
	Name string `json:"name"`
	Num  int    `json:"num"`
}

// TestInterfaceValuesRoundtrip is the regression guard for the asymmetry this
// package exists to remove: encoding dispatched on a value's dynamic type
// while decoding dispatched on its destination, so anything stored through an
// interface-typed cache was written compactly and then failed to decode.
func TestInterfaceValuesRoundtrip(t *testing.T) {
	cases := []struct {
		name  string
		value any
	}{
		{"string", "hello"},
		{"bytes", []byte("hello")},
		{"int", 42},
		{"bool", true},
		{"float", 1.5},
		{"struct", sample{Name: "n", Num: 7}},
		{"nil", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := codec.Marshal(c.value)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var back any
			if err := codec.Unmarshal(data, &back); err != nil {
				t.Fatalf("unmarshal %q: %v", data, err)
			}
		})
	}
}

// TestConcreteValuesRoundtrip covers the ordinary path, where the compact
// encoding applies and both directions see the same concrete type.
func TestConcreteValuesRoundtrip(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		data, err := codec.Marshal("hello")
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var back string
		if err := codec.Unmarshal(data, &back); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if back != "hello" {
			t.Fatalf("want %q, got %q", "hello", back)
		}
	})
	t.Run("int", func(t *testing.T) {
		data, err := codec.Marshal(42)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var back int
		if err := codec.Unmarshal(data, &back); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if back != 42 {
			t.Fatalf("want 42, got %d", back)
		}
	})
	t.Run("struct", func(t *testing.T) {
		want := sample{Name: "n", Num: 7}
		data, err := codec.Marshal(want)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var back sample
		if err := codec.Unmarshal(data, &back); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if back != want {
			t.Fatalf("want %+v, got %+v", want, back)
		}
	})
	t.Run("bytes keep the compact form", func(t *testing.T) {
		data, err := codec.Marshal([]byte("hello"))
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		// The compact path must not base64 the payload; that would inflate it
		// by a third and eat into the byte-addressed backends' entry limits.
		if string(data) != "hello" {
			t.Fatalf("want the raw bytes, got %q", data)
		}
	})
}
