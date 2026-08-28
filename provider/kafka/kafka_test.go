package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
)

const sampleTopic = "sample-topic"

func TestNew(t *testing.T) {
	t.Run("produce and consume roundtrip", func(t *testing.T) {
		addrs := newFakeCluster(t)
		cfg := config.Kafka{Brokers: addrs, ClientID: "gst-test"}

		producer, err := New(cfg)
		if err != nil {
			t.Fatalf("New producer: %v", err)
		}
		defer producer.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err = producer.Ping(ctx); err != nil {
			t.Fatalf("Ping: %v", err)
		}
		if err = producer.ProduceSync(ctx, &kgo.Record{Topic: sampleTopic, Value: []byte("hello")}).FirstErr(); err != nil {
			t.Fatalf("ProduceSync: %v", err)
		}

		// Extra options are appended after framework options, turning the
		// client into a direct consumer.
		consumer, err := New(
			cfg,
			kgo.ConsumeTopics(sampleTopic),
			kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		)
		if err != nil {
			t.Fatalf("New consumer: %v", err)
		}
		defer consumer.Close()

		fetches := consumer.PollFetches(ctx)
		if errs := fetches.Errors(); len(errs) > 0 {
			t.Fatalf("PollFetches: %v", errs)
		}
		records := fetches.Records()
		if len(records) != 1 || string(records[0].Value) != "hello" {
			t.Fatalf("expected 1 record with value hello, got %d records", len(records))
		}
	})

	t.Run("unsupported sasl mechanism", func(t *testing.T) {
		cfg := config.Kafka{
			Brokers:       []string{"127.0.0.1:9092"},
			SASLEnabled:   true,
			SASLMechanism: "unknown",
		}
		if _, err := New(cfg); err == nil {
			t.Fatal("expected error for unsupported sasl mechanism, got nil")
		}
	})

	t.Run("invalid tls files", func(t *testing.T) {
		cfg := config.Kafka{
			Brokers:    []string{"127.0.0.1:9092"},
			TLSEnabled: true,
			CertFile:   "/nonexistent/cert.pem",
			KeyFile:    "/nonexistent/key.pem",
		}
		if _, err := New(cfg); err == nil {
			t.Fatal("expected error for invalid tls files, got nil")
		}
	})
}

func TestInitProvider(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		old := config.App.Kafka
		config.App.Kafka = config.Kafka{Enabled: false}
		t.Cleanup(func() { config.App.Kafka = old })

		if err := initProvider(); err != nil {
			t.Fatalf("Init with kafka disabled: %v", err)
		}
		if _, err := Client(); err == nil {
			t.Fatal("expected error from Client before initialization, got nil")
		}
	})

	t.Run("initialized against cluster", func(t *testing.T) {
		addrs := newFakeCluster(t)
		old := config.App.Kafka
		config.App.Kafka = config.Kafka{Enabled: true, Brokers: addrs, ClientID: "gst-test"}
		t.Cleanup(func() {
			_ = closeProvider()
			config.App.Kafka = old
		})

		if err := initProvider(); err != nil {
			t.Fatalf("Init: %v", err)
		}

		c, err := Client()
		if err != nil {
			t.Fatalf("Client: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err = c.Ping(ctx); err != nil {
			t.Fatalf("Ping: %v", err)
		}

		a, err := Admin()
		if err != nil {
			t.Fatalf("Admin: %v", err)
		}
		topics, err := a.ListTopics(ctx)
		if err != nil {
			t.Fatalf("ListTopics: %v", err)
		}
		if !topics.Has(sampleTopic) {
			t.Fatalf("expected topic %s to exist", sampleTopic)
		}

		if err = closeProvider(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if _, err = Client(); err == nil {
			t.Fatal("expected error from Client after Close, got nil")
		}
	})
}

// newFakeCluster starts an in-memory kfake cluster seeded with sampleTopic
// and returns its listen addresses. The cluster is closed on test cleanup.
func newFakeCluster(t *testing.T) []string {
	t.Helper()
	cluster, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, sampleTopic))
	if err != nil {
		t.Fatalf("start kfake cluster: %v", err)
	}
	t.Cleanup(cluster.Close)
	return cluster.ListenAddrs()
}
