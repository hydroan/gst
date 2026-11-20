package controller

import (
	"reflect"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// TestNormalizeValueCompactsNilSliceElements covers the reflective walk over
// the value shapes JSON binding can produce.
func TestNormalizeValueCompactsNilSliceElements(t *testing.T) {
	type inner struct {
		Records []*normalizeProbeItem `json:"records"`
	}
	type sample struct {
		Items   []*normalizeProbeItem            `json:"items"`
		Nested  inner                            `json:"nested"`
		Chained *inner                           `json:"chained"`
		ByKey   map[string][]*normalizeProbeItem `json:"by_key"`
		Names   []string                         `json:"names"`
		Missing []*normalizeProbeItem            `json:"missing"`
	}

	first, second := &normalizeProbeItem{Name: "first"}, &normalizeProbeItem{Name: "second"}
	s := &sample{
		Items:   []*normalizeProbeItem{nil, first, nil, second},
		Nested:  inner{Records: []*normalizeProbeItem{nil, first}},
		Chained: &inner{Records: []*normalizeProbeItem{second, nil}},
		ByKey:   map[string][]*normalizeProbeItem{"only": {nil, first, nil}},
		Names:   []string{"kept", "", "kept-too"},
	}
	normalizeValue(reflect.ValueOf(s))

	require.Equal(t, []*normalizeProbeItem{first, second}, s.Items, "top-level slice keeps order without nils")
	require.Equal(t, []*normalizeProbeItem{first}, s.Nested.Records, "slices inside nested structs are compacted")
	require.Equal(t, []*normalizeProbeItem{second}, s.Chained.Records, "slices behind pointer chains are compacted")
	require.Equal(t, []*normalizeProbeItem{first}, s.ByKey["only"], "slices held as map values are compacted")
	require.Equal(t, []string{"kept", "", "kept-too"}, s.Names, "slices of non-nilable elements stay untouched")
	require.Nil(t, s.Missing, "nil slices stay nil instead of becoming empty")
}

// TestNormalizeValueReadsDatesInUTC covers the date half of the walk: every
// datatypes.Date it can set — a field, behind a pointer, in a slice, held as
// a map value, inside a nested struct — becomes the UTC day of the instant
// at midnight, the zero date stays zero, and a time.Time, an instant, is
// left as it is.
func TestNormalizeValueReadsDatesInUTC(t *testing.T) {
	type inner struct {
		Day datatypes.Date `json:"day"`
	}
	type sample struct {
		Day     datatypes.Date            `json:"day"`
		Ptr     *datatypes.Date           `json:"ptr"`
		Days    []datatypes.Date          `json:"days"`
		ByKey   map[string]datatypes.Date `json:"by_key"`
		Nested  inner                     `json:"nested"`
		Zero    datatypes.Date            `json:"zero"`
		Instant time.Time                 `json:"instant"`
	}

	shanghai := time.FixedZone("CST", 8*60*60)
	sent := time.Date(2026, time.January, 2, 0, 0, 0, 0, shanghai)
	ptr := datatypes.Date(sent)
	s := &sample{
		Day:     datatypes.Date(sent),
		Ptr:     &ptr,
		Days:    []datatypes.Date{datatypes.Date(sent)},
		ByKey:   map[string]datatypes.Date{"only": datatypes.Date(sent)},
		Nested:  inner{Day: datatypes.Date(sent)},
		Instant: sent,
	}
	normalizeValue(reflect.ValueOf(s))

	want := datatypes.Date(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
	require.Equal(t, want, s.Day)
	require.Equal(t, want, *s.Ptr)
	require.Equal(t, []datatypes.Date{want}, s.Days)
	require.Equal(t, map[string]datatypes.Date{"only": want}, s.ByKey)
	require.Equal(t, want, s.Nested.Day)
	require.True(t, time.Time(s.Zero).IsZero(), "the zero date stays zero")
	require.True(t, s.Instant.Equal(sent), "an instant is not a calendar date and stays as sent")
	require.Equal(t, shanghai, s.Instant.Location())

	// A time a model declares a date column by its type tag is a day too, in
	// a model and in the items of a batch alike; an instant beside it stays.
	type scheduled struct {
		Due    time.Time  `json:"due" gorm:"type:date"`
		DuePtr *time.Time `json:"due_ptr" gorm:"type:date"`
		At     time.Time  `json:"at"`

		modelregistry.Base
	}
	ptrSent := sent
	items := []*scheduled{{Due: sent, DuePtr: &ptrSent, At: sent}}
	normalizeValue(reflect.ValueOf(&items))

	day := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	require.Equal(t, day, items[0].Due)
	require.Equal(t, day, *items[0].DuePtr)
	require.True(t, items[0].At.Equal(sent), "an instant stays as sent")
	require.Equal(t, shanghai, items[0].At.Location())
}
