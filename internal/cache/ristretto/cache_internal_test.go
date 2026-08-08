package ristretto

import (
	"testing"

	"github.com/hydroan/gst/config"
)

// TestBuildConfSizesFromCapacity pins the sizing rule: MaxCost bounds the
// entry count (cost 1 per entry) and NumCounters is ten times the capacity as
// the admission policy requires. This is the regression guard for the earlier
// misconfiguration that left MaxCost effectively unbounded.
func TestBuildConfSizesFromCapacity(t *testing.T) {
	old := config.App.Cache.Capacity
	config.App.Cache.Capacity = 5000
	defer func() { config.App.Cache.Capacity = old }()

	conf := buildConf[string]()
	if conf.MaxCost != 5000 {
		t.Fatalf("want MaxCost to equal the capacity, got %d", conf.MaxCost)
	}
	if conf.NumCounters != 50000 {
		t.Fatalf("want NumCounters to be ten times the capacity, got %d", conf.NumCounters)
	}
}
