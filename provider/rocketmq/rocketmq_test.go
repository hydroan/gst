package rocketmq_test

import (
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/provider/rocketmq"
	"github.com/stretchr/testify/require"
)

// TestNewPushConsumerReportsAStartItDoesNotKnow pins that a consume_from_where
// outside its set is an error from NewPushConsumer, before any name server is
// contacted, instead of a consumer started from the last offset. The
// configuration decode does not let such a value through; one set in code
// arrives here.
func TestNewPushConsumerReportsAStartItDoesNotKnow(t *testing.T) {
	_, err := rocketmq.NewPushConsumer(config.RocketMQ{ConsumeFromWhere: "middle"}, "sample-group")
	require.ErrorContains(t, err, "consume_from_where: middle")
}
