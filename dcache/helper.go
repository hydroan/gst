package dcache

import (
	"fmt"
	"os"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/types/consts"
	"github.com/twmb/franz-go/pkg/kgo"
)

// cacheTopic returns the Kafka topic carrying cache set/delete events. The
// default derives from the application name so applications sharing one
// Kafka cluster do not consume each other's cache events.
func cacheTopic() string {
	if t := config.App.Cache.Topic; t != "" {
		return t
	}
	return appName() + "-dcache"
}

// appName guards topic derivation against reads before the configuration is
// loaded.
func appName() string {
	if config.App.Name != "" {
		return config.App.Name
	}
	return consts.FrameworkName
}

func newProducer(brokers []string, topic string) (*kgo.Client, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	return kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.AllowAutoTopicCreation(),
		kgo.ClientID(fmt.Sprintf("producer-%s-%s", topic, hostname)),

		// low latency tuning
		kgo.ProducerLinger(1*time.Millisecond), // extremely short batching delay
		// kgo.ProducerBatchMaxBytes(n),           // smaller batches
		// kgo.MaxBufferedRecords(n),              // large buffer to absorb traffic bursts

		// trade reliability for lower latency
		// message idempotency is not needed: the state node deduplicates and tracks the highest
		// timestamp, which is what guarantees eventual state consistency
		// locally the settings below were found to cut 100-200ms per operator batch
		// kgo.RequiredAcks(kgo.NoAck()),
		// kgo.DisableIdempotentWrite(),           // disable idempotency to reduce the overhead

		// Bound the produce path for real: RetryTimeout only covers the
		// client's own metadata requests, never produces, so the record
		// limits below are what keeps a kafka outage from buffering and
		// retrying forever until the publishing pool blocks.
		kgo.RetryTimeout(300*time.Millisecond),
		kgo.RecordRetries(3),
		kgo.RecordDeliveryTimeout(3*time.Second),

		// TCP connection tuning
		kgo.DialTimeout(300*time.Millisecond),     // short connect timeout
		kgo.RequestTimeoutOverhead(1*time.Second), // at least 1s, otherwise kgo.NewClient errors out
	)
}

// newConsumer creates a kafka consumer; there are several of them.
func newConsumer(brokers []string, topic string, group string) (*kgo.Client, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	return kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.AllowAutoTopicCreation(),
		kgo.ConsumeTopics(topic),
		kgo.ClientID(fmt.Sprintf("consumer-%s-%s", topic, hostname)),

		// neither automatic nor manual commits are needed, every restart starts from the latest offset
		kgo.DisableAutoCommit(),
		// The group is private to the instance (the caller suffixes it with
		// the instance id), so every instance receives every partition; a
		// shared group would silently split the partitions between members.
		kgo.ConsumerGroup(group),
		// always consume the newest messages after a start
		kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()),

		// low latency consumption tuning
		kgo.FetchMaxWait(10*time.Millisecond), // very short fetch wait
		kgo.FetchMinBytes(1),                  // return as soon as any data is available
		// kgo.FetchMaxBytes(n),           // larger maximum fetch size (10MB)

		// TCP connection tuning
		kgo.DialTimeout(300*time.Millisecond),
	)
}

func calculateHitRatio(hits, misses int64) int64 {
	if hits+misses == 0 {
		return 0
	}
	return hits * 100 / (hits + misses)
}
