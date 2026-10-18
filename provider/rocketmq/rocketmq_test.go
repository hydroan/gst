package rocketmq_test

import (
	"context"
	"testing"
	"time"

	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/provider/rocketmq"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{RocketMQ: true})
}

// TestProducerSends pins that Producer hands out an instance the bootstrap
// started: a message sent through it reaches the broker.
func TestProducerSends(t *testing.T) {
	p, err := rocketmq.Producer()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := p.SendSync(ctx, primitive.NewMessage("sample-topic", []byte("sample")))
	require.NoError(t, err)
	require.Equal(t, primitive.SendOK, result.Status)
}

// TestAdminListsTopics pins that Admin hands out a client that reaches the
// name servers.
func TestAdminListsTopics(t *testing.T) {
	a, err := rocketmq.Admin()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	topics, err := a.FetchAllTopicList(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, topics.TopicList)
}

// TestNewPushConsumerReportsAStartItDoesNotKnow pins that a consume_from_where
// outside its set is an error from NewPushConsumer, before any name server is
// contacted, instead of a consumer started from the last offset. The
// configuration decode does not let such a value through; one set in code
// arrives here.
func TestNewPushConsumerReportsAStartItDoesNotKnow(t *testing.T) {
	_, err := rocketmq.NewPushConsumer(config.RocketMQ{ConsumeFromWhere: "middle"}, "sample-group")
	require.ErrorContains(t, err, "consume_from_where: middle")
}
