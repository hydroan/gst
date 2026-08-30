package redis_test

import (
	"testing"

	"github.com/hydroan/gst/internal/testutil"
)

func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{Redis: true})
}
