package dcache

import (
	"fmt"
	"os"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/provider/kafka"
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

// newProducer builds the publishing client through the kafka provider's New,
// so SASL, TLS and the other connection concerns configured there are
// honored; dcache's own tuning is appended on top. Importing the provider
// also registers it with bootstrap — deliberate: dcache requires kafka, and
// the provider's startup ping surfaces a dead broker configuration at boot.
func newProducer(cfg config.Kafka, topic string) (*kgo.Client, error) {
	hostname, err := os.Hostname()
	if err != nil {
		// First-hand exit of a stack-less third-party/standard-library error;
		// see the error-stack contract in the database package doc.
		return nil, errors.WithStack(err)
	}
	return kafka.New(cfg,
		// Route the client's own log lines into dcache's file instead of the
		// kafka provider's; the last logger option wins.
		kafka.Logger(&logger.Dcache),
		kgo.AllowAutoTopicCreation(),
		kgo.ClientID(fmt.Sprintf("producer-%s-%s", topic, hostname)),

		// low latency tuning
		kgo.ProducerLinger(1*time.Millisecond), // extremely short batching delay
		// kgo.ProducerBatchMaxBytes(n),           // smaller batches
		// kgo.MaxBufferedRecords(n),              // large buffer to absorb traffic bursts

		// Idempotent dedup is not needed for correctness — every instance
		// deduplicates through its per-key timestamp watermark — so the
		// produce path may cancel idempotent batches when the limits below
		// expire, accepting a possible duplicate delivery, which the
		// watermark absorbs. Without the cancellation the limits would only
		// hold while no request is in flight.
		kgo.AllowIdempotentProduceCancellation(),

		// Bound the produce path for real: RetryTimeout only covers the
		// client's own metadata requests, never produces; the record limits
		// below are what keeps a kafka outage from buffering and retrying
		// forever until the publishing pool blocks. A request wedged on a
		// dead connection can still hold one attempt for roughly the produce
		// request timeout before the record fails.
		kgo.RetryTimeout(300*time.Millisecond),
		kgo.RecordRetries(3),
		kgo.RecordDeliveryTimeout(3*time.Second),

		// TCP connection tuning
		kgo.DialTimeout(300*time.Millisecond),     // short connect timeout
		kgo.RequestTimeoutOverhead(1*time.Second), // well above franz-go's 100ms floor, tolerant of slow brokers
	)
}

// newConsumer creates a kafka consumer; each cache type's instance owns one.
// Like newProducer it goes through the kafka provider's New.
func newConsumer(cfg config.Kafka, topic string, group string) (*kgo.Client, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return kafka.New(cfg,
		kafka.Logger(&logger.Dcache),
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

func hitRatio(hits, misses int64) int64 {
	if hits+misses == 0 {
		return 0
	}
	return hits * 100 / (hits + misses)
}
