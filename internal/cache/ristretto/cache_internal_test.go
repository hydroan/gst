package ristretto

import "testing"

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
