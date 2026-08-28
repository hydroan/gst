package dcache

import (
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/types/consts"
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
	if got := appName(); got != consts.FrameworkName {
		t.Fatalf("want the framework name fallback, got %q", got)
	}
}
