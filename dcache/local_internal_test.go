package dcache

import (
	"testing"

	"github.com/hydroan/gst/config"
)

// TestLocalBuildConfSizesFromDefault pins the local tier sizing: MaxCost
// bounds the entry count (cost 1 per entry), NumCounters is ten times the
// capacity, and metrics stay enabled for the distributed cache monitor. This
// guards the regression where the local tier allocated counters for 16.7M
// entries (roughly 768MB per type) up front.
func TestLocalBuildConfSizesFromDefault(t *testing.T) {
	conf := buildConf[string]()
	if conf.MaxCost != defaultMaxEntries {
		t.Fatalf("want MaxCost to equal the default capacity, got %d", conf.MaxCost)
	}
	if conf.NumCounters != defaultMaxEntries*10 {
		t.Fatalf("want NumCounters to be ten times the capacity, got %d", conf.NumCounters)
	}
	if !conf.Metrics {
		t.Fatal("want metrics enabled for the distributed cache monitor")
	}
	if !conf.IgnoreInternalCost {
		t.Fatal("want internal cost ignored so MaxCost keeps the entry-count semantics")
	}
}

// TestLocalBuildConfHonorsConfiguredMaxEntries asserts the shared cache
// configuration knob also drives the local tier.
func TestLocalBuildConfHonorsConfiguredMaxEntries(t *testing.T) {
	old := config.App.Cache.MaxEntries
	defer func() { config.App.Cache.MaxEntries = old }()

	config.App.Cache.MaxEntries = 5000
	conf := buildConf[string]()
	if conf.MaxCost != 5000 || conf.NumCounters != 50000 {
		t.Fatalf("want the configured capacity to win, got MaxCost=%d NumCounters=%d", conf.MaxCost, conf.NumCounters)
	}
}
