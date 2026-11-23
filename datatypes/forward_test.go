package datatypes_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hydroan/gst/datatypes"
	internaldatatypes "github.com/hydroan/gst/internal/datatypes"
	"github.com/stretchr/testify/require"
)

// TestTheTypesAreTheFrameworksOwn pins the public package as a forwarder:
// each type a model declares through it is the one the framework reads,
// stores and writes, and each function answers what the one it forwards
// answers.
func TestTheTypesAreTheFrameworksOwn(t *testing.T) {
	require.Equal(t, reflect.TypeFor[internaldatatypes.Date](), reflect.TypeFor[datatypes.Date]())
	require.Equal(t, reflect.TypeFor[internaldatatypes.TimeOfDay](), reflect.TypeFor[datatypes.TimeOfDay]())
	require.Equal(t, reflect.TypeFor[internaldatatypes.JSON[json.RawMessage]](), reflect.TypeFor[datatypes.JSON[json.RawMessage]]())
	require.Equal(t, reflect.TypeFor[internaldatatypes.JSONSlice[string]](), reflect.TypeFor[datatypes.JSONSlice[string]]())
	require.Equal(t, reflect.TypeFor[internaldatatypes.JSONMap](), reflect.TypeFor[datatypes.JSONMap]())

	day, err := datatypes.ParseDate("2026-01-02")
	require.NoError(t, err)
	require.Equal(t, "2026-01-02", day.String())

	opens, err := datatypes.ParseTimeOfDay("17:30:00")
	require.NoError(t, err)
	require.Equal(t, datatypes.NewTimeOfDay(17, 30, 0, 0), opens)

	require.Equal(t, []string{"a"}, datatypes.NewJSON([]string{"a"}).Data())
	require.Equal(t, datatypes.JSONSlice[string]{"a"}, datatypes.NewJSONSlice([]string{"a"}))
}
