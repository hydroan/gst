package controller

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
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
