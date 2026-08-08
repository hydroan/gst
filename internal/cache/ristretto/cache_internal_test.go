package ristretto

import (
	"testing"

	"github.com/hydroan/gst/config"
)

// TestBuildConfSizesFromDefault pins the sizing rule: MaxCost bounds the
// entry count (cost 1 per entry) and NumCounters is ten times the capacity as
// the admission policy requires. This is the regression guard for the earlier
// misconfiguration that left MaxCost effectively unbounded.
func TestBuildConfSizesFromDefault(t *testing.T) {
	conf := buildConf[string]()
	if conf.MaxCost != defaultMaxEntries {
		t.Fatalf("want MaxCost to equal the default capacity, got %d", conf.MaxCost)
	}
	if conf.NumCounters != defaultMaxEntries*10 {
		t.Fatalf("want NumCounters to be ten times the capacity, got %d", conf.NumCounters)
	}
}

// TestBuildConfHonorsConfiguredMaxEntries asserts the single cache
// configuration knob overrides the built-in default, and that non-positive
// values fall back to it.
func TestBuildConfHonorsConfiguredMaxEntries(t *testing.T) {
	old := config.App.Cache.MaxEntries
	defer func() { config.App.Cache.MaxEntries = old }()

	config.App.Cache.MaxEntries = 5000
	conf := buildConf[string]()
	if conf.MaxCost != 5000 || conf.NumCounters != 50000 {
		t.Fatalf("want the configured capacity to win, got MaxCost=%d NumCounters=%d", conf.MaxCost, conf.NumCounters)
	}

	config.App.Cache.MaxEntries = -1
	conf = buildConf[string]()
	if conf.MaxCost != defaultMaxEntries {
		t.Fatalf("want fallback to the default for non-positive values, got %d", conf.MaxCost)
	}
}
