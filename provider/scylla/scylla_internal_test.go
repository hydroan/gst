package scylla

import (
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

// TestConsistencyLevelsCoverEveryConfiguredLevel pins that every consistency
// level the configuration admits has a driver level to map onto: a level
// added to config without a row in consistencyLevels would be an error from
// New although the configuration accepted it.
func TestConsistencyLevelsCoverEveryConfiguredLevel(t *testing.T) {
	levels := []config.Consistency{
		config.ConsistencyAny, config.ConsistencyOne, config.ConsistencyTwo, config.ConsistencyThree,
		config.ConsistencyQuorum, config.ConsistencyAll, config.ConsistencyLocalQuorum,
		config.ConsistencyEachQuorum, config.ConsistencyLocalOne,
	}
	require.Len(t, consistencyLevels, len(levels), "a row for each level and no other")
	for _, level := range levels {
		_, ok := consistencyLevels[level]
		require.True(t, ok, string(level))
	}
}
