package redis_test

import (
	"testing"

	"github.com/hydroan/gst/redis"
)

// TestHealthReportsReachableServer asserts the readiness answer a connected
// process gives.
func TestHealthReportsReachableServer(t *testing.T) {
	if err := redis.Health(t.Context()); err != nil {
		t.Fatalf("want a healthy connection, got %v", err)
	}
}
