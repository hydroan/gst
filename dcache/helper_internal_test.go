package dcache

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/types"
	"github.com/panjf2000/ants/v2"
)

// TestHitRatio pins the percentage arithmetic, including the zero-sample
// case.
func TestHitRatio(t *testing.T) {
	if got := hitRatio(0, 0); got != 0 {
		t.Fatalf("want 0 for no samples, got %d", got)
	}
	if got := hitRatio(3, 1); got != 75 {
		t.Fatalf("want 75, got %d", got)
	}
}

// TestCacheTopic pins the topic resolution order: the configured topic wins,
// otherwise it derives from the application name.
func TestCacheTopic(t *testing.T) {
	oldTopic, oldName := config.App.Cache.Topic, config.App.Name
	defer func() { config.App.Cache.Topic, config.App.Name = oldTopic, oldName }()

	config.App.Cache.Topic = "configured-topic"
	if got := cacheTopic(); got != "configured-topic" {
		t.Fatalf("want the configured topic, got %q", got)
	}

	config.App.Cache.Topic = ""
	config.App.Name = "sample"
	if got := cacheTopic(); got != "sample-dcache" {
		t.Fatalf("want the derived topic, got %q", got)
	}
}

// TestAppName pins the fallback to the framework name before the
// configuration is loaded.
func TestAppName(t *testing.T) {
	old := config.App.Name
	defer func() { config.App.Name = old }()

	config.App.Name = ""
	if got := appName(); got != types.FrameworkName {
		t.Fatalf("want the framework name fallback, got %q", got)
	}
}

// TestMetricsSnapshotsTheCounters pins the snapshot the monitor logs: every
// counter as it stands, the hit ratio of the hits and the misses, and the
// capacity and the running goroutines of the publish pool.
func TestMetricsSnapshotsTheCounters(t *testing.T) {
	pool, err := ants.NewPool(2)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Release()
	dc := &replicatedCache[string]{pubPool: pool}
	dc.hits.Store(3)
	dc.misses.Store(1)
	dc.sets.Store(4)
	dc.deletes.Store(5)
	dc.peerSets.Store(6)
	dc.peerDeletes.Store(7)
	dc.publishDropped.Store(8)
	dc.publishFailed.Store(9)

	want := &replicatedMetrics{
		Hits: 3, Misses: 1, Ratio: 75, Sets: 4, Deletes: 5,
		PeerSets: 6, PeerDeletes: 7,
		PublishDropped: 8, PublishFailed: 9,
		GoroutinesPoolCap: 2, GoroutinesUsed: 0,
	}
	if got := dc.metrics(); !reflect.DeepEqual(got, want) {
		t.Fatalf("metrics() = %+v, want %+v", got, want)
	}
}
