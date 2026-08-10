package testcontainer

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestSetupKafka(t *testing.T) {
	isolateEnv(t, config.KAFKA_BROKERS, config.KAFKA_ENABLED)

	cleanup, err := SetupKafka()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanup()) })

	brokers := os.Getenv(config.KAFKA_BROKERS)
	require.NotEmpty(t, brokers)
	require.Equal(t, "true", os.Getenv(config.KAFKA_ENABLED))

	// A produce-consume round trip is what proves the advertised listener
	// points back at the mapped port; a bare dial would pass on a broker no
	// client could actually talk to.
	client, err := kgo.NewClient(
		kgo.SeedBrokers(strings.Split(brokers, ",")...),
		kgo.ConsumeTopics("sample"),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err = kadm.NewClient(client).CreateTopic(ctx, 1, 1, nil, "sample")
	require.NoError(t, err)
	require.NoError(t, client.ProduceSync(ctx, &kgo.Record{Topic: "sample", Value: []byte("value")}).FirstErr())

	fetches := client.PollFetches(ctx)
	require.NoError(t, fetches.Err())
	records := fetches.Records()
	require.Len(t, records, 1)
	require.Equal(t, "value", string(records[0].Value))
}
