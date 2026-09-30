package sparetest

import (
	"testing"

	sinklogger "example.com/module/sink/logger"
)

func TestUse(t *testing.T) {
	if sinklogger.New() == "" {
		t.Fatal("empty")
	}
}
