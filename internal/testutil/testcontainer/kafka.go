package testcontainer

import (
	"context"
	"os"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/testcontainers/testcontainers-go/modules/kafka"
)

const kafkaImage = "confluentinc/confluent-local:7.5.0"

// SetupKafka starts a kafka container of its own and points the framework at
// it, returning the function that terminates it.
//
// Unlike the other services this one is dedicated even by default: kafka has
// no server-side namespace a shared container could hand out per test binary,
// so isolation would take a topic-prefix contract imposed on business code,
// which is not the framework's call to make. A container per binary keeps
// topics and consumer groups apart by construction.
func SetupKafka() (func() error, error) {
	prepareContainerRuntime()
	ctx := context.Background()

	c, err := kafka.Run(ctx, kafkaImage)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start kafka container")
	}
	terminate := func() error { return c.Terminate(ctx) }

	brokers, err := c.Brokers(ctx)
	if err != nil {
		return nil, errors.CombineErrors(
			errors.Wrap(err, "failed to resolve the kafka brokers"), terminate(),
		)
	}

	// Brokers is a slice, which ApplyConfigToEnv skips; the comma-joined form
	// is what the config layer splits back apart.
	addr := strings.Join(brokers, ",")
	os.Setenv(config.KAFKA_BROKERS, addr)
	os.Setenv(config.KAFKA_ENABLED, "true")
	reportServiceReady("kafka", addr)

	return terminate, nil
}
