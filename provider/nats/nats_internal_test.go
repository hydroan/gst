package nats

import (
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/logger"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestConnectOptionsMaxReconnectsVerbatim(t *testing.T) {
	// The configured value must reach the client untranslated: -1 keeps
	// reconnecting forever, 0 disables reconnecting, a positive value bounds
	// the attempts. A > 0 gate would silently swap -1 and 0 for the NATS
	// library default of 60 bounded attempts, which permanently closes the
	// connection once spent.
	for _, configured := range []int{-1, 0, 7} {
		opts, err := connectOptions(config.Nats{MaxReconnects: configured})
		require.NoError(t, err)

		applied := nats.GetDefaultOptions()
		for _, opt := range opts {
			require.NoError(t, opt(&applied))
		}
		require.Equal(t, configured, applied.MaxReconnect)
	}
}

// TestErrorHandlerTakesAnErrorOfTheConnection pins that the error handler
// logs an error the client reports with no subscription, the way it reports
// every error of the connection itself, a permission refused or a failed
// flush: the entry carries the error and no subject. An error of a
// subscription is logged with the subject of the subscription.
func TestErrorHandlerTakesAnErrorOfTheConnection(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	t.Cleanup(zap.ReplaceGlobals(zap.New(core)))
	original := logger.Nats
	logger.Nats = logger.Fallback("nats")
	t.Cleanup(func() { logger.Nats = original })

	opts, err := connectOptions(config.Nats{})
	require.NoError(t, err)
	applied := nats.GetDefaultOptions()
	for _, opt := range opts {
		require.NoError(t, opt(&applied))
	}

	applied.AsyncErrorCB(nil, nil, nats.ErrAuthorization)
	applied.AsyncErrorCB(nil, &nats.Subscription{Subject: "sample.events"}, nats.ErrSlowConsumer)

	entries := logs.All()
	require.Len(t, entries, 2)
	require.Equal(t, "nats error", entries[0].Message)
	require.Equal(t, nats.ErrAuthorization.Error(), entries[0].ContextMap()["error"])
	require.NotContains(t, entries[0].ContextMap(), "subject")
	require.Equal(t, nats.ErrSlowConsumer.Error(), entries[1].ContextMap()["error"])
	require.Equal(t, "sample.events", entries[1].ContextMap()["subject"])
}
