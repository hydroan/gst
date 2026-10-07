package codec_test

import (
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/cache/codec"
	"github.com/hydroan/gst/internal/types"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type sample struct {
	Name string `json:"name"`
	Num  int    `json:"num"`
}

// TestInterfaceValuesRoundtrip guards the symmetry this package exists for: an
// encoding that dispatches on a value's dynamic type while decoding dispatches
// on its destination writes anything stored through an interface-typed cache
// compactly and then fails to decode it.
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

// TestDropUnreadableAnswersAMissAfterDroppingTheEntry pins the one answer
// every byte-addressed backend gives an entry whose bytes do not decode as the
// caller's type: a Warn naming the key and the decode error, the drop, and
// types.ErrEntryNotFound so the caller treats it like a plain miss. A drop
// that fails is logged as well and the answer is still a miss.
func TestDropUnreadableAnswersAMissAfterDroppingTheEntry(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	t.Cleanup(zap.ReplaceGlobals(zap.New(core)))

	t.Run("dropped", func(t *testing.T) {
		logs.TakeAll()
		dropped := 0
		err := codec.DropUnreadable("sample-key", errors.New("unexpected value type"), func() error {
			dropped++
			return nil
		})
		if !errors.Is(err, types.ErrEntryNotFound) {
			t.Fatalf("want ErrEntryNotFound, got %v", err)
		}
		if dropped != 1 {
			t.Fatalf("want the entry dropped once, got %d", dropped)
		}
		entries := logs.TakeAll()
		if len(entries) != 1 {
			t.Fatalf("want one warning, got %+v", entries)
		}
		fields := entries[0].ContextMap()
		if entries[0].Level != zap.WarnLevel || entries[0].Message != "dropping a cache entry that cannot be decoded" ||
			fields["key"] != "sample-key" || fields["error"] != "unexpected value type" {
			t.Fatalf("want a Warn naming the key and the decode error, got %+v", entries[0])
		}
	})

	t.Run("drop failed", func(t *testing.T) {
		logs.TakeAll()
		err := codec.DropUnreadable("sample-key", errors.New("unexpected value type"), func() error {
			return errors.New("connection reset")
		})
		if !errors.Is(err, types.ErrEntryNotFound) {
			t.Fatalf("want ErrEntryNotFound even when the drop fails, got %v", err)
		}
		entries := logs.TakeAll()
		if len(entries) != 2 || entries[1].Message != "failed to drop the unreadable cache entry" ||
			entries[1].ContextMap()["key"] != "sample-key" || entries[1].ContextMap()["error"] != "connection reset" {
			t.Fatalf("want a second warning carrying the drop error, got %+v", entries)
		}
	})
}
